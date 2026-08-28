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

- 每个旧项目迁入为一个新的学习小组。
- `GROUP_NAME` 是后台展示和维护的小组名称。
- `GROUP_CODE` 是迁移和资源目录使用的内部稳定标识，迁入后后台不再修改。
- 成员只以中文姓名导入，系统生成拼音账号。
- 周任务、任务资源绑定和历史打卡按小组隔离写入。
- 资源迁移优先复用其他小组已共享的同名同类资源。
- 未命中共享资源的本组独有文件复制到 `data/resources`。
- NAS 独立数据目录通过 `.env` 的 `AGP_DATA_DIR` 或 `AGP_RESOURCE_ROOT` 定位。

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
- `warnings` 和 `failures` 为空或已确认。
- `shared_assets` 中列出的资源确实可复用。

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

## 可选参数

```bash
PREFER_SHARED_ASSETS=true
ALLOW_DUPLICATE_AS_DELETED=false
FAIL_ON_GENERATED_USERNAMES=false
RESOURCE_MIGRATION_DRY_RUN_ONLY=false
RESOURCE_LEGACY_ASSETS_ROOT=/volume1/docker/zw1-checkin/data/assets
```

说明：

- `PREFER_SHARED_ASSETS=true`：优先复用其他小组已共享资源。
- `ALLOW_DUPLICATE_AS_DELETED=true`：重复打卡以软删除历史保留。
- `FAIL_ON_GENERATED_USERNAMES=true`：需要自动生成账号时直接失败。
- `RESOURCE_MIGRATION_DRY_RUN_ONLY=true`：只写入数据，不复制资源文件。
- `RESOURCE_LEGACY_ASSETS_ROOT`：旧项目存在额外上传目录时指定。

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
