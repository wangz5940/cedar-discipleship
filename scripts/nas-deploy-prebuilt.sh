#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GIT="${GIT:-/usr/local/bin/git}"
DOCKER="${DOCKER:-/usr/local/bin/docker}"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/deploy/docker-compose.separated.yml}"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
LOCK_DIR="${AGP_DEPLOY_LOCK_DIR:-/tmp/cedar-prebuilt-deploy.lock}"

log() {
  printf '[%s] %s\n' "$(date '+%F %T')" "$*"
}

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

docker_cmd() {
  if [ "${AGP_DOCKER_USE_SUDO:-true}" = "true" ]; then
    sudo -n "$DOCKER" "$@"
  else
    "$DOCKER" "$@"
  fi
}

lower() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

github_path_from_origin() {
  local origin path
  origin="$("$GIT" -C "$ROOT_DIR" remote get-url origin 2>/dev/null || true)"
  case "$origin" in
    git@github.com:*)
      path="${origin#git@github.com:}"
      ;;
    https://github.com/*)
      path="${origin#https://github.com/}"
      ;;
    *)
      path="wangz5940/cedar-discipleship"
      ;;
  esac
  path="${path%.git}"
  printf '%s\n' "$path"
}

if [ ! -f "$ENV_FILE" ]; then
  fail "missing env file: $ENV_FILE"
fi

if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  fail "another deployment appears to be running: $LOCK_DIR"
fi

IMAGE_ENV=""
cleanup() {
  if [ -n "$IMAGE_ENV" ] && [ -f "$IMAGE_ENV" ]; then
    rm -f "$IMAGE_ENV"
  fi
  rmdir "$LOCK_DIR" 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM

cd "$ROOT_DIR"

if [ -n "$("$GIT" status --porcelain=v1 -uno)" ]; then
  fail "tracked worktree changes exist on NAS; stash or commit them before deploying"
fi

log "Syncing repository"
"$GIT" fetch origin --prune

if [ -n "${AGP_GIT_REF:-}" ]; then
  "$GIT" checkout --detach "$AGP_GIT_REF"
else
  "$GIT" switch master
  "$GIT" pull --ff-only origin master
fi

set +u
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
set -u

COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-cedar}"
AGP_CONTAINER_PREFIX="${AGP_CONTAINER_PREFIX:-cedar}"
AGP_IMAGE_REGISTRY="${AGP_IMAGE_REGISTRY:-ghcr.io}"
AGP_IMAGE_TAG="${AGP_IMAGE_TAG:-$("$GIT" rev-parse HEAD)}"

github_path="$(lower "$(github_path_from_origin)")"
AGP_IMAGE_OWNER="${AGP_IMAGE_OWNER:-${github_path%%/*}}"
AGP_IMAGE_REPO="${AGP_IMAGE_REPO:-${github_path#*/}}"

AGP_BACKEND_IMAGE="${AGP_BACKEND_IMAGE:-${AGP_IMAGE_REGISTRY}/${AGP_IMAGE_OWNER}/${AGP_IMAGE_REPO}-backend:${AGP_IMAGE_TAG}}"
AGP_FRONTEND_IMAGE="${AGP_FRONTEND_IMAGE:-${AGP_IMAGE_REGISTRY}/${AGP_IMAGE_OWNER}/${AGP_IMAGE_REPO}-frontend:${AGP_IMAGE_TAG}}"

IMAGE_ENV="$(mktemp /tmp/cedar-images.XXXXXX)"
{
  printf 'AGP_BACKEND_IMAGE=%s\n' "$AGP_BACKEND_IMAGE"
  printf 'AGP_FRONTEND_IMAGE=%s\n' "$AGP_FRONTEND_IMAGE"
} >"$IMAGE_ENV"
chmod 600 "$IMAGE_ENV"

compose_args=(
  --env-file "$ENV_FILE"
  --env-file "$IMAGE_ENV"
  -p "$COMPOSE_PROJECT_NAME"
  -f "$COMPOSE_FILE"
)

log "Pulling prebuilt images for $AGP_IMAGE_TAG"
if ! docker_cmd compose "${compose_args[@]}" pull backend frontend; then
  cat >&2 <<EOF

Unable to pull prebuilt images.
If the GHCR packages are private, log in once on the NAS:
  sudo /usr/local/bin/docker login ghcr.io -u <github-user>

Expected images:
  $AGP_BACKEND_IMAGE
  $AGP_FRONTEND_IMAGE
EOF
  exit 1
fi

log "Starting containers without rebuilding"
docker_cmd compose "${compose_args[@]}" up -d --no-build backend frontend

log "Current containers"
docker_cmd ps --filter "name=${AGP_CONTAINER_PREFIX}-" --format '{{.Names}} {{.Status}} {{.Image}}'
