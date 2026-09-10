# Cedar Discipleship

> A paper-inspired discipleship workspace for study planning, resource curation, and daily check-ins
>
> 建议 GitHub 仓库名：`cedar-discipleship`

这是一个采用纸感知识库风格的小组研修与打卡平台。把学习计划、内容查看、打卡记录、统计看板和多小组管理收在同一个 Web 应用里，适合门训、课程共学、读书小组等需要“持续学习 + 过程记录”的场景。当前主版本已经升级为前后端分离架构：

- 后端：Go
- 数据库：MySQL 8.0
- 前端：Vue 3 + Vite + Pinia
- 部署：Docker Compose

当前版本支持多小组隔离、按组学习内容配置、成员与权限管理、每日/周任务打卡、统计看板、资源库与旧数据迁移。

## 功能概览

- 多小组隔离：成员、打卡、周任务、资源按 `group_id` 隔离
- 权限体系：超级管理员、组长、小组管理员、普通成员
- 打卡工作台：首页展示当天学习任务、日期切换、回到今天与个人打卡记录
- 统计中心：小组完成率、成员矩阵、本月累计排行，并支持导出柱状图 PNG
- 学习内容管理：按组配置每日内容、周任务、视频、读物、背经、提纲图
- 资源库：资源按学习小组独立存储，支持跨组授权、逻辑导入、依赖图谱和导入历史
- 内容查看器：统一预览 Markdown / PDF / 视频 / 图片，并支持同主题资料“上一篇 / 下一篇”连续浏览
- 历史迁移：支持把旧 `config.json` 和 `records.json` 导入 MySQL 平台

## 开源边界

- 本仓库公开的是应用代码、部署配置和迁移脚本，不包含生产数据。
- `.env`、本地数据库目录、备份文件和上传文件均应保留在部署环境，不进入版本控制。
- `config.json` 与 `data/records.json` 仅作为历史数据迁移输入，不应视为公开示例数据集。

## 目录结构

```text
.
├── backend/                     # Go 后端
│   ├── cmd/server/main.go
│   ├── cmd/migrate-json/main.go
│   ├── migrations/
│   └── Dockerfile
├── frontend/                    # Vue 3 + Vite + Pinia 前端
│   ├── src/main.js
│   ├── src/App.vue
│   ├── src/stores/
│   ├── src/legacy-app.js        # 当前前端业务运行时与状态桥接层
│   ├── src/styles.css
│   ├── package.json
│   ├── vite.config.js
│   ├── nginx.conf
│   └── Dockerfile
├── deploy/
│   └── docker-compose.separated.yml
├── scripts/
│   ├── init-deploy-env.sh      # 生成本地部署 .env
│   ├── deploy-oneclick.sh       # 新环境一键部署
│   ├── migrate-group.sh         # 底层旧 JSON 迁移入口
│   └── migrate-legacy-project.sh # 旧独立项目一站式迁移入口
├── docs/
│   ├── ops-commands.md
│   ├── deploy-new-environment.md
│   ├── migrate-other-groups.md
│   └── implementation-notes.md
├── data/
│   ├── mysql/
│   ├── resources/
│   └── backups/mysql/
└── config.json                  # 旧数据迁移输入之一
```

## 快速开始

### 1. 直接启动当前平台

在项目根目录执行：

```bash
./scripts/init-deploy-env.sh
docker compose --env-file .env -f deploy/docker-compose.separated.yml up -d --build
```

默认访问地址：

```text
http://127.0.0.1:5114
```

前端容器默认监听宿主机 `0.0.0.0:${AGP_WEB_PORT:-5114}`，在局域网或服务器环境中也可以通过 `http://<宿主机IP>:5114` 访问。

默认 MySQL 端口：

```bash
./scripts/init-deploy-env.sh

docker compose --env-file .env -f deploy/docker-compose.separated.yml up -d --build
```

脚本会补齐 `.env` 中缺失的部署变量和随机密钥，不覆盖已存在的值，并创建 `AGP_DATA_DIR` 下的 `mysql`、`resources`、`backups/mysql` 目录。`AGP_RESOURCE_ROOT` 保存按组隔离的不可变资源，目录格式为 `team-{group_code}-resources/objects/{resource_key}/{filename}`；资源上传后默认对所有学习小组开放导入。

首次超级管理员由环境变量创建。已运行 `./scripts/init-deploy-env.sh` 时，所需变量会写入 `.env`；未运行该脚本而直接使用 Docker Compose 启动时，必须手动提供：

```bash
export AGP_JWT_SECRET='替换为长随机字符串'
export BOOTSTRAP_SUPERADMIN_USERNAME='admin'
export BOOTSTRAP_SUPERADMIN_PASSWORD='替换为强密码'
export BOOTSTRAP_SUPERADMIN_DISPLAY_NAME='超级管理员'

# 可选：默认空值表示登录令牌永久有效；例如 24h 表示 24 小时过期
export AGP_TOKEN_TTL=''
```

