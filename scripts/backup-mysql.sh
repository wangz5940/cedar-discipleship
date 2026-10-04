#!/usr/bin/env bash
# 完整 SQL 快照，不替代资源文件备份。目录应位于独立的备份存储上。
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "用法: $0 <MySQL 容器名> <备份目录>" >&2
  exit 2
fi
container="$1"
backup_dir="$2"
umask 077
mkdir -p -- "$backup_dir"
backup_dir="$(cd -- "$backup_dir" && pwd)"
temporary="$(mktemp -d "$backup_dir/.mysql-backup.XXXXXXXX")"
trap 'rm -f -- "$temporary/snapshot.sql" "$temporary/snapshot.sql.gz" "$temporary/checksum"; rmdir -- "$temporary"' EXIT

# 凭据仅在数据库容器内读取，不写入命令参数、日志或备份文件名。
docker exec "$container" sh -c '
  MYSQL_PWD="$MYSQL_PASSWORD" exec mysqldump -u"$MYSQL_USER" \
    --single-transaction --quick --no-tablespaces "$MYSQL_DATABASE"
' > "$temporary/snapshot.sql"
test -s "$temporary/snapshot.sql"
gzip -c "$temporary/snapshot.sql" > "$temporary/snapshot.sql.gz"
gzip -t "$temporary/snapshot.sql.gz"
filename="mysql-$(date -u +%Y%m%dT%H%M%SZ)-${temporary##*.}.sql.gz"
(cd "$temporary" && sha256sum snapshot.sql.gz) | sed "s/snapshot.sql.gz/$filename/" > "$temporary/checksum"
# 确认写盘后再发布文件，失败的导出不会成为可恢复的正式备份。
sync
mv -- "$temporary/snapshot.sql.gz" "$backup_dir/$filename"
mv -- "$temporary/checksum" "$backup_dir/$filename.sha256"
sync
printf '%s\n' "$backup_dir/$filename"
