# Cedar Discipleship

> A paper-inspired discipleship workspace for study planning, resource curation, and daily check-ins
>
> 建议 GitHub 仓库名：`cedar-discipleship`

这是一个采用纸感知识库风格的小组研修与打卡平台。把学习计划、内容查看、打卡记录、统计看板和多小组管理收在同一个 Web 应用里，适合门训、课程共学、读书小组等需要“持续学习 + 过程记录”的场景。当前主版本已经升级为前后端分离架构：

- 后端：Go
- 数据库：MySQL 8.0
- 前端：Vue 3 + Vite + Pinia
- 部署：Docker Compose

当前版本支持多小组隔离、按组学习内容配置、成员与权限管理、每日/周任务打卡、统计看板、资源库与旧数据迁移。

## 功能概览

- 多小组隔离：成员、打卡、周任务、资源按 `group_id` 隔离
- 权限体系：超级管理员、组长、小组管理员、普通成员
- 打卡工作台：首页展示当天学习任务、日期切换、回到今天与个人打卡记录
- 统计中心：小组完成率、成员矩阵、本月累计排行，并支持导出柱状图 PNG
- 学习内容管理：按组配置每日内容、周任务、视频、读物、背经、提纲图
- 资源库：资源按学习小组独立存储，支持跨组授权、逻辑导入、依赖图谱和导入历史
- 内容查看器：统一预览 Markdown / PDF / 视频 / 图片，并支持同主题资料“上一篇 / 下一篇”连续浏览
- 历史迁移：支持把旧 `config.json` 和 `records.json` 导入 MySQL 平台

## 开源边界

- 本仓库公开的是应用代码、部署配置和迁移脚本，不包含生产数据。
- `.env`、本地数据库目录、备份文件和上传文件均应保留在部署环境，不进入版本控制。
- `config.json` 与 `data/records.json` 仅作为历史数据迁移输入，不应视为公开示例数据集。

## 目录结构

```text
.
├── backend/                     # Go 后端
│   ├── cmd/server/main.go
│   ├── cmd/migrate-json/main.go
│   ├── migrations/
│   └── Dockerfile
├── frontend/                    # Vue 3 + Vite + Pinia 前端
│   ├── src/main.js
│   ├── src/App.vue
│   ├── src/stores/
│   ├── src/legacy-app.js        # 当前前端业务运行时与状态桥接层
│   ├── src/styles.css
│   ├── package.json
│   ├── vite.config.js
│   ├── nginx.conf
│   └── Dockerfile
├── deploy/
│   └── docker-compose.separated.yml
├── scripts/
│   ├── init-deploy-env.sh      # 生成本地部署 .env
│   ├── deploy-oneclick.sh       # 新环境一键部署
│   ├── migrate-group.sh         # 底层旧 JSON 迁移入口
│   └── migrate-legacy-project.sh # 旧独立项目一站式迁移入口
├── docs/
│   ├── ops-commands.md
│   ├── deploy-new-environment.md
│   ├── migrate-other-groups.md
│   └── implementation-notes.md
├── data/
│   ├── mysql/
│   ├── resources/
│   └── backups/mysql/
└── config.json                  # 旧数据迁移输入之一
```

## 快速开始

### 1. 直接启动当前平台

在项目根目录执行：

```bash
./scripts/init-deploy-env.sh
docker compose --env-file .env -f deploy/docker-compose.separated.yml up -d --build
```

默认访问地址：

```text
http://127.0.0.1:5114
```

前端容器默认监听宿主机 `0.0.0.0:${AGP_WEB_PORT:-5114}`，在局域网或服务器环境中也可以通过 `http://<宿主机IP>:5114` 访问。

默认 MySQL 端口：

```bash
./scripts/init-deploy-env.sh

docker compose --env-file .env -f deploy/docker-compose.separated.yml up -d --build
```

脚本会补齐 `.env` 中缺失的部署变量和随机密钥，不覆盖已存在的值，并创建 `AGP_DATA_DIR` 下的 `mysql`、`resources`、`backups/mysql` 目录。`AGP_RESOURCE_ROOT` 保存按组隔离的不可变资源，目录格式为 `team-{group_code}-resources/objects/{resource_key}/{filename}`；资源上传后默认对所有学习小组开放导入。

首次超级管理员由环境变量创建。已运行 `./scripts/init-deploy-env.sh` 时，所需变量会写入 `.env`；未运行该脚本而直接使用 Docker Compose 启动时，必须手动提供：

```bash
export AGP_JWT_SECRET='替换为长随机字符串'
export BOOTSTRAP_SUPERADMIN_USERNAME='admin'
export BOOTSTRAP_SUPERADMIN_PASSWORD='替换为强密码'
export BOOTSTRAP_SUPERADMIN_DISPLAY_NAME='超级管理员'

# 可选：默认空值表示登录令牌永久有效；例如 24h 表示 24 小时过期
export AGP_TOKEN_TTL=''
```