如果部署机器无法访问 `proxy.golang.org` 或 `registry.npmjs.org`，镜像构建会在依赖下载阶段超时。NAS 或受限网络环境里，先设置 Go 模块代理和 npm registry 再执行部署：

这些变量会透传到 `backend`/`frontend` 镜像构建，以及迁移脚本内部启动的 `golang:1.25-bookworm` 容器。

### 2. 本地检查

```bash
cd backend
go test ./...

cd ..
cd frontend
npm install
npm run build

cd ..
docker compose -f deploy/docker-compose.separated.yml config
```

## 新环境一键部署

如果你要在一台新的服务器、NAS 或 Docker 主机上直接部署：

```bash
./scripts/deploy-oneclick.sh
```

这个脚本会：

1. 初始化 `data/mysql`、`data/resources`、`data/backups/mysql`
2. 启动 `mysql / backend / frontend`
3. 等待 MySQL 就绪
4. 可选执行首个小组 JSON 数据迁移
5. 可选执行资源文件迁移，将数据库中该组旧资源路径对应的文件复制到 `data/resources`

## 旧数据迁移

### 首次部署时迁移首个小组

```bash
export PRIMARY_GROUP_CODE='agape-a'
export PRIMARY_GROUP_NAME='AGAPE A组'
export PRIMARY_GROUP_DEFAULT_PASSWORD='Abc12345'
export PRIMARY_CONFIG_PATH='/absolute/path/to/config.json'
export PRIMARY_RECORDS_PATH='/absolute/path/to/records.json'
export RESOURCE_MIGRATION_GROUP_NAME='AGAPE A组'
export RESOURCE_LEGACY_ROOT='/absolute/path/to/old-resource-root'

./scripts/deploy-oneclick.sh
```

资源文件迁移按 `RESOURCE_MIGRATION_GROUP_NAME` 查询数据库中的 `study_groups.name`。文件目标目录使用查到的 `study_groups.code`，最终路径为 `data/resources/team-{group_code}-resources/objects/{resource_key}/{filename}`。如果 `RESOURCE_MIGRATION_GROUP_NAME` 未设置，会使用 `PRIMARY_GROUP_NAME`；脚本不写死任何小组名称。

### 已上线后继续迁移其他组

旧独立项目目录迁移使用一站式入口。它会读取旧项目下的 `config.json` 和 `data/records.json`，正式导入后迁移本组独有资料文件；已由其他小组共享的同名同类资源会优先复用，不重复复制文件。

```bash
SOURCE_PROJECT_DIR='/volume1/docker/zw1-checkin' \
GROUP_CODE='zw1' \
GROUP_NAME='ZW1小组' \
GROUP_DEFAULT_PASSWORD='Abc12345' \
EXECUTE_IMPORT=false \
./scripts/migrate-legacy-project.sh
```

确认 dry-run 报告后，将 `EXECUTE_IMPORT=true` 重新执行。`GROUP_CODE` 是迁移和资源路径使用的内部稳定标识；管理后台只展示和维护小组名称。

## 跨组资源治理

资源上传后不可修改；内容变化时上传为新的独立资源。新资源默认对所有学习小组共享；跨组导入只建立数据库逻辑引用，不复制文件。资料库只展示当前学习小组已上传或已导入的数据库资源。资源治理页支持事务性的批量权限设置、批量删除和批量导入，并记录聚合审计日志。

详细权限、目录和访问规范见 [资源治理规范](docs/resource-governance.md)。

## 专项小组初始化

专项小组是新功能，不涉及旧数据迁移。表结构由后端启动时的数据库建表流程创建；专项小组目录的独立初始化 SQL 在：

```text
backend/sql/init_ministry_groups.sql
```

这份 SQL 会为每个现有 `study_groups` 初始化以下专项小组，且可重复执行：

```text
领会组、主持组、伙食组、后勤组、整洁组、技术组、策划组、数点组、
探望组、回报组、娃娃组、守望组、门训数点组、门训规划发布组、门训批改组
```

Docker Compose 部署环境下一键执行：

```bash
./scripts/init-ministry-groups.sh
```

如果需要直连本机或远端 MySQL：

```bash
USE_LOCAL_MYSQL=true \
MYSQL_HOST=127.0.0.1 \
MYSQL_PORT=3307 \
MYSQL_DATABASE=agp \
MYSQL_USER=agp \
MYSQL_PASSWORD=agp \
./scripts/init-ministry-groups.sh
```

如果现有 MySQL 数据卷里的应用账号密码与当前配置不一致，脚本会尝试读取正在运行的
`agp-mysql` 容器环境变量作为兜底；也可显式提供 root 密码：

