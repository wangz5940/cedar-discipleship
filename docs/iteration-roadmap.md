# 功能迭代路线图

本文档记录前后端分离版本的下一轮功能迭代。当前已完成基础后端、静态前端、MySQL schema、分区维护和 Docker Compose 部署；本轮迭代聚焦默认小组、旧数据迁移、完整管理台、资源上传统计和 NAS 备份。

## 1. 迭代目标

1. 支持多小组成员设置默认展示小组。
2. 增加旧 JSON 数据迁移 CLI，将 `config.json`、`data/records.json` 导入 MySQL。
3. 扩展当前静态前端为更完整的管理台，包括周计划编辑、资源上传和统计图表。
4. 根据 NAS 实际路径调整 `AGP_ASSETS_ROOT`，并把 `/data/agp/assets` 纳入备份策略。

## 2. 默认展示小组

### 2.1 需求

- 当成员只属于一个小组时，登录后直接进入该小组。
- 当成员属于多个小组时，允许成员设置一个默认展示小组。
- 设置默认小组后，下次登录成功后直接进入默认小组。
- 默认小组必须是该成员当前仍然属于的小组。
- 如果默认小组被移除、停用或成员被移出该组，则登录后回退到小组选择页。

### 2.2 表结构变更

在 `users` 表增加默认小组字段：

```sql
ALTER TABLE users
  ADD COLUMN default_group_id BIGINT UNSIGNED NULL AFTER is_super_admin,
  ADD KEY idx_default_group (default_group_id);
```

说明：

- `default_group_id` 只表示登录后的默认展示小组。
- 后端每次使用前必须校验该用户仍属于该组。
- 超级管理员可以为空；若超级管理员同时加入多个小组，也可设置默认展示小组。

### 2.3 API 变更

认证与用户接口：

- `GET /api/auth/me`：返回 `default_group_id`。
- `POST /api/auth/default-group`
  - 请求：`{ "group_id": 123 }`
  - 权限：账号本人。
  - 校验：`group_id` 必须在当前用户所属小组内。
  - 行为：更新 `users.default_group_id`。

登录流程：

1. 用户输入 `username + password`。
2. 后端认证成功后读取用户所属小组。
3. 若 `default_group_id` 有效，则 token 中写入 `current_group_id=default_group_id`。
4. 若无默认小组且只属于一个小组，则进入唯一小组。
5. 若无默认小组且属于多个小组，则前端展示小组选择页。

### 2.4 前端变更

- 在个人菜单或小组选择器旁增加“设为默认小组”。
- 多小组用户进入小组选择页时展示：
  - 小组名称
  - 当前默认标记
  - “进入”
  - “设为默认”
- 修改默认小组后无需重新登录，刷新当前用户信息即可。

## 3. 旧 JSON 数据迁移 CLI

### 3.1 目标

新增 Go CLI，将旧版本数据导入 MySQL：

- `config.json`
- `data/records.json`
- 后续可扩展导入 `data/app.json`、`data/members.json`、`data/weekly_schedule.json`

建议路径：

```text
backend/cmd/migrate-json/main.go
```

### 3.2 CLI 参数

```bash
go run ./cmd/migrate-json \
  --dsn "$AGP_DSN" \
  --group-code agape-a \
  --group-name "AGAPE A组" \
  --config ../config.json \
  --records ../data/records.json \
  --default-password "Abc12345" \
  --assets-root /volume1/agp-data/assets/agape-a \
  --dry-run=false
```

参数说明：

- `--dsn`：MySQL DSN。
- `--group-code`：新小组编码，必须唯一。
- `--group-name`：新小组名称。
- `--config`：旧 `config.json` 路径。
- `--records`：旧 `records.json` 路径。
- `--default-password`：该组默认密码；未传则自动生成 8 位字母数字。
- `--assets-root`：该组资源根目录。
- `--dry-run`：只解析和统计，不写入数据库。

### 3.3 config.json 映射

导入目标：

