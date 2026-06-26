# Cedar Discipleship 运维命令速查

本文档面向当前 Go + MySQL + 前端分离版本，对应部署文件：

```bash
deploy/docker-compose.separated.yml
```

新环境一键部署与分组迁移脚本见：

```text
scripts/deploy-oneclick.sh
scripts/migrate-group.sh
docs/deploy-new-environment.md
docs/migrate-other-groups.md
```

## 启动与停止

在项目根目录执行：

```bash
cd /Users/bytedance/program/agp

# 构建并启动 MySQL、后端、前端
docker compose -f deploy/docker-compose.separated.yml up -d --build

# 查看运行状态
docker compose -f deploy/docker-compose.separated.yml ps

# 查看全部服务日志
docker compose -f deploy/docker-compose.separated.yml logs -f

# 只查看后端日志
docker compose -f deploy/docker-compose.separated.yml logs -f backend

# 只查看 MySQL 日志
docker compose -f deploy/docker-compose.separated.yml logs -f mysql

# 重启后端
docker compose -f deploy/docker-compose.separated.yml restart backend

# 停止服务，保留数据卷目录 data/mysql 和 data/assets
docker compose -f deploy/docker-compose.separated.yml down
```

如果在 NAS 或受限网络环境中卡在 `backend` 的 `go mod download`，先设置：

```bash
export GOPROXY='https://goproxy.cn,direct'

# 只有在模块校验服务不可达时再加
# export GOSUMDB='off'
```

然后重新执行 `docker compose ... up -d --build`。

默认访问地址：

```text
http://127.0.0.1:5114
```

可通过环境变量覆盖前端端口：

```bash
AGP_WEB_PORT=8088 docker compose -f deploy/docker-compose.separated.yml up -d --build
```

## MySQL 连接

默认配置来自 `deploy/docker-compose.separated.yml`：

```text
数据库: agp
用户: agp
密码: agp
容器: agp-mysql
宿主机端口: 127.0.0.1:3307
```

小组表名为 `study_groups`。

进入 MySQL：

```bash
docker exec -it agp-mysql mysql -uagp -pagp agp
```

使用 root 进入：

```bash
docker exec -it agp-mysql mysql -uroot -pagp-root agp
```

在宿主机直接执行一条 SQL：

```bash
docker exec -i agp-mysql mysql -uagp -pagp agp -e "SHOW TABLES;"
```

导出数据库备份：

```bash
mkdir -p data/backups/mysql
docker exec agp-mysql mysqldump -uagp -pagp agp > data/backups/mysql/agp-$(date +%F).sql
```

## MySQL 启动故障排查

如果日志里出现下面这类错误：

```text
Different lower_case_table_names settings for server ('0') and data dictionary ('2')
Data Dictionary initialization failed
```

说明当前挂载的 `data/mysql` 不是在这个环境里初始化的。常见场景是把在 macOS 或大小写不敏感文件系统上生成的 MySQL 数据目录，直接拷到 Linux/NAS 上继续挂载。

处理原则：

1. 不要跨操作系统直接复用 MySQL 原始数据目录。
2. 新环境空库部署时，直接备份并删除或重命名目标机器上的 `data/mysql`，让容器重新初始化。
3. 需要保留历史数据时，先在原环境导出逻辑备份，再在新环境导入。

示例：

```bash
# 原环境导出
mkdir -p data/backups/mysql
docker exec agp-mysql mysqldump -uagp -pagp agp > data/backups/mysql/agp-$(date +%F).sql

# 新环境初始化空数据目录后导入
cat data/backups/mysql/agp-$(date +%F).sql | docker exec -i agp-mysql mysql -uagp -pagp agp
```

## 查询指定小组数据

以下 SQL 可在 MySQL 客户端中执行。先按小组编码或名称设置变量：

```sql
-- 推荐使用小组编码，例：agape-a
SET @group_code = '替换为小组编码';
SET @group_id = (
  SELECT id
  FROM study_groups
  WHERE code = @group_code
  LIMIT 1
);

SELECT @group_id AS group_id;
```

如果只知道小组名称：

```sql
SET @group_name = '替换为小组名称';
SET @group_id = (
  SELECT id
  FROM study_groups
  WHERE name = @group_name
  LIMIT 1
);

SELECT @group_id AS group_id;
```

查看小组基本信息：

```sql
SELECT id, code, name, description, status, created_at, updated_at
FROM study_groups
WHERE id = @group_id;
```

查看小组成员和角色：

```sql
SELECT
  m.id AS member_id,
  u.id AS user_id,
  u.username,
  u.display_name,
  m.member_name,
  u.is_super_admin,
  u.status AS user_status,
  GROUP_CONCAT(r.role ORDER BY r.role SEPARATOR ',') AS roles,
  m.joined_at
FROM group_members m
JOIN users u ON u.id = m.user_id
LEFT JOIN user_group_roles r
  ON r.group_id = m.group_id AND r.user_id = m.user_id
WHERE m.group_id = @group_id
  AND m.status = 1
GROUP BY m.id, u.id, u.username, u.display_name, m.member_name, u.is_super_admin, u.status, m.joined_at
ORDER BY m.id;
```

查看指定小组最近打卡记录：