## 打卡群通知

使用 Potato 机器人的 `sendTextMessage` 接口。将机器人加入目标群，并在部署环境 `.env` 中设置：

```dotenv
AGP_POTATO_BOT_TOKEN='机器人 Token'
AGP_POTATO_GROUPS='{"1":{"chat_id":12345678,"chat_type":2},"2":{"chat_id":23456789,"chat_type":3}}'
```

`AGP_POTATO_GROUPS` 仅用于首次启动时导入绑定。之后由超级管理员在“管理后台 → 机器人管理”中查看机器人当前加入的群聊，并设置对应学习小组。绑定保存在 `${AGP_NOTIFICATION_DIR}/bindings.json`；一个群聊只能绑定一个学习小组，一个学习小组可绑定多个群聊。Token 仅保存在服务端环境中。

重新构建并启动后端：

```bash
docker compose --env-file .env -f deploy/docker-compose.separated.yml up -d --build --no-deps backend
```

通知规则：

- 管理员可在“学习内容管理”中分别启用或关闭每日灵修通知、周任务通知；未设置时默认启用。
- 首次启用小组与群聊绑定时，自动分两条发送接入时的进展：每日灵修、本周任务。初始汇总不标 `【新】`；无人打卡时显示“暂无打卡记录”。
- 每个群聊按每日灵修、周任务分别保存最后成功发送状态；重启不重发，变更到新群时向新群发送两条汇总。
- 通知重新启用时，系统按发送时的最新数据库数据分别生成两类汇总。差异仅用于决定发送灵修、周任务或两者；消息正文始终是今日灵修全量或本周任务全量，不是新增部分。比较时忽略临时 `【新】` 标记。
- 每日灵修汇总当天已打卡成员，周任务汇总本周书籍和视频；标题独占一行，每人一行。
- 汇总包含机器人接入前的有效记录。例如接入前已有 3 人完成灵修，第 4 人打卡时显示全部 4 人。
- 本周继续使用的同一视频资源，计入以前已完成该视频的成员。
- 每日灵修不显示 `【新】`。周任务仅将触发该通知的最后一次任务项标记 `【新】`，下一次周任务通知中标记移至最新任务项。同一成员的内容合并展示，按首次打卡顺序编号。
- 书籍名称取学习任务 `book_name`，缺省时取任务标题，展示前两个 Unicode 字符；视频展示为 `视频`。
- 使用北京时间；历史日期灵修、往周任务、撤销打卡和重复提交均不发送。本周内补打之前日期的周任务仍发送。
- 每条消息对应一次新增打卡；超过 3500 字节优先在人员行之间分段，失败后从未完成的段继续。

通知示例：

```text
每日灵修
1 张三
2 李四
3 王五
4 赵六
```

```text
本周任务
1 张三 基督 史剧
2 李四 史剧 【新】视频
```

待发、完成、失败记录分别保存在 `${AGP_DATA_DIR}/notifications/{pending,completed,failed}`，每群每类别最后成功发送状态保存在 `${AGP_DATA_DIR}/notifications/last-sent`。Compose 默认路径为 `data/notifications`。每个队列目录由一个后端实例使用；备份和清理时按服务数据管理，其中包含群内通知正文。完成、失败记录保留用于排查，可按运维留存周期归档。直接运行 Go 后端时可通过 `AGP_NOTIFICATION_DIR` 设置队列路径，保持独立于公开资源目录。

升级时会从现有 `completed` 记录自动建立 `last-sent` 状态，避免重复发送历史通知。

发送超时为 10 秒，队列每秒最多发送一段。网络错误、HTTP 429/5xx、Potato 1001/1007/4048 最多尝试 5 次，按 10/20/40/80 秒退避并遵守 `Retry-After`。永久错误进入 `failed`；超过当天或本周有效期的消息归档为 `skipped`。重试使用已保存的正文；周任务保留已冻结的【新】标记。

查看发送日志：

```bash
docker compose --env-file .env -f deploy/docker-compose.separated.yml logs backend | grep 'checkin notification'
```

日志包含打卡 ID、小组 ID、尝试次数、分段进度、结果、耗时和错误分类，省略 Token 与正文。入队失败仍保留打卡成功结果，并记录 `enqueue failed`。MySQL 提交与文件入队之间存在进程崩溃丢通知的窗口；机器人接口未提供幂等键，发送成功但响应丢失时重试可能重复通知。

上线验证：完成一次当天灵修和一次本周书籍／视频打卡，检查对应群消息及 `status=sent` 日志；再补录历史灵修和往周任务，确认没有群消息。

## License

This project is licensed under the MIT License. See [LICENSE](file:///Users/bytedance/program/agp/LICENSE).
