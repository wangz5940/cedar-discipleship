#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_PROJECT_DIR="${SOURCE_PROJECT_DIR:-}"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/deploy/docker-compose.separated.yml}"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"

usage() {
  cat <<EOF
用法:
  SOURCE_PROJECT_DIR=/volume1/docker/zw1-checkin \\
  GROUP_CODE=zw1 \\
  GROUP_NAME="ZW1小组" \\
  EXECUTE_IMPORT=false \\
  ./scripts/migrate-legacy-project.sh

说明:
  默认 dry-run，不写数据库。
  默认 PREFER_SHARED_ASSETS=true，会优先复用其他小组已共享的同名同类资源。
  EXECUTE_IMPORT=true 时，会在数据导入后迁移本小组独有资料文件。
EOF
}

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

for env_name in \
  MYSQL_DATABASE MYSQL_USER MYSQL_PASSWORD \
  COMPOSE_PROJECT_NAME AGP_CONTAINER_PREFIX AGP_DATA_DIR AGP_RESOURCE_ROOT \
  RESOURCE_MIGRATION_DRY_RUN_ONLY RESOURCE_LEGACY_ASSETS_ROOT EXECUTE_IMPORT; do
  if [ -z "${!env_name:-}" ]; then
    printf -v "$env_name" '%s' "$(env_file_value "$env_name")"
  fi
done

if [ -z "$SOURCE_PROJECT_DIR" ]; then
  echo "缺少 SOURCE_PROJECT_DIR" >&2
  usage
  exit 1
fi

if [ ! -d "$SOURCE_PROJECT_DIR" ]; then
  echo "旧项目目录不存在: $SOURCE_PROJECT_DIR" >&2
  exit 1
fi

CONFIG_PATH="${CONFIG_PATH:-$SOURCE_PROJECT_DIR/config.json}"
RECORDS_PATH="${RECORDS_PATH:-$SOURCE_PROJECT_DIR/data/records.json}"
PREFER_SHARED_ASSETS="${PREFER_SHARED_ASSETS:-true}"
EXECUTE_IMPORT="${EXECUTE_IMPORT:-false}"
export CONFIG_PATH RECORDS_PATH PREFER_SHARED_ASSETS

"$ROOT_DIR/scripts/migrate-group.sh"

if [ "$EXECUTE_IMPORT" != "true" ]; then
  exit 0
fi

AGP_CONTAINER_PREFIX="${AGP_CONTAINER_PREFIX:-agp}"
MYSQL_CONTAINER_NAME="${MYSQL_CONTAINER_NAME:-${AGP_CONTAINER_PREFIX}-mysql}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-$(docker inspect "$MYSQL_CONTAINER_NAME" --format '{{ index .Config.Labels "com.docker.compose.project" }}' 2>/dev/null || true)}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-$AGP_CONTAINER_PREFIX}"
MYSQL_DATABASE="${MYSQL_DATABASE:-agp}"
MYSQL_USER="${MYSQL_USER:-agp}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-agp}"
AGP_DATA_DIR="${AGP_DATA_DIR:-$ROOT_DIR/data}"
AGP_RESOURCE_ROOT="${AGP_RESOURCE_ROOT:-$AGP_DATA_DIR/resources}"
RESOURCE_MIGRATION_DRY_RUN_ONLY="${RESOURCE_MIGRATION_DRY_RUN_ONLY:-false}"

compose() {
  local args=()
  if [ -f "$ENV_FILE" ]; then
    args+=(--env-file "$ENV_FILE")
  fi
  docker compose "${args[@]}" -p "$COMPOSE_PROJECT_NAME" -f "$COMPOSE_FILE" "$@"
}

abs_path() {
  local target="$1"
  if [ -d "$target" ]; then
    (cd "$target" && pwd)
  else
    (cd "$(dirname "$target")" && printf '%s/%s\n' "$(pwd)" "$(basename "$target")")
  fi
}

run_resource_file_migration() {
  local dry_run="$1"
  local dsn="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(mysql:3306)/${MYSQL_DATABASE}?parseTime=true&multiStatements=false&charset=utf8mb4,utf8"
  local legacy_root_abs
  legacy_root_abs="$(abs_path "$SOURCE_PROJECT_DIR")"
  local resource_root_abs
  resource_root_abs="$(abs_path "$AGP_RESOURCE_ROOT")"
  local args=(
    "/app/migrate-resource-files"
    "--dsn" "$dsn"
    "--group-code" "$GROUP_CODE"
    "--legacy-root" "/legacy-root"
    "--resource-root" "/resource-root"
    "--dry-run=${dry_run}"
  )
  local mount_args=(
    -v "$legacy_root_abs:/legacy-root:ro"
    -v "$resource_root_abs:/resource-root"
  )
  if [ -n "${RESOURCE_LEGACY_ASSETS_ROOT:-}" ] && [ -d "$RESOURCE_LEGACY_ASSETS_ROOT" ]; then
    args+=("--legacy-assets-root" "/legacy-assets-root")
    mount_args+=(-v "$(abs_path "$RESOURCE_LEGACY_ASSETS_ROOT"):/legacy-assets-root:ro")
  fi
  compose run --rm -T --no-deps "${mount_args[@]}" backend "${args[@]}"
}

echo ">>> resource-file dry-run: ${GROUP_CODE}"
run_resource_file_migration true

if [ "$RESOURCE_MIGRATION_DRY_RUN_ONLY" = "true" ]; then
  echo ">>> 已按 RESOURCE_MIGRATION_DRY_RUN_ONLY=true 跳过正式资源文件迁移"
  exit 0
fi

echo ">>> resource-file import: ${GROUP_CODE}"
run_resource_file_migration false
