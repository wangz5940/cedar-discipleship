# 前后端分离实现说明

本次实现为当前使用中的前后端分离版本说明。

## 新增目录

- `backend/`：Go 后端服务。
  - `cmd/server/main.go`：HTTP API、认证、权限、打卡、成员、小组、资源、分区维护。
  - `migrations/`：MySQL 8.0 初始化 SQL。
  - `Dockerfile`：Go 后端镜像。
- `frontend/`：Vue 3 + Vite + Pinia 浏览器 Web 前端。
  - `index.html`
  - `src/main.js`
  - `src/App.vue`
  - `src/components/AppRoot.vue`：Vue 化的全局页面壳，负责登录、布局、导航、小组选择、资源页、管理页、月历和 toast；资源页已按读物 / 讲义 / 视频分组展示，并带资料库概览。
  - `src/components/Dashboard.vue`：Vue 化的统计页，负责小组完成率、成员打卡矩阵、月度分项排行和柱状图导出。
  - `src/components/CheckinWorkbench.vue`：Vue 化的首页打卡工作台，负责日期切换、回到今天、任务卡、打卡操作和我的记录。
  - `src/components/ContentViewer.vue`：Vue 化的内容查看器，负责 Markdown/PDF/视频/图片弹窗、关联资料区以及同组资料“上一篇 / 下一篇”连续浏览。
  - `src/stores/`
  - `src/stores/appState.js`：全局页面壳 Pinia 状态。
  - `src/stores/dashboard.js`：统计页 Pinia 状态。
  - `src/stores/checkinWorkbench.js`：首页打卡工作台 Pinia 状态。
  - `src/stores/contentViewer.js`：内容查看器 Pinia 状态。
  - `src/legacy-app.js`：前端业务运行时与状态桥接层，负责 API 调用、状态计算和动作函数；主页面 DOM 已迁出到 Vue。
  - `src/styles.css`
  - `package.json`
  - `vite.config.js`
  - `nginx.conf`
  - `Dockerfile`
- `deploy/docker-compose.separated.yml`：MySQL + Go 后端 + Nginx 前端部署。

## 运行方式

```bash
docker compose -f deploy/docker-compose.separated.yml up -d --build
```

默认访问端口：

```text
http://127.0.0.1:5114
```

默认首个超级管理员：

```text
账号：admin
密码：ChangeMe123
```

生产部署前必须通过环境变量覆盖：

```bash
export AGP_JWT_SECRET='替换为长随机字符串'
export BOOTSTRAP_SUPERADMIN_USERNAME='你的超级管理员账号'
export BOOTSTRAP_SUPERADMIN_PASSWORD='你的强密码'
```

## 已实现的核心规则

- 每个账号只有一个实际登录密码，登录只需要 `username + password`。
- 登录成功后后端返回 Token，前端持久化到 `localStorage`，刷新页面后可自动恢复登录态。
- 未认证状态下不展示用户所属小组，避免通过用户名枚举小组归属。
- 单小组成员不展示额外的小组切换 UI；只有多小组且当前未锁定小组时，前端才展示选择入口。
- 小组默认密码只用于新建本组用户初始化密码，以及按规则批量重置成员密码。
- 组长修改本组默认密码时：
  - 仅影响只属于该组、非组长、非超级管理员的成员账号；
  - 多小组成员不被覆盖；
  - 组长本人不被覆盖；
  - API 返回受影响账号数，前端提示影响范围。
- 超级管理员可以全局重置所有非超级管理员账号密码。
- 首期启用 MySQL 分区表 `checkin_records`，后端启动时会检查并提前创建未来分区。
- 打卡记录按 `group_id` 隔离，成员只能撤销本人最近 7 天记录；管理员纠错删除走管理接口并写审计。
- 资源页按读物 / 讲义 / 视频做物理分组；视频查看时会把同主题资料聚合到一个查看面板中。
- PDF 读物支持按页范围裁切；对于 `/range` 裁页 PDF，前端会从第 1 页开始渲染，避免默认跳到原始页码。

## 已完成的配套能力

- `backend/cmd/migrate-json/main.go`：旧 `config.json` / `records.json` 导入 MySQL 的 CLI。
- `scripts/deploy-oneclick.sh`：新环境一键部署，并可串联首组迁移。
- `scripts/migrate-group.sh`：已上线环境补迁其他组数据。
- 资源库后台会同时扫描静态目录 `Book/`、`Newtestament/`、`PPT/` 与数据库上传文件。

## 本地检查

```bash
cd backend
go build ./cmd/server

cd ..
cd frontend
npm install
npm run build

cd ..
docker compose -f deploy/docker-compose.separated.yml config
```

## 后续建议

- 继续把 `src/legacy-app.js` 中的业务计算和 API 动作迁入更清晰的 Pinia actions / composables。
- 补充围绕日期切换、资源连续阅读和统计页布局的自动化测试。
- 根据部署环境梳理 `Book/`、`Newtestament/`、`PPT/` 的挂载约定，并把 `/data/agp/assets` 纳入备份。
