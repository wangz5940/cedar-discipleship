FROM node:22-bookworm-slim AS builder

WORKDIR /app

ARG NPM_CONFIG_REGISTRY=https://registry.npmjs.org/

COPY frontend/package*.json ./
RUN npm ci \
  --registry="$NPM_CONFIG_REGISTRY" \
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