- study_groups
- `group_settings`
- `users`
- `group_members`
- `study_weeks`
- `study_tasks`
- `assets`
- `task_assets`

映射规则：

- `site_info` -> `group_settings`
- `members` -> `users` + `group_members`
- `weekly_schedule` -> `study_weeks` + `study_tasks`
- `mounted_files` -> 作为资源扫描配置使用，不直接原样入库。
- 书籍、视频、PPT、图片、Markdown 路径 -> `assets`
- 周计划中的 `readings/videos/shares/outlineImage` -> `task_assets`

账号生成规则：

- 优先使用内置用户名映射和标准化规则生成 `username`。
- 如果仍无法得到稳定用户名，CLI 会在迁移报告中输出 `generated_usernames` 供人工确认。

### 3.4 records.json 映射

旧记录字段：

- `name`
- `checkin_time`
- `logical_date`
- `is_retro`
- `daily`
- `book`
- `video`
- `verse`
- `detail`
- `note`
- `kind`
- `part`

映射规则：

- `daily = done` -> `task_type = daily_devotion`
- `book = done` -> `task_type = weekly_book`
- `video = done` -> `task_type = weekly_video`
- `verse = done` -> `task_type = weekly_verse`
- `kind = reflection` -> `task_type = reflection`
- `kind = recite_exam` -> `checkin_records.task_type = recite_exam` + `recite_attempts`

导入约束：

- 按 `name` 匹配 `users.display_name` 或 `group_members.member_name`。
- 匹配失败的记录写入失败报告，不直接丢弃。
- 对重复有效打卡记录，按 `uk_one_active_checkin` 处理：
  - 默认跳过重复。
  - 输出重复记录报告。
  - 可加 `--allow-duplicate-as-deleted` 将后续重复记录导入为软删除记录。

### 3.5 迁移报告

CLI 执行结束输出：

- 小组创建/复用情况。
- 成员总数、成功数、失败数。
- 周计划成功数、失败数。
- 资源成功数、失败数。
- 打卡记录成功数、跳过重复数、失败数。
- 失败报告文件路径。

建议输出文件：

```text
data/migration-reports/agape-a-2026xxxx.json
```

## 4. 管理台扩展

当前前端已有基础页面：首页、打卡、资源、管理入口。本轮将管理台拆成三个优先模块。

### 4.1 周计划编辑

功能：

- 周计划列表。
- 新增/编辑/删除周计划。
- 设置开始日期、结束日期、标题、背经、默写文本。
- 开关：读物、视频、背经、提纲。
- 绑定资源：读物 PDF、视频、分享资料、提纲图片。

API：

- `GET /api/study-weeks`
- `POST /api/admin/study-weeks`
- `PUT /api/admin/study-weeks/{id}`
- `DELETE /api/admin/study-weeks/{id}`
- `POST /api/admin/tasks/{id}/assets`
- `DELETE /api/admin/tasks/{task_id}/assets/{asset_id}`

前端要求：

- 电脑端使用表格 + 抽屉表单。
- 手机端使用卡片列表 + 全屏表单。
- 保存时只提交真实变化字段，减少审计噪音。

### 4.2 资源上传

功能：

- 上传 PDF、视频、图片、Markdown。
- 自动识别文件类型和大小。
- 计算 `checksum_sha256`。
- 写入 `assets` 元数据。
- 支持按小组目录保存。

NAS 路径建议：

```text
${AGP_ASSETS_ROOT}/
  {group_code}/
    book/
    ppt/
    video/
    mentor/
    outline/
    markdown/
```

API：

- `POST /api/admin/assets/upload`
  - `multipart/form-data`
  - 字段：`category`、`file`、`title`
- `GET /api/assets?category=&keyword=&page=`
- `GET /api/assets/{id}/download`

权限：

- `group_leader`：可上传、编辑、删除本组资源。
- `group_admin`：可上传和编辑本组资源，是否允许删除可配置。
- `super_admin`：可管理所有小组资源。

安全要求：