```sql
SELECT
  cr.id,
  cr.logical_date,
  cr.checkin_time,
  u.username,
  u.display_name,
  cr.task_type,
  cr.status,
  cr.is_retro,
  cr.detail,
  cr.part,
  cr.deleted_at
FROM checkin_records cr
JOIN users u ON u.id = cr.user_id
WHERE cr.group_id = @group_id
  AND cr.deleted_at IS NULL
ORDER BY cr.logical_date DESC, cr.checkin_time DESC
LIMIT 100;
```

按日期范围查询打卡记录：

```sql
SET @start_date = '2026-06-01';
SET @end_date = '2026-06-30';

SELECT
  cr.logical_date,
  u.display_name,
  cr.task_type,
  cr.detail,
  cr.checkin_time
FROM checkin_records cr
JOIN users u ON u.id = cr.user_id
WHERE cr.group_id = @group_id
  AND cr.logical_date BETWEEN @start_date AND @end_date
  AND cr.deleted_at IS NULL
ORDER BY cr.logical_date, u.display_name, cr.task_type;
```

统计每日打卡数量：

```sql
SELECT
  logical_date,
  task_type,
  COUNT(*) AS total
FROM checkin_records
WHERE group_id = @group_id
  AND deleted_at IS NULL
GROUP BY logical_date, task_type
ORDER BY logical_date DESC, task_type;
```

统计成员维度打卡数量：

```sql
SELECT
  u.display_name,
  cr.task_type,
  COUNT(*) AS total
FROM checkin_records cr
JOIN users u ON u.id = cr.user_id
WHERE cr.group_id = @group_id
  AND cr.deleted_at IS NULL
GROUP BY u.id, u.display_name, cr.task_type
ORDER BY u.display_name, cr.task_type;
```

查看周计划：

```sql
SELECT
  id,
  start_date,
  end_date,
  title,
  verse_ref,
  book_enabled,
  video_enabled,
  verse_enabled,
  outline_enabled,
  sort_order,
  created_at,
  updated_at
FROM study_weeks
WHERE group_id = @group_id
ORDER BY start_date DESC;
```

查看资源文件：

```sql
SELECT
  id,
  category,
  title,
  original_name,
  storage_path,
  mime_type,
  file_size,
  created_at
FROM assets
WHERE group_id = @group_id
ORDER BY category, id DESC;
```

查看管理操作审计：

```sql
SELECT
  id,
  actor_user_id,
  action,
  target_type,
  target_id,
  ip,
  created_at
FROM audit_logs
WHERE group_id = @group_id
ORDER BY id DESC
LIMIT 100;
```

## 常用定位命令

检查后端健康：

```bash
curl -s http://127.0.0.1:5114/api/health
```

查看后端实际环境变量：

```bash
docker exec agp-backend env | grep '^AGP_'
```

查看 MySQL 表：

```bash
docker exec -i agp-mysql mysql -uagp -pagp agp -e "SHOW TABLES;"
```

查看打卡表分区：

```bash
docker exec -i agp-mysql mysql -uagp -pagp agp -e "
SELECT PARTITION_NAME, PARTITION_DESCRIPTION, TABLE_ROWS
FROM information_schema.PARTITIONS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'checkin_records'
ORDER BY PARTITION_ORDINAL_POSITION;
"
```

## 旧 JSON 数据迁移

迁移工具路径：

```text
backend/cmd/migrate-json
```

先执行 dry-run，只解析 `config.json` 和 `data/records.json`，不写数据库：

```bash
cd /Users/bytedance/program/agp/backend

go run ./cmd/migrate-json \
  --group-code agape-a \
  --group-name "AGAPE A组" \
  --config ../config.json \
  --records ../data/records.json \
  --default-password "Abc12345" \
  --report-dir ../data/migration-reports \
  --dry-run=true
```

确认报告后执行真实导入：

```bash
cd /Users/bytedance/program/agp/backend

go run ./cmd/migrate-json \
  --dsn "agp:agp@tcp(127.0.0.1:3307)/agp?parseTime=true&multiStatements=false&charset=utf8mb4,utf8" \
  --group-code agape-a \
  --group-name "AGAPE A组" \
  --config ../config.json \
  --records ../data/records.json \
  --default-password "Abc12345" \
  --report-dir ../data/migration-reports \
  --dry-run=false
```

如果在 Docker Compose 网络内执行，DSN 中 MySQL 地址应使用服务名：

```text
agp:agp@tcp(mysql:3306)/agp?parseTime=true&multiStatements=false&charset=utf8mb4,utf8
```

迁移器会优先使用内置用户名映射；未命中时会自动生成用户名，并把结果写到迁移报告里的 `generated_usernames`。

迁移报告会输出到：

```text
data/migration-reports/
```

当前导入规则：

- `config.json` 导入 `study_groups`、`group_settings`、`users`、`group_members`、`user_group_roles`、`study_weeks`、`study_tasks`、`assets`、`task_assets`。
- `records.json` 导入 `checkin_records`。
- `daily=done` -> `daily_devotion`。
- `book=done` -> `weekly_book`。
- `video=done` -> `weekly_video`。
- `verse=done` -> `weekly_verse`。
- `kind=reflection` -> `reflection`。
- `kind=recite_exam` -> `recite_exam`。
- 重复有效打卡默认跳过；如需保留为软删除记录，增加 `--allow-duplicate-as-deleted=true`。
