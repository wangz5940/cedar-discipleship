# 旧独立小组项目迁移

本流程用于把 `zw1-checkin` 这类旧独立项目迁入当前 `cedar-discipleship` 平台，并创建为新的学习小组。

## 适用输入

旧项目目录需包含：

```text
zw1-checkin/
  ├── config.json
  ├── data/records.json
  ├── Passage/
  ├── PPT/
  ├── MP3/
  └── MP4/
```

## 迁移原则

- 每个旧项目按 `GROUP_CODE` 迁入一个学习小组；编码不存在时创建，已存在时复用。
- `GROUP_NAME` 是后台展示和维护的小组名称。
- `GROUP_CODE` 是迁移和资源目录使用的内部稳定标识，迁入后后台不再修改。
- `GROUP_CODE` 已存在时复用原小组；可同步展示名称，但不会修改该小组的启停状态。
- 成员只以中文姓名导入；已配置的显式用户名保持不变，其余账号默认按 `{GROUP_CODE}-memberNNN` 生成，避免不同小组或 tenant 复用同一账号。
- 显式用户名只允许在同一 tenant 内复用；关闭自动用户名命名空间时，旧 `memberNNN` 也只能用于目标小组的兼容重跑。
- 周任务、任务资源绑定和历史打卡按小组隔离写入。
- 文件唯一性只由 `SHA-256 + 文件字节长度` 判断；文件名、标题、目录和分类不参与判重。
- 当前小组已有相同指纹时复用当前记录；其他小组已有相同指纹时，只允许从启用小组的有效自有资源导入，且必须存在面向当前小组或所有小组的有效导入授权。
- 其他小组存在相同文件但无有效授权时，迁移以 `cross_group_file_not_importable` 失败，不会复制第二份。
- 系统中不存在相同指纹时，该文件才视为本组独有文件并复制到 `data/resources`。
- NAS 独立数据目录通过 `.env` 的 `AGP_RESOURCE_ROOT` 或 `AGP_DATA_DIR/resources` 定位；如果配置缺失，会自动识别同级 `cedar-discipleship-data/resources`。

## Dry Run

```bash
cd /volume1/docker/cedar-discipleship

SOURCE_PROJECT_DIR=/volume1/docker/zw1-checkin \
GROUP_CODE=zw1 \
GROUP_NAME="ZW1小组" \
GROUP_DEFAULT_PASSWORD='Abc12345' \
EXECUTE_IMPORT=false \
./scripts/migrate-legacy-project.sh
```

检查报告目录：

```text
data/migration-reports/
```

重点确认：

- 小组名称和内部编码正确。
- 成员、周任务、打卡记录数量符合旧项目。
- `warnings` 已确认，`failures` 必须为空。存在失败时 dry-run 返回非零，阻止后续正式迁移。
- 资源文件 dry-run 中的 `imported_files` 和 `would import` 与预期一致。

## 正式迁移

```bash
cd /volume1/docker/cedar-discipleship

SOURCE_PROJECT_DIR=/volume1/docker/zw1-checkin \
GROUP_CODE=zw1 \
GROUP_NAME="ZW1小组" \
GROUP_DEFAULT_PASSWORD='Abc12345' \
EXECUTE_IMPORT=true \
./scripts/migrate-legacy-project.sh
```

正式迁移会依次执行：

1. 数据 dry-run。
2. 写入新学习小组、成员、周任务、资源引用和打卡记录。
3. 资源文件 dry-run。
4. 复制本组独有资料文件。

正式数据导入只在报告无失败时提交；任何失败都会回滚本轮数据库变更，并返回非零。报告中的 `outcome` 区分 `planned`、`blocked`、`rolled_back`、`committed`，计数表示该轮尝试的操作，只有 `committed` 表示已保存。

使用 `--force-overwrite` 替换周配置时，已被历史签到引用的任务会脱离原周并停用，保留任务及资源关联，确保历史记录和同视频跨周完成继承不丢失。

## 可选参数

```bash
ALLOW_DUPLICATE_AS_DELETED=false
FAIL_ON_GENERATED_USERNAMES=false
NAMESPACE_GENERATED_USERNAMES=true
RESOURCE_MIGRATION_DRY_RUN_ONLY=false
RESOURCE_LEGACY_ASSETS_ROOT=/volume1/docker/zw1-checkin/data/assets
```

说明：

- `ALLOW_DUPLICATE_AS_DELETED=true`：重复打卡以软删除历史保留。
- `FAIL_ON_GENERATED_USERNAMES=true`：需要自动生成账号时直接失败。
- `NAMESPACE_GENERATED_USERNAMES=true`：默认使用稳定的 `GROUP_CODE` 隔离自动生成账号；仅兼容旧迁移重跑时可显式设为 `false`。
- `RESOURCE_MIGRATION_DRY_RUN_ONLY=true`：只写入数据，不复制资源文件。
- `RESOURCE_LEGACY_ASSETS_ROOT`：旧项目存在额外上传目录时指定。

`migrate-json --prefer-shared-assets` 仅作为无行为影响的 CLI 兼容参数保留。资源迁移始终在资源文件阶段读取实际文件内容，并按上述指纹规则决定导入或复制。

## 验收清单

正式迁移后逐项确认：

1. 新学习小组出现在后台小组列表。
2. 成员名单、组长和管理员角色正确。
3. 历史打卡按成员、日期、任务类型统计一致。
4. 周任务日期范围、标题、页码和任务开关正确。
5. 历史读物、视频、讲义和提纲能打开。
6. 共享复用资源没有重复文件。
7. 本组独有文件已复制到 `data/resources/team-{group_code}-resources/objects/`。

## 回滚

迁移前导出数据库备份：

```bash
mkdir -p data/backups/mysql
docker exec cedar-mysql mysqldump -uagp -p"$MYSQL_PASSWORD" agp > data/backups/mysql/before-zw1-$(date +%F-%H%M%S).sql
```

迁移结果异常时，停止新的导入，保留迁移报告，使用备份恢复后重新执行。
