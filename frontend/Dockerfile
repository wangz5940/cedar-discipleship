FROM node:22-bookworm-slim AS builder

WORKDIR /app

COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build

FROM nginx:1.27-alpine

COPY --from=builder /app/dist/ /usr/share/nginx/html/
COPY config.json /usr/share/nginx/html/config.json
COPY newtestament.md /usr/share/nginx/html/newtestament.md
COPY weekly_task.md /usr/share/nginx/html/weekly_task.md
COPY Kuangye.md /usr/share/nginx/html/Kuangye.md
COPY Book/ /usr/share/nginx/html/Book/
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
