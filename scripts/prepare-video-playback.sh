#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/deploy/docker-compose.separated.yml}"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"

if [ -f "$ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
fi

COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-cedar}"
MYSQL_DATABASE="${MYSQL_DATABASE:-agp}"
MYSQL_USER="${MYSQL_USER:-agp}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-agp}"
AGP_LOG_DIR="${AGP_LOG_DIR:-$ROOT_DIR/logs}"
DRY_RUN=false

if [ "${1:-}" = "--dry-run" ]; then
  DRY_RUN=true
elif [ "$#" -gt 0 ]; then
  echo "用法: $0 [--dry-run]" >&2
  exit 2
fi

mkdir -p "$AGP_LOG_DIR/video-playback"
LOG_FILE="$AGP_LOG_DIR/video-playback/prepare-$(date '+%Y%m%d-%H%M%S').log"
exec > >(tee -a "$LOG_FILE") 2>&1
echo "日志: $LOG_FILE"

compose() {
  local args=()
  if [ -f "$ENV_FILE" ]; then
    args+=(--env-file "$ENV_FILE")
  fi
  docker compose "${args[@]}" -p "$COMPOSE_PROJECT_NAME" -f "$COMPOSE_FILE" "$@"
}

query="
SELECT MIN(a.id) AS asset_id,a.storage_path
FROM assets a
JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
WHERE (
    a.category='video'
    OR a.mime_type LIKE 'video/%'
    OR RIGHT(LOWER(a.original_name),4) IN ('.mp4','.m4v','.mov')
    OR RIGHT(LOWER(a.original_name),5)='.webm'
  )
  AND a.storage_path LIKE 'team-%-resources/objects/%'
GROUP BY a.storage_path
ORDER BY asset_id"

list_file="$(mktemp "${TMPDIR:-/tmp}/agp-video-playback.XXXXXX")"
trap 'rm -f "$list_file"' EXIT

compose exec -T -e MYSQL_PWD="$MYSQL_PASSWORD" mysql \
  mysql --default-character-set=utf8mb4 --batch --raw --skip-column-names \
  -u"$MYSQL_USER" "$MYSQL_DATABASE" -e "$query" >"$list_file"

total=0
created=0
skipped=0
failed=0

while IFS=$'\t' read -r asset_id storage_path || [ -n "${asset_id:-}" ]; do
  [ -n "$asset_id" ] || continue
  total=$((total + 1))
  case "$storage_path" in
    team-*-resources/objects/*) ;;
    *)
      echo "拒绝非法资源路径: asset_id=$asset_id path=$storage_path" >&2
      failed=$((failed + 1))
      continue
      ;;
  esac

  source_path="/data/agp/resources/$storage_path"
  playback_path="${source_path}.playback.mp4"
  if compose exec -T backend sh -c 'test -s "$1" && ffprobe -v error "$1" >/dev/null 2>&1' sh "$playback_path" </dev/null; then
    echo "跳过 asset_id=${asset_id}，播放衍生文件已存在"
    skipped=$((skipped + 1))
    continue
  fi
  if [ "$DRY_RUN" = true ]; then
    echo "待处理 asset_id=$asset_id path=$storage_path"
    continue
  fi

  echo "生成 asset_id=$asset_id 的播放衍生文件"
  if compose exec -T backend sh -ceu '
    source_path="$1"
    playback_path="$2"
    temp_path="${playback_path}.tmp.mp4"
    test -s "$source_path"
    rm -f "$temp_path"
    trap "rm -f \"$temp_path\"" EXIT
    ffmpeg -nostdin -hide_banner -loglevel error \
      -i "$source_path" -map 0 -c copy \
      -movflags +frag_keyframe+empty_moov+default_base_moof \
      "$temp_path"
    ffprobe -v error "$temp_path" >/dev/null
    chmod 0640 "$temp_path"
    mv "$temp_path" "$playback_path"
    trap - EXIT
  ' sh "$source_path" "$playback_path" </dev/null; then
    created=$((created + 1))
  else
    echo "生成失败: asset_id=$asset_id path=$storage_path" >&2
    failed=$((failed + 1))
  fi
done <"$list_file"

echo "video_playback_prepare total=$total created=$created skipped=$skipped failed=$failed dry_run=$DRY_RUN"
if [ "$failed" -gt 0 ]; then
  exit 1
fi