如果部署机器无法访问 `proxy.golang.org` 或 `registry.npmjs.org`，镜像构建会在依赖下载阶段超时。NAS 或受限网络环境里，先设置 Go 模块代理和 npm registry 再执行部署：

这些变量会透传到 `backend`/`frontend` 镜像构建，以及迁移脚本内部启动的 `golang:1.25-bookworm` 容器。

### 2. 本地检查

```bash
cd backend
go test ./...

cd ..
cd frontend
npm install
npm run build

cd ..
docker compose -f deploy/docker-compose.separated.yml config
```

## 新环境一键部署

如果你要在一台新的服务器、NAS 或 Docker 主机上直接部署：

```bash
./scripts/deploy-oneclick.sh
```

这个脚本会：

1. 初始化 `data/mysql`、`data/resources`、`data/backups/mysql`
2. 启动 `mysql / backend / frontend`
3. 等待 MySQL 就绪
4. 可选执行首个小组 JSON 数据迁移
5. 可选执行资源文件迁移，将数据库中该组旧资源路径对应的文件复制到 `data/resources`

## 旧数据迁移

### 首次部署时迁移首个小组

```bash
export PRIMARY_GROUP_CODE='agape-a'
export PRIMARY_GROUP_NAME='AGAPE A组'
export PRIMARY_GROUP_DEFAULT_PASSWORD='Abc12345'
export PRIMARY_CONFIG_PATH='/absolute/path/to/config.json'
export PRIMARY_RECORDS_PATH='/absolute/path/to/records.json'
export RESOURCE_MIGRATION_GROUP_NAME='AGAPE A组'
export RESOURCE_LEGACY_ROOT='/absolute/path/to/old-resource-root'

./scripts/deploy-oneclick.sh
```

资源文件迁移按 `RESOURCE_MIGRATION_GROUP_NAME` 查询数据库中的 `study_groups.name`。文件目标目录使用查到的 `study_groups.code`，最终路径为 `data/resources/team-{group_code}-resources/objects/{resource_key}/{filename}`。如果 `RESOURCE_MIGRATION_GROUP_NAME` 未设置，会使用 `PRIMARY_GROUP_NAME`；脚本不写死任何小组名称。

### 已上线后继续迁移其他组

旧独立项目目录迁移使用一站式入口。它会读取旧项目下的 `config.json` 和 `data/records.json`，正式导入后迁移本组独有资料文件；已由其他小组共享的同名同类资源会优先复用，不重复复制文件。

```bash
SOURCE_PROJECT_DIR='/volume1/docker/zw1-checkin' \
GROUP_CODE='zw1' \
GROUP_NAME='ZW1小组' \
GROUP_DEFAULT_PASSWORD='Abc12345' \
EXECUTE_IMPORT=false \
./scripts/migrate-legacy-project.sh
```

确认 dry-run 报告后，将 `EXECUTE_IMPORT=true` 重新执行。`GROUP_CODE` 是迁移和资源路径使用的内部稳定标识；管理后台只展示和维护小组名称。

## 跨组资源治理

资源上传后不可修改；内容变化时上传为新的独立资源。新资源默认对所有学习小组共享；跨组导入只建立数据库逻辑引用，不复制文件。资料库只展示当前学习小组已上传或已导入的数据库资源。资源治理页支持事务性的批量权限设置、批量删除和批量导入，并记录聚合审计日志。

详细权限、目录和访问规范见 [资源治理规范](docs/resource-governance.md)。

## 专项小组初始化

专项小组是新功能，不涉及旧数据迁移。表结构由后端启动时的数据库建表流程创建；专项小组目录的独立初始化 SQL 在：

```text
backend/sql/init_ministry_groups.sql
```

这份 SQL 会为每个现有 `study_groups` 初始化以下专项小组，且可重复执行：

```text
领会组、主持组、伙食组、后勤组、整洁组、技术组、策划组、数点组、
探望组、回报组、娃娃组、守望组、门训数点组、门训规划发布组、门训批改组
```

Docker Compose 部署环境下一键执行：

```bash
./scripts/init-ministry-groups.sh
```

如果需要直连本机或远端 MySQL：

```bash
USE_LOCAL_MYSQL=true \
MYSQL_HOST=127.0.0.1 \
MYSQL_PORT=3307 \
MYSQL_DATABASE=agp \
MYSQL_USER=agp \
MYSQL_PASSWORD=agp \
./scripts/init-ministry-groups.sh
```

如果现有 MySQL 数据卷里的应用账号密码与当前配置不一致，脚本会尝试读取正在运行的
`agp-mysql` 容器环境变量作为兜底；也可显式提供 root 密码：

## License

This project is licensed under the MIT License. See [LICENSE](file:///Users/bytedance/program/agp/LICENSE).
