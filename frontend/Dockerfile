FROM node:22-bookworm-slim AS builder

WORKDIR /app

ARG NPM_CONFIG_REGISTRY=https://registry.npmjs.org/

COPY frontend/package*.json ./
RUN clean_registry="$(printf '%s' "$NPM_CONFIG_REGISTRY" | tr -d '\047\042\140[:space:]')" \
  && case "$clean_registry" in http://*|https://*) ;; *) echo "Invalid NPM_CONFIG_REGISTRY: $NPM_CONFIG_REGISTRY" >&2; exit 1 ;; esac \
  && npm ci \
  --registry="$clean_registry" \
  --fetch-retries=5 \
  --fetch-retry-mintimeout=20000 \
  --fetch-retry-maxtimeout=120000 \
  --fetch-timeout=120000 \
  --no-audit \
  --no-fund

COPY frontend/ ./
RUN npm run build

FROM nginx:1.27-alpine

COPY --from=builder /app/dist/ /usr/share/nginx/html/
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
