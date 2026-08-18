#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/deploy/docker-compose.separated.yml}"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
SQL_FILE="${SQL_FILE:-$ROOT_DIR/backend/sql/init_ministry_groups.sql}"
SCHEMA_FILE="${SCHEMA_FILE:-$ROOT_DIR/backend/migrations/003_ministry_groups.sql}"
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

for env_name in COMPOSE_PROJECT_NAME AGP_CONTAINER_PREFIX MYSQL_HOST MYSQL_PORT MYSQL_DATABASE MYSQL_USER MYSQL_PASSWORD MYSQL_ROOT_PASSWORD; do
  if [ -z "${!env_name:-}" ]; then
    printf -v "$env_name" '%s' "$(env_file_value "$env_name")"
  fi
done

AGP_CONTAINER_PREFIX="${AGP_CONTAINER_PREFIX:-agp}"
MYSQL_CONTAINER_NAME="${MYSQL_CONTAINER_NAME:-${AGP_CONTAINER_PREFIX}-mysql}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-$(docker inspect "$MYSQL_CONTAINER_NAME" --format '{{ index .Config.Labels "com.docker.compose.project" }}' 2>/dev/null || true)}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-$AGP_CONTAINER_PREFIX}"
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
  AGP_CONTAINER_PREFIX=agp

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

可覆盖建表 SQL 文件:
  SCHEMA_FILE=backend/migrations/003_ministry_groups.sql ./scripts/init-ministry-groups.sh
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

container_env_value() {
  local env_name="$1"
  docker inspect "$MYSQL_CONTAINER_NAME" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
    | awk -v key="$env_name" 'index($0, key "=") == 1 { print substr($0, length(key) + 2); exit }'
}

probe_compose_login() {
  local user="$1"
  local password="$2"
  local database="$3"

  compose exec -T -e MYSQL_PWD="$password" mysql \
    mysql -h 127.0.0.1 -u"$user" "$database" -e "SELECT 1" >/dev/null 2>&1
}

run_compose_mysql_file() {
  local user="$1"
  local password="$2"
  local database="$3"
  local file="$4"

  compose exec -T -e MYSQL_PWD="$password" mysql \
    mysql -h 127.0.0.1 -u"$user" "$database" < "$file"
}

run_compose_init() {
  local user="$1"
  local password="$2"
  local database="$3"

  if ! probe_compose_login "$user" "$password" "$database"; then
    return 1
  fi

  if [ -f "$SCHEMA_FILE" ]; then
    echo "执行建表 SQL: $SCHEMA_FILE"
    if ! run_compose_mysql_file "$user" "$password" "$database" "$SCHEMA_FILE"; then
      return 2
    fi
  fi

  if ! run_compose_mysql_file "$user" "$password" "$database" "$SQL_FILE"; then
    return 2
  fi

  return 0
}

try_compose_init() {
  local status

  run_compose_init "$@"
  status=$?
  if [ "$status" -eq 0 ]; then
    return 0
  fi
  if [ "$status" -ne 1 ]; then
    echo "SQL 执行失败，专项小组初始化未完成。" >&2
    exit "$status"
  fi

  return 1
}

run_with_compose() {
  require_cmd docker
  compose ps mysql >/dev/null

  if try_compose_init "$MYSQL_USER" "$MYSQL_PASSWORD" "$MYSQL_DATABASE"; then
    return
  fi

  local container_user
  local container_password
  container_user="$(container_env_value MYSQL_USER)"
  container_password="$(container_env_value MYSQL_PASSWORD)"
  if [ -n "$container_user" ] && [ -n "$container_password" ]; then
    echo "当前应用数据库账号连接失败，尝试使用运行中 MySQL 容器的应用账号配置..." >&2
    if try_compose_init "$container_user" "$container_password" "$MYSQL_DATABASE"; then
      return
    fi
  fi

  echo "应用数据库账号连接失败，尝试使用 root 账号执行 SQL..." >&2
  if try_compose_init root "$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"; then
    return
  fi

  local container_root_password
  container_root_password="$(container_env_value MYSQL_ROOT_PASSWORD)"
  if [ -n "$container_root_password" ]; then
    echo "当前 root 密码连接失败，尝试使用运行中 MySQL 容器的 root 配置..." >&2
    if try_compose_init root "$container_root_password" "$MYSQL_DATABASE"; then
      return
    fi
  fi

  echo "数据库账号连接失败。请确认 MYSQL_PASSWORD 或 MYSQL_ROOT_PASSWORD 与现有 MySQL 数据卷一致。" >&2
  exit 1
}

run_with_local_mysql() {
  require_cmd mysql
  if [ -f "$SCHEMA_FILE" ]; then
    echo "执行建表 SQL: $SCHEMA_FILE"
    MYSQL_PWD="$MYSQL_PASSWORD" mysql \
      -h "$MYSQL_HOST" \
      -P "$MYSQL_PORT" \
      -u "$MYSQL_USER" \
      "$MYSQL_DATABASE" < "$SCHEMA_FILE"
  fi

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
echo "Schema: $SCHEMA_FILE"
echo "DB:  $MYSQL_DATABASE"

if [ ! -f "$SCHEMA_FILE" ]; then
  echo "建表 SQL 文件不存在，将仅执行初始化 SQL: $SCHEMA_FILE" >&2
fi

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
