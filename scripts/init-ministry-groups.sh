#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/deploy/docker-compose.separated.yml}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-$(docker inspect agp-mysql --format '{{ index .Config.Labels "com.docker.compose.project" }}' 2>/dev/null || true)}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-agp}"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
SQL_FILE="${SQL_FILE:-$ROOT_DIR/backend/sql/init_ministry_groups.sql}"
USE_LOCAL_MYSQL="${USE_LOCAL_MYSQL:-false}"

env_file_value() {
  local env_name="$1"
  [ -f "$ENV_FILE" ] || return 0
  (
    set +u
    set -a
    # shellcheck disable=SC1090
    . "$ENV_FILE"
    printf '%s' "${!env_name:-}"
  )
}

for env_name in MYSQL_HOST MYSQL_PORT MYSQL_DATABASE MYSQL_USER MYSQL_PASSWORD MYSQL_ROOT_PASSWORD; do
  if [ -z "${!env_name:-}" ]; then
    printf -v "$env_name" '%s' "$(env_file_value "$env_name")"
  fi
done

MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-3307}"
MYSQL_DATABASE="${MYSQL_DATABASE:-agp}"
MYSQL_USER="${MYSQL_USER:-agp}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-agp}"
MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-agp-root}"

usage() {
  cat <<EOF
用法:
  ./scripts/init-ministry-groups.sh

默认通过 Docker Compose 的 mysql 服务执行:
  COMPOSE_FILE=deploy/docker-compose.separated.yml
  COMPOSE_PROJECT_NAME=agp

如需直连本机或远端 MySQL:
  USE_LOCAL_MYSQL=true \\
  MYSQL_HOST=127.0.0.1 \\
  MYSQL_PORT=3307 \\
  MYSQL_DATABASE=agp \\
  MYSQL_USER=agp \\
  MYSQL_PASSWORD=agp \\
  MYSQL_ROOT_PASSWORD=agp-root \\
  ./scripts/init-ministry-groups.sh

可覆盖 SQL 文件:
  SQL_FILE=backend/sql/init_ministry_groups.sql ./scripts/init-ministry-groups.sh
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少命令: $1" >&2
    exit 1
  fi
}

compose() {
  local args=()
  if [ -f "$ENV_FILE" ]; then
    args+=(--env-file "$ENV_FILE")
  fi
  docker compose "${args[@]}" -p "$COMPOSE_PROJECT_NAME" -f "$COMPOSE_FILE" "$@"
}

run_with_compose() {
  require_cmd docker
  compose ps mysql >/dev/null
  if compose exec -T -e MYSQL_PWD="$MYSQL_PASSWORD" mysql \
    mysqladmin ping -h 127.0.0.1 -u"$MYSQL_USER" >/dev/null 2>&1; then
    compose exec -T -e MYSQL_PWD="$MYSQL_PASSWORD" mysql \
      mysql -h 127.0.0.1 -u"$MYSQL_USER" "$MYSQL_DATABASE" < "$SQL_FILE"
    return
  fi

  echo "应用数据库账号连接失败，尝试使用 root 账号执行 SQL..." >&2
  compose exec -T -e MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql \
    mysqladmin ping -h 127.0.0.1 -uroot >/dev/null
  compose exec -T -e MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql \
    mysql -h 127.0.0.1 -uroot "$MYSQL_DATABASE" < "$SQL_FILE"
}

run_with_local_mysql() {
  require_cmd mysql
  MYSQL_PWD="$MYSQL_PASSWORD" mysql \
    -h "$MYSQL_HOST" \
    -P "$MYSQL_PORT" \
    -u "$MYSQL_USER" \
    "$MYSQL_DATABASE" < "$SQL_FILE"
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

if [ ! -f "$SQL_FILE" ]; then
  echo "SQL 文件不存在: $SQL_FILE" >&2
  exit 1
fi

echo ">>> 初始化专项小组"
echo "SQL: $SQL_FILE"
echo "DB:  $MYSQL_DATABASE"

if [ "$USE_LOCAL_MYSQL" = "true" ]; then
  echo "连接: ${MYSQL_HOST}:${MYSQL_PORT}"
  run_with_local_mysql
else
  echo "连接: docker compose project=${COMPOSE_PROJECT_NAME}, service=mysql"
  run_with_compose
fi

cat <<EOF

专项小组初始化完成。

已确保以下组存在于每个 study_group 下：
  领会组、主持组、伙食组、后勤组、整洁组、技术组、策划组、数点组、
  探望组、回报组、娃娃组、守望组、门训数点组、门训规划发布组、门训批改组

该 SQL 可重复执行。
EOF
