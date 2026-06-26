#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/deploy/docker-compose.separated.yml}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-agp}"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
MYSQL_DATABASE="${MYSQL_DATABASE:-agp}"
GOPROXY="${GOPROXY:-}"
GOSUMDB="${GOSUMDB:-}"
GOPRIVATE="${GOPRIVATE:-}"
GONOSUMDB="${GONOSUMDB:-}"
GONOPROXY="${GONOPROXY:-}"

GROUP_CODE="${GROUP_CODE:-}"
GROUP_NAME="${GROUP_NAME:-}"
CONFIG_PATH="${CONFIG_PATH:-}"
RECORDS_PATH="${RECORDS_PATH:-}"
GROUP_DEFAULT_PASSWORD="${GROUP_DEFAULT_PASSWORD:-}"
REPORT_DIR="${REPORT_DIR:-$ROOT_DIR/data/migration-reports}"
ALLOW_DUPLICATE_AS_DELETED="${ALLOW_DUPLICATE_AS_DELETED:-false}"
FAIL_ON_GENERATED_USERNAMES="${FAIL_ON_GENERATED_USERNAMES:-false}"
EXECUTE_IMPORT="${EXECUTE_IMPORT:-false}"
TMP_INPUT_DIR=""
CONFIG_IN_CONTAINER=""
RECORDS_IN_CONTAINER=""

usage() {
  cat <<EOF
用法:
  GROUP_CODE=agape-b \\
  GROUP_NAME="AGAPE B组" \\
  CONFIG_PATH=/path/to/config.json \\
  RECORDS_PATH=/path/to/records.json \\
  GROUP_DEFAULT_PASSWORD='Abc12345' \\
  EXECUTE_IMPORT=true \\
  ./scripts/migrate-group.sh

默认行为只执行 dry-run，不写数据库。
将 EXECUTE_IMPORT=true 后，会先 dry-run，再执行正式导入。
EOF
}

rand_password() {
  LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "${1:-12}"
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

abs_path() {
  local target="$1"
  if [ -d "$target" ]; then
    (cd "$target" && pwd)
  else
    (cd "$(dirname "$target")" && printf '%s/%s\n' "$(pwd)" "$(basename "$target")")
  fi
}

validate() {
  [ -n "$GROUP_CODE" ] || { echo "缺少 GROUP_CODE" >&2; usage; exit 1; }
  [ -n "$GROUP_NAME" ] || { echo "缺少 GROUP_NAME" >&2; usage; exit 1; }
  [ -n "$CONFIG_PATH" ] || { echo "缺少 CONFIG_PATH" >&2; usage; exit 1; }
  [ -n "$RECORDS_PATH" ] || { echo "缺少 RECORDS_PATH" >&2; usage; exit 1; }
  [ -f "$CONFIG_PATH" ] || { echo "配置文件不存在: $CONFIG_PATH" >&2; exit 1; }
  [ -f "$RECORDS_PATH" ] || { echo "记录文件不存在: $RECORDS_PATH" >&2; exit 1; }
  if [ -z "$GROUP_DEFAULT_PASSWORD" ]; then
    GROUP_DEFAULT_PASSWORD="$(rand_password 12)"
  fi
}

prepare_inputs() {
  TMP_INPUT_DIR="$ROOT_DIR/.tmp/migration-inputs-${GROUP_CODE}"
  rm -rf "$TMP_INPUT_DIR"
  mkdir -p "$TMP_INPUT_DIR"
  cp "$(abs_path "$CONFIG_PATH")" "$TMP_INPUT_DIR/config.json"
  cp "$(abs_path "$RECORDS_PATH")" "$TMP_INPUT_DIR/records.json"
  CONFIG_IN_CONTAINER="/workspace/.tmp/migration-inputs-${GROUP_CODE}/config.json"
  RECORDS_IN_CONTAINER="/workspace/.tmp/migration-inputs-${GROUP_CODE}/records.json"
}

cleanup() {
  if [ -n "$TMP_INPUT_DIR" ] && [ -d "$TMP_INPUT_DIR" ]; then
    rm -rf "$TMP_INPUT_DIR"
  fi
}

ensure_mysql() {
  compose ps mysql >/dev/null
  compose exec -T mysql mysqladmin ping -h 127.0.0.1 -uagp -pagp >/dev/null
}

run_cli() {
  local dry_run="$1"
  local network_name="${COMPOSE_PROJECT_NAME}_default"
  local dsn="agp:agp@tcp(mysql:3306)/${MYSQL_DATABASE}?parseTime=true&multiStatements=false&charset=utf8mb4,utf8"
  local docker_run_args=(--rm --network "$network_name")
  local docker_env_args=()
  local args=(
    "go" "run" "./cmd/migrate-json"
    "--dsn" "$dsn"
    "--group-code" "$GROUP_CODE"
    "--group-name" "$GROUP_NAME"
    "--config" "$CONFIG_IN_CONTAINER"
    "--records" "$RECORDS_IN_CONTAINER"
    "--default-password" "$GROUP_DEFAULT_PASSWORD"
    "--report-dir" "/workspace/${REPORT_DIR#$ROOT_DIR/}"
    "--dry-run=${dry_run}"
    "--allow-duplicate-as-deleted=${ALLOW_DUPLICATE_AS_DELETED}"
    "--fail-on-generated-usernames=${FAIL_ON_GENERATED_USERNAMES}"
  )
  for env_name in GOPROXY GOSUMDB GOPRIVATE GONOSUMDB GONOPROXY; do
    if [ -n "${!env_name:-}" ]; then
      docker_env_args+=(-e "${env_name}=${!env_name}")
    fi
  done
  if [ -f "$ENV_FILE" ]; then
    docker_run_args+=(--env-file "$ENV_FILE")
  fi
  docker run "${docker_run_args[@]}" \
    "${docker_env_args[@]}" \
    -v "$ROOT_DIR:/workspace" \
    -w /workspace/backend \
    golang:1.25-bookworm \
    "${args[@]}"
}

require_cmd docker
validate
mkdir -p "$REPORT_DIR"
trap cleanup EXIT
prepare_inputs
ensure_mysql

echo ">>> dry-run: ${GROUP_CODE} / ${GROUP_NAME}"
run_cli true

if [ "$EXECUTE_IMPORT" = "true" ]; then
  echo ">>> import: ${GROUP_CODE} / ${GROUP_NAME}"
  run_cli false
else
  echo ">>> 当前仅 dry-run。如需正式导入，请增加 EXECUTE_IMPORT=true"
fi

cat <<EOF

迁移完成。
group_code: ${GROUP_CODE}
group_name: ${GROUP_NAME}
default_password: ${GROUP_DEFAULT_PASSWORD}
report_dir: ${REPORT_DIR}

EOF
