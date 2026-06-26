# Cedar Discipleship

> A paper-inspired discipleship workspace for study planning, resource curation, and daily check-ins
>
> 建议 GitHub 仓库名：`cedar-discipleship`

这是一个采用纸感知识库风格的小组研修与打卡平台。对外英文名称为 `Cedar Discipleship`。它把学习计划、内容查看、打卡记录、统计看板和多小组管理收在同一个 Web 应用里，适合门训、课程共学、读书小组等需要“持续学习 + 过程记录”的场景。当前主版本已经升级为前后端分离架构：

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
- 资源库：按读物 / 讲义 / 视频分组展示资料，并支持 Markdown/PDF/视频/讲义上传
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
│   ├── deploy-oneclick.sh       # 新环境一键部署
│   └── migrate-group.sh         # 独立迁移其他组
├── docs/
│   ├── ops-commands.md
│   ├── deploy-new-environment.md
│   ├── migrate-other-groups.md
│   └── implementation-notes.md
├── Book/
├── Newtestament/
├── PPT/
└── config.json                  # 旧数据迁移输入之一
```

## 快速开始

### 1. 直接启动当前平台

在项目根目录执行：

```bash
docker compose -f deploy/docker-compose.separated.yml up -d --build
```

默认访问地址：

```text
http://127.0.0.1:5114
```

前端容器默认监听宿主机 `0.0.0.0:${AGP_WEB_PORT:-5114}`，在局域网或服务器环境中也可以通过 `http://<宿主机IP>:5114` 访问。

默认 MySQL 端口：

```text
127.0.0.1:3307
```

默认首个超级管理员：

```text
账号：admin
密码：ChangeMe123
```

生产环境必须覆盖：

```bash
export AGP_JWT_SECRET='替换为长随机字符串'
export BOOTSTRAP_SUPERADMIN_USERNAME='admin'
export BOOTSTRAP_SUPERADMIN_PASSWORD='替换为强密码'
export BOOTSTRAP_SUPERADMIN_DISPLAY_NAME='超级管理员'
```

如果部署机器无法访问 `proxy.golang.org`，后端镜像构建会在 `go mod download` 阶段超时。NAS 或受限网络环境里，先设置 Go 模块代理再执行部署：

```bash
export GOPROXY='https://goproxy.cn,direct'

# 如果模块校验服务仍不可达，再临时关闭校验数据库
# export GOSUMDB='off'
```

这些变量会透传到 `backend` 镜像构建，以及迁移脚本内部启动的 `golang:1.25-bookworm` 容器。

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

1. 初始化 `data/mysql`、`data/assets`、`data/backups/mysql`
2. 启动 `mysql / backend / frontend`
3. 等待 MySQL 就绪
4. 可选执行首个小组迁移

## 旧数据迁移

### 首次部署时迁移首个小组

```bash
export PRIMARY_GROUP_CODE='agape-a'
export PRIMARY_GROUP_NAME='AGAPE A组'
export PRIMARY_GROUP_DEFAULT_PASSWORD='Abc12345'
export PRIMARY_CONFIG_PATH='/absolute/path/to/config.json'
export PRIMARY_RECORDS_PATH='/absolute/path/to/records.json'

./scripts/deploy-oneclick.sh
```

### 已上线后继续迁移其他组

```bash
GROUP_CODE='agape-b' \
GROUP_NAME='AGAPE B组' \
CONFIG_PATH='/absolute/path/to/config.json' \
RECORDS_PATH='/absolute/path/to/records.json' \
GROUP_DEFAULT_PASSWORD='Abc12345' \
EXECUTE_IMPORT=true \
./scripts/migrate-group.sh
```

默认建议先只做 dry-run，检查迁移报告里的 `generated_usernames`、`warnings` 和 `failures` 后再导入。

## 运行与运维

常用命令：

```bash
# 启动
docker compose -f deploy/docker-compose.separated.yml up -d --build

# 查看状态
docker compose -f deploy/docker-compose.separated.yml ps

# 查看日志
docker compose -f deploy/docker-compose.separated.yml logs -f

# 停止
docker compose -f deploy/docker-compose.separated.yml down
```

MySQL 进入方式：

```bash
docker exec -it agp-mysql mysql -uagp -pagp agp
```

数据库备份：

```bash
mkdir -p data/backups/mysql
docker exec agp-mysql mysqldump -uagp -pagp agp > data/backups/mysql/agp-$(date +%F).sql
```

## 静态资料与上传目录

- `Book/`：静态 PDF 读物目录，用于资源库和部分周计划资料匹配
- `Newtestament/`：静态视频目录
- `PPT/`：静态讲义目录
- `data/assets/`：后台上传到资源库的文件目录

如果你把仓库部署到新的机器上，需要保留这些目录，或同步调整 [`deploy/docker-compose.separated.yml`](file:///Users/bytedance/program/agp/deploy/docker-compose.separated.yml) 里的挂载路径。

## 当前实现规则

- 每个账号只有一个真实登录密码
- 登录成功后后端签发 Token，前端会持久化到 `localStorage`
- 未认证状态下不展示用户所属小组
- 单小组用户不显示额外的小组切换入口
- 小组默认密码只影响只属于该组的普通成员
- 小组管理员和组长不能操作超级管理员、同级或自己
- 打卡记录按小组隔离
- 未来日期禁止打卡
- PDF 读物通过后端裁页接口只暴露指定页范围
- 对 `/range` 裁页 PDF，前端查看器会强制从第 1 页开始渲染，避免默认跳页

  <br />

## 迁移输入说明

当前仓库保留的旧数据输入主要是迁移所需配置与记录文件，例如：

- `config.json`
- `data/records.json`

它们仅用于迁移旧数据，不作为当前版本的运行入口。

## License

This project is licensed under the MIT License. See [LICENSE](file:///Users/bytedance/program/agp/LICENSE).