- 后端必须校验扩展名和 MIME。
- 文件名只作为展示，实际存储使用安全文件名或 hash 文件名。
- 下载必须校验用户属于该资源所在小组。
- 视频下载支持 HTTP Range。

### 4.3 统计图表

功能：

- 今日完成情况。
- 本周完成率。
- 成员维度统计。
- 任务类型维度统计。
- 连续灵修天数。
- 背经/默写排行榜。

API：

- `GET /api/dashboard/summary?from=&to=`
- `GET /api/dashboard/member-rank?from=&to=&task_type=`
- `GET /api/dashboard/streaks`
- `GET /api/recite/leaderboard?week_id=`

前端实现建议：

- 首期可使用原生 SVG 或轻量图表库。
- 图表数据由后端 SQL 聚合返回，前端不拉全量打卡记录计算。
- 手机端显示简化卡片和横向滚动榜单。
- 电脑端显示柱状图、折线图和成员矩阵。

## 5. NAS 路径与备份

### 5.1 AGP_ASSETS_ROOT 调整

当前默认：

```text
/data/agp/assets
```

NAS 部署建议根据实际卷路径调整，例如：

```bash
export AGP_ASSETS_ROOT=/volume1/agp-data/assets
```

Docker Compose volume 示例：

```yaml
backend:
  environment:
    AGP_ASSETS_ROOT: /data/agp/assets
  volumes:
    - /volume1/agp-data/assets:/data/agp/assets
```

### 5.2 备份范围

必须纳入 NAS 备份：

- MySQL 数据目录：

```text
/volume1/agp-data/mysql
```

- 资源目录：

```text
/volume1/agp-data/assets
```

- 迁移报告：

```text
/volume1/agp-data/migration-reports
```

### 5.3 备份策略

建议：

- MySQL：每日逻辑备份 `mysqldump` + NAS 快照。
- assets：NAS 文件快照。
- 保留策略：
  - 每日备份保留 14 天。
  - 每周备份保留 8 周。
  - 每月备份保留 12 个月。
- 每月至少做一次恢复演练，验证 MySQL + assets 能恢复到同一时间点。

示例备份命令：

```bash
docker exec agp-mysql mysqldump -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" agp > /volume1/agp-data/backups/mysql/agp-$(date +%F).sql
rsync -a --delete /volume1/agp-data/assets/ /volume1/agp-data/backups/assets-latest/
```

## 6. 实施顺序

建议按以下顺序推进：

1. 已完成：数据库迁移，增加 `users.default_group_id`，小组表使用 `study_groups`。
2. 已完成：后端实现默认小组 API 与登录后默认小组选择逻辑。
3. 已完成：前端增加默认小组设置入口和多小组选择页。
4. 已完成：后端实现 `migrate-json` CLI 的 dry-run 和报告输出。
5. 已完成：后端实现真实导入 `config.json`。
6. 已完成：后端实现真实导入 `records.json`。
7. 前端：实现周计划编辑。
8. 后端 + 前端：实现资源上传。
9. 后端 + 前端：实现统计图表。
10. 部署：调整 `AGP_ASSETS_ROOT` 到 NAS 实际路径，并配置备份。

## 7. 验收标准

### 默认小组

- 多小组成员可设置默认小组。
- 默认小组有效时登录后自动进入。
- 默认小组无效时回退到小组选择。
- 未认证用户无法通过用户名获取所属小组列表。

### 迁移 CLI

- dry-run 不写数据库。
- 导入后成员数、周计划数、记录数与旧 JSON 对齐。
- 失败记录有报告。
- 重复打卡处理符合预期。

### 管理台

- 电脑端可以完成周计划编辑、资源上传和统计查看。
- 手机端至少可以查看和进行基础编辑。
- 周计划保存只更新真实变化字段。

### 备份

- `AGP_ASSETS_ROOT` 指向 NAS 实际资源目录。
- `/data/agp/assets` 对应的宿主机目录进入 NAS 备份。
- MySQL 和 assets 可完成一次恢复演练。
