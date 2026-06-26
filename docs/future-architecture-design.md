# 前后端分离改造设计方案

本文档描述目标方案：将当前单进程、单小组、JSON 文件存储的打卡程序，改造成一个前后端分离、多小组、多账户、MySQL 持久化的系统。

## 1. 目标与原则

目标：

- 一个程序承载多个小组，不再通过多个端口和多个进程区分小组。
- 用户登录后，根据账户所属小组自动进入对应空间。
- 普通成员只看到自己小组的信息。
- 小组管理员只能管理自己小组的成员、任务、资料和打卡记录。
- 超级管理员可以创建小组、创建组长用户和组员用户、设置组长、添加组员和管理全部小组。
- 组长可以管理自己小组、创建本组组员用户、添加组员，并给某位组员授予本组管理员权限。
- 系统采用邀请制/白名单制，只有超级管理员或组长创建的用户可以登录网站；不允许用户自行注册。
- 每个账号只有一个实际登录密码，成员账号使用人员姓名拼音。
- 每个小组有一个默认密码，用于新建本组用户时初始化账号密码，也可按规则批量重置成员密码。
- 只有超级管理员或本人可以设置某个账号的个人密码；组长不能直接修改其他人的个人密码。
- 组长可以修改本组默认密码；超级管理员可以修改任意小组默认密码，并可执行全局统一密码重置。
- 前端为浏览器 Web 站点，同一套 Web 应用需要同时支持手机浏览器和电脑浏览器访问。
- 后端使用 Go 实现，数据库使用 MySQL。
- 针对打卡数据长期线性增长设计表结构、索引、分页和归档策略。
- NAS 仍然可以作为文件存储位置，但文件不再直接混放在代码目录中。

原则：

- 所有业务表都显式带 `group_id`，后端从登录态解析小组，不信任前端传入的 `group_id`。
- 打卡记录按时间增长，使用可分页查询、服务端聚合，并在首期启用 MySQL 分区与自动分区维护任务。
- 管理操作只更新真实变化的字段，减少无意义写入和审计噪音。
- 文件使用数据库记录元数据，NAS 只存二进制内容。
- Redis 不是首期必需组件；先用 MySQL + Go 进程内短缓存即可。后续访问量或多实例部署上来后再引入 Redis。

## 2. 总体架构

推荐目录形态：

```text
agp/
  frontend/              # Vue/React 前端项目
  backend/               # Go 后端服务
  deploy/
    docker-compose.yml
    mysql/
  docs/
  migrations/
```

运行架构：

```text
浏览器
  |
  | HTTPS
  v
Nginx / Caddy
  |-- /                 -> 前端静态文件
  |-- /api              -> Go API
  |-- /assets/private   -> Go 鉴权后读取 NAS 文件
  v
Go Backend
  |-- MySQL            -> 用户、组、任务、打卡、审计、文件元数据
  |-- NAS 文件目录      -> PDF、视频、图片、Markdown
```

部署在 NAS 上时，可以使用一个 `docker-compose.yml` 启动：

- `frontend`：构建后产物由 Nginx/Caddy 托管。
- `backend`：Go 编译出的单二进制服务。
- `mysql`：MySQL 8.0，数据卷挂载到 NAS。
- `redis`：可选，首期不启用。

## 3. 登录与小组分流

用户登录后，后端返回包含用户身份和默认小组的 token/session。登录入口只面向已创建用户，不提供公开注册。

账户模型：

- 用户由超级管理员或组长创建，默认账号为人员姓名拼音，例如 `zhangjiale`。若拼音重复，追加短后缀或数字，例如 `zhangjiale2`。
- 每个账号只有一个实际登录密码，统一保存在 `users.password_hash`。登录请求只需要 `username + password`，不要求未认证用户先选择小组，也不向未认证用户暴露其所属小组列表。
- 小组默认密码保存在 `study_groups.default_password_hash`，不是登录时额外校验的第二个密码，而是用于新建本组用户时初始化 `users.password_hash`，以及按规则批量重置成员密码。
- 超级管理员使用独立账号和独立密码登录，不受任何小组默认密码影响。
- 一个用户可以属于一个或多个小组，但必须由超级管理员或组长创建后才能登录。
- 若只属于一个小组，登录后直接进入该小组。
- 若属于多个小组，必须先用账号密码完成认证；认证成功后前端再显示小组选择器。这样不会在未认证状态下通过用户名枚举成员所属小组。
- 组长和小组管理员角色绑定在 `user_group_roles` 上，只对对应小组生效。
- 全局超级管理员身份不绑定具体小组，存放在 `users.is_super_admin`，不写入 `user_group_roles`。

身份与安全权衡：

- 默认新成员会继承小组默认密码，因此初始阶段仍然是弱身份认证：知道拼音账号和默认密码即可尝试登录。系统通过首次登录改密提示、账号级密码、登录限流与失败计数降低风险。
- 只有超级管理员或账号本人可以设置/修改某个账号的个人密码。组长不能直接改某位组员的个人密码，只能修改本组默认密码，并按下述规则影响部分成员账号。

小组默认密码规则：

- 创建小组时，系统自动生成 8 位字母 + 数字组合的默认密码，并保存其 hash 到 `study_groups.default_password_hash`。明文默认密码仅在创建结果中展示一次，便于超级管理员交给组长。
- 超级管理员创建 A 组并把 `a1` 加入 A 组且设置为组长时，若 `a1` 是新用户，则 `a1.password_hash` 初始化为 A 组默认密码。
- 组长 `a1` 可以修改 A 组默认密码。后续由 `a1` 在 A 组内创建的新用户，例如 `a2`，默认使用 A 组当前默认密码。
- 若组长修改本组默认密码，系统同步更新“仅属于该组、且不是组长、且不是超级管理员”的成员账号密码；多小组成员不会被修改，页面必须提示“多小组成员密码不会随本组默认密码变化”；组长自己的密码也不会被本组默认密码覆盖。
- 若超级管理员把已存在用户 `a2` 加入 B 组，`a2` 的账号密码不被 B 组默认密码覆盖，因为 `a2` 已经是已存在账号。
- 超级管理员可以通过修改统一密码的方式，将所有非超级管理员账号的密码重置为同一个统一密码；该操作是全局批量重置，必须二次确认并记录审计日志。

系统初始化（首个超级管理员）：

- 系统采用邀请制，无公开注册，因此首个超级管理员不能从界面创建，需通过引导方式注入。
- 启动时若不存在任何超级管理员，后端读取环境变量（如 `BOOTSTRAP_SUPERADMIN_USERNAME`、`BOOTSTRAP_SUPERADMIN_PASSWORD`）创建一个 `is_super_admin=1` 的账号；或由迁移 CLI 写入种子账号。
- 引导账号首次登录后必须强制修改密码，环境变量随后可移除。

权限模型：

- `super_admin`：可以创建小组、设置/修改任意小组默认密码、创建组长用户和组员用户、把用户加入任意小组、设置/取消组长、重置任意账号密码、管理全部小组配置和数据。
- `group_leader`：可以管理自己小组的配置、成员、周计划、学习内容、资料和打卡记录；可以创建本组组员用户、把用户加入本组、给本组组员授予或取消小组管理员权限、修改本组默认密码；不能创建超级管理员，不能设置其他组的组长，不能管理其他小组，不能直接修改组员个人密码。
- `group_admin`：由组长授予，只能管理本组部分配置、成员、任务、资料和打卡记录；不能修改小组密码，不能继续授予管理员权限，不能创建组长。
- `member`：只能查看和操作自己所属小组内允许访问的内容。

组长创建和授权组员时的处理：

- 若用户不存在，组长可以创建本组组员用户，系统按成员姓名拼音生成账号，并自动加入本组。
- 若用户已存在，组长可以把该用户加入本组，但不能修改该用户在其他小组的角色。
- 组长可以把本组某位组员提升为 `group_admin`，也可以取消该组员的本组管理员权限。
- 组长不能创建或授予 `group_leader`、`super_admin`。

后端处理规则：

- 普通接口从 token/session 中取 `current_group_id`。
- 管理接口按操作类型检查当前用户在该 `group_id` 下是否具备 `group_admin`、`group_leader` 或更高权限。
- 超级管理员接口必须检查 `users.is_super_admin = 1`。
- 登录接口只返回模糊错误，例如“账号或密码错误”，不得在未认证状态下暴露账号是否存在、所属小组列表或小组数量。
- 登录接口必须有限流与失败锁定：按 IP 和按账号分别计数，连续失败超过阈值后短时锁定或加验证码。首期可用 Go 进程内计数实现，多实例部署后迁移到 Redis。
- 查询和写入都追加 `WHERE group_id = ?`。
- 禁止通过 URL 或请求体传入任意 `group_id` 后直接访问跨组数据。

## 4. 前端设计

建议使用 Vue 3 + TypeScript + Vite，或 React + TypeScript + Vite。考虑当前是单页应用且管理页面较多，推荐 Vue 3，表单和页面状态会更直接。

前端定位为手机浏览器和电脑浏览器共用的响应式 Web 应用，不开发原生 App，也不拆成单独的移动端工程。用户在手机上同样通过浏览器访问同一个站点，前端根据屏幕宽度、触控能力和视口方向调整布局。打卡、阅读、视频、个人记录等高频功能必须优先适配手机浏览器；管理后台、批量编辑、统计分析等复杂功能在电脑浏览器提供完整体验，同时在手机浏览器保留可用但简化的操作路径。

页面规划：

- 登录页：账号、密码、记住登录、首次初始化入口。
- 小组选择页：仅多小组用户展示。
- 首页仪表盘：今日打卡、周任务进度、小组概览、成员列表。
- 打卡页：每日灵修、周读物、视频、背经、默写、得着分享。
- 成员个人页：日历视图、历史记录、连续天数、任务完成情况。
- 资源中心：按周、类别、关键词查看 PDF、视频、图片、Markdown。
- 管理后台：
  - 成员管理
  - 周计划管理
  - 学习内容管理
  - 资源上传与绑定
  - 打卡记录查询和纠错
  - 数据统计
  - 操作审计

视觉方向：

- 首页采用“学习进度卡片 + 今日行动区 + 成员完成矩阵”的结构。
- 大屏使用卡片、渐变背景、细边框和轻阴影；移动端优先保证打卡按钮清晰。
- 管理后台使用表格、抽屉表单、批量操作和筛选器，避免把所有配置塞进一个长页面。

响应式设计要求：

- 手机浏览器以单列信息流为主，底部 Tab 放置首页、打卡、资源、我的等高频入口。
- 电脑端使用左侧导航 + 内容区布局，充分利用横向空间展示成员矩阵、统计图和管理表格。
- 断点建议：`< 640px` 为手机，`640px - 1024px` 为平板/窄屏，`>= 1024px` 为桌面。
- 所有打卡按钮、日期切换、弹窗关闭按钮满足触控尺寸，建议点击区域不小于 `44px`。
- 表格在手机浏览器改为卡片列表，筛选条件折叠到抽屉或顶部筛选栏。
- 视频、PDF、Markdown 阅读器需要适配手机浏览器竖屏；PDF 在手机浏览器提供下载/新窗口打开兜底。
- 管理后台的批量导入、批量编辑、复杂图表优先保证电脑浏览器效率，手机浏览器允许降级为查看和少量编辑。
- 同一接口返回结构保持一致，前端只根据屏幕宽度调整展示方式，不为手机和电脑维护两套业务逻辑。

前端不要再一次性拉取全部记录。建议接口按场景拆分：

- 今日状态：只取当前日期或当前周。
- 成员日历：按成员 + 月份查询。
- 统计图：由后端返回聚合结果。
- 管理列表：分页、按日期范围、成员、任务类型筛选。

## 5. Go 后端设计

推荐技术栈：

- Web 框架：Gin。NAS 自部署场景推荐 Gin，依赖少、部署简单。
- 数据库访问：`gorm`。开发速度优先，用 GORM。MySQL 使用 8.0。
- MySQL 驱动：`go-sql-driver/mysql`。
- 配置：环境变量 + YAML/TOML 配置文件，含超级管理员引导变量。
- 鉴权：JWT 或服务端 session。单实例 NAS 推荐 JWT + 短期过期；后续多实例可改为 Redis session。
- 登录防护：进程内限流 + 失败计数（按 IP、按账号），多实例后迁移到 Redis。
- 文件：后端统一读取 NAS 目录并做鉴权，不让 Nginx 直接暴露全部资料。

后端模块：

- `auth`：登录、密码、token、当前用户和小组上下文。
- study_groups：小组、角色、成员。
- `tasks`：学习任务、周计划、任务项。
- `checkins`：打卡、撤销、纠错、查询、统计。
- `assets`：资源上传、元数据、访问 URL。
- `admin`：管理后台接口。
- `audit`：管理操作审计。
- `migration`：旧 JSON 数据迁移。

## 6. MySQL 表结构设计

以下是核心表设计。字段类型可在实现时按 ORM/SQL 迁移工具微调。

### 6.1 小组与用户

```sql
CREATE TABLE study_groups (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  code VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description VARCHAR(512) NOT NULL DEFAULT '',
  default_password_hash VARCHAR(255) NOT NULL DEFAULT '',
  status TINYINT NOT NULL DEFAULT 1,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE users (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  username VARCHAR(64) NOT NULL UNIQUE,
  display_name VARCHAR(128) NOT NULL,
  name_pinyin VARCHAR(128) NOT NULL,
  password_hash VARCHAR(255) NOT NULL DEFAULT '',
  is_super_admin TINYINT NOT NULL DEFAULT 0,
  email VARCHAR(255) NOT NULL DEFAULT '',
  phone VARCHAR(32) NOT NULL DEFAULT '',
  status TINYINT NOT NULL DEFAULT 1,
  created_by BIGINT UNSIGNED NULL,
  last_login_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_pinyin (name_pinyin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE group_members (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  member_name VARCHAR(128) NOT NULL,
  status TINYINT NOT NULL DEFAULT 1,
  joined_at DATETIME(3) NOT NULL,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_group_user (group_id, user_id),
  KEY idx_user_status (user_id, status),
  KEY idx_group_status_name (group_id, status, member_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE user_group_roles (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  role VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_group_user_role (group_id, user_id, role),
  KEY idx_user_role (user_id, role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

字段说明：

- `study_groups.default_password_hash`：小组默认密码 hash。它不参与登录二次校验，只用于新建本组用户时初始化 `users.password_hash`，以及按规则批量重置成员密码。组长可改本组默认密码，超级管理员可改任意小组默认密码。
- `users.username`：登录账号，默认由姓名拼音生成。
- `users.name_pinyin`：姓名拼音，便于检索和处理重名。
- `users.display_name`：全局显示名；小组内可由 `group_members.member_name` 覆盖，便于同名成员在不同小组使用不同称呼。
- `users.password_hash`：账号唯一登录密码 hash。创建新用户时默认来自其初始小组的默认密码；之后只有超级管理员或账号本人可以直接修改该账号密码。
- `users.is_super_admin`：全局超级管理员标记。该身份不绑定具体小组，不写入 `user_group_roles`；后端超级管理员接口据此鉴权。
- `users.created_by`：创建该登录用户的操作者。后端要求可登录用户只能由超级管理员或组长创建。
- `group_members.created_by`：把用户加入小组的操作者，可以是超级管理员或该组组长。

角色建议：

- `member`：普通成员。
- `group_admin`：小组管理员，由组长授予，只能管理本组，不能改组密码。
- `group_leader`：组长，可以管理自己小组、创建本组组员用户、授予本组管理员权限、修改本组默认密码。
- 超级管理员不属于 `user_group_roles` 的某个 `role`，而是由 `users.is_super_admin=1` 表示，可管理全部小组、组长用户、组员用户和组长设置。

### 6.2 小组配置

```sql
CREATE TABLE group_settings (
  group_id BIGINT UNSIGNED PRIMARY KEY,
  site_title VARCHAR(128) NOT NULL DEFAULT '',
  brand_name VARCHAR(128) NOT NULL DEFAULT '',
  hero_kicker VARCHAR(128) NOT NULL DEFAULT '',
  hero_desc VARCHAR(512) NOT NULL DEFAULT '',
  dashboard_title VARCHAR(128) NOT NULL DEFAULT '',
  button_labels JSON NULL,
  settings JSON NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

这张表承接当前 `config.json` 中的站点文案、按钮名称、任务区域配置等非高频数据。

### 6.3 周计划与任务项

```sql
CREATE TABLE study_weeks (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  start_date DATE NOT NULL,
  end_date DATE NOT NULL,
  title VARCHAR(512) NOT NULL DEFAULT '',
  verse_ref VARCHAR(255) NOT NULL DEFAULT '',
  recite_text TEXT NULL,
  book_enabled TINYINT NOT NULL DEFAULT 1,
  video_enabled TINYINT NOT NULL DEFAULT 1,
  verse_enabled TINYINT NOT NULL DEFAULT 1,
  outline_enabled TINYINT NOT NULL DEFAULT 1,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_group_week (group_id, start_date, end_date),
  KEY idx_group_start (group_id, start_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE study_tasks (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  week_id BIGINT UNSIGNED NULL,
  task_type VARCHAR(32) NOT NULL,
  title VARCHAR(512) NOT NULL,
  content TEXT NULL,
  required TINYINT NOT NULL DEFAULT 1,
  enabled TINYINT NOT NULL DEFAULT 1,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_group_week_type (group_id, week_id, task_type),
  KEY idx_group_type_enabled (group_id, task_type, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

`task_type` 建议枚举：

- `daily_devotion`
- `daily_scripture`
- `weekly_book`
- `weekly_video`
- `weekly_verse`
- `weekly_outline`
- `share`
- `reflection`（得着分享，对应旧数据 `kind=reflection`）
- `recite_exam`（默写，主记录写入 `checkin_records`，明细写入 `recite_attempts`）

### 6.4 文件资源

```sql
CREATE TABLE assets (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  category VARCHAR(32) NOT NULL,
  title VARCHAR(255) NOT NULL,
  original_name VARCHAR(255) NOT NULL,
  storage_path VARCHAR(1024) NOT NULL,
  mime_type VARCHAR(128) NOT NULL DEFAULT '',
  file_size BIGINT UNSIGNED NOT NULL DEFAULT 0,
  checksum_sha256 CHAR(64) NOT NULL DEFAULT '',
  visibility VARCHAR(32) NOT NULL DEFAULT 'group',
  created_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_group_category (group_id, category),
  KEY idx_group_title (group_id, title)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE task_assets (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  task_id BIGINT UNSIGNED NOT NULL,
  asset_id BIGINT UNSIGNED NOT NULL,
  usage_type VARCHAR(32) NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_task_asset_usage (task_id, asset_id, usage_type),
  KEY idx_group_task (group_id, task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

NAS 文件目录建议：

```text
/volume1/agp-data/
  assets/
    group-a/
      book/
      ppt/
      video/
      mentor/
      outline/
      markdown/
  backups/
  mysql/
```

数据库只保存 `storage_path` 和元数据。访问文件时走后端接口，例如 `GET /api/assets/{id}/download`，由后端校验用户是否属于该小组。

`assets.checksum_sha256` 仅用于完整性校验和重复检测提示，不做跨组去重：各小组按 `group_id` 独立存储文件，即使内容相同也分别保存，以保证小组隔离和迁移独立性。

### 6.5 打卡记录

打卡记录是增长最快的表，需要单独设计。

```sql
CREATE TABLE checkin_records (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  task_id BIGINT UNSIGNED NULL,
  week_id BIGINT UNSIGNED NULL,
  logical_date DATE NOT NULL,
  checkin_time DATETIME(3) NOT NULL,
  task_type VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'done',
  is_retro TINYINT NOT NULL DEFAULT 0,
  detail VARCHAR(1024) NOT NULL DEFAULT '',
  note TEXT NULL,
  part VARCHAR(64) NOT NULL DEFAULT '',
  source VARCHAR(32) NOT NULL DEFAULT 'web',
  active_key BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  deleted_at DATETIME(3) NULL,
  PRIMARY KEY (id, logical_date),
  UNIQUE KEY uk_one_active_checkin (group_id, user_id, task_type, logical_date, part, active_key),
  KEY idx_group_date_type (group_id, logical_date, task_type),
  KEY idx_group_user_date (group_id, user_id, logical_date),
  KEY idx_group_week_type (group_id, week_id, task_type),
  KEY idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
PARTITION BY RANGE COLUMNS(logical_date) (
  PARTITION p2026q1 VALUES LESS THAN ('2026-04-01'),
  PARTITION p2026q2 VALUES LESS THAN ('2026-07-01'),
  PARTITION p2026q3 VALUES LESS THAN ('2026-10-01'),
  PARTITION p2026q4 VALUES LESS THAN ('2027-01-01'),
  PARTITION pmax VALUES LESS THAN (MAXVALUE)
);
```

说明：

- `group_id` 是隔离字段。
- `logical_date` 是业务日期，用于补签、日历、周统计和分区。计算时固定使用 `Asia/Shanghai` 时区（与旧系统一致），后端统一按该时区换算逻辑日期，避免跨零点补签和连续天数统计出现偏差。
- `checkin_time` 是真实提交时间。
- `task_type` 取代当前 `daily/book/video/verse` 多列模式，后续新增任务类型不需要改表。
- `part` 用于区分同一天同类型的多个读物，例如书 1、书 2。长度限制为 64，控制 `uk_one_active_checkin` 二级索引体积；若未来出现更长取值，改用 `part_hash` 入唯一键。
- `deleted_at` 支持软删除，便于审计和恢复。
- `active_key` 用于唯一约束：有效记录固定为 `0`；软删除时更新为该记录 `id`，这样同一任务可重新打卡，同时仍保证有效记录唯一。
- 首期即启用 MySQL 分区，按季度或月份维护；索引必须按 `group_id + date` 设计。
- 唯一键和主键都包含 `logical_date`，满足 MySQL 分区表「唯一键必须包含分区列」的要求。

分区维护（运维前置条件）：

- `pmax` 只是兜底分区，不能依赖它长期承载新数据，否则分区裁剪失效、查询退化为全表扫描。
- 必须有定时任务（Go 定时任务或 NAS cron）按月/季度提前 `ALTER TABLE ... REORGANIZE PARTITION pmax INTO (...)` 新建未来分区。
- 同一定时任务负责归档/转储超过保留期的老分区（见数据增长策略）。
- 首期必须交付自动分区维护任务，并在部署时验证未来至少 2 个周期的分区已提前创建。

### 6.6 默写记录

默写记录不建议继续塞进普通打卡表的扩展中文字段，应该拆表。

```sql
CREATE TABLE recite_attempts (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  week_id BIGINT UNSIGNED NULL,
  checkin_record_id BIGINT UNSIGNED NULL,
  verse_ref VARCHAR(255) NOT NULL,
  logical_date DATE NOT NULL,
  blank_percent TINYINT NOT NULL,
  blank_count INT NOT NULL,
  correct_count INT NOT NULL,
  accuracy TINYINT NOT NULL,
  score INT NOT NULL,
  attempt_no INT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_attempt (group_id, user_id, week_id, verse_ref, attempt_no),
  KEY idx_group_ref_score (group_id, verse_ref, score),
  KEY idx_group_user_date (group_id, user_id, logical_date),
  KEY idx_group_week (group_id, week_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

说明：

- `attempt_no` 是同一人同一周、同一段经文的第几次默写。不要在应用层「查最大值 +1」后直接插入，并发提交会拿到相同序号。
- 通过 `uk_attempt` 唯一约束保证序号不重复：插入冲突时后端自增重试，或改用插入后回填的串行策略。

### 6.7 反馈与审计

```sql
CREATE TABLE feedbacks (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NULL,
  user_id BIGINT UNSIGNED NULL,
  name VARCHAR(128) NOT NULL DEFAULT '',
  contact VARCHAR(255) NOT NULL DEFAULT '',
  message TEXT NOT NULL,
  page VARCHAR(255) NOT NULL DEFAULT '',
  user_agent VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_group_created (group_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE audit_logs (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NULL,
  actor_user_id BIGINT UNSIGNED NOT NULL,
  action VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id BIGINT UNSIGNED NULL,
  before_json JSON NULL,
  after_json JSON NULL,
  ip VARCHAR(64) NOT NULL DEFAULT '',
  user_agent VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_group_created (group_id, created_at),
  KEY idx_actor_created (actor_user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

## 7. 数据增长策略

打卡数据会随时间线性增长，建议从首期就做到：

- 所有记录查询必须带日期范围或分页。
- 管理后台默认只查最近 30 天。
- 首页只查今日、当前周和必要的聚合结果。
- 成员日历按月份查，不拉全量。
- 图表统计由服务端 SQL 聚合，不在前端遍历全部 records。
- `checkin_records` 建立 `group_id + logical_date`、`group_id + user_id + logical_date` 索引。
- 首期即启用按月/季度分区，并实现自动分区维护任务；数据量达到百万级后评估是否增加汇总表或归档表。
- 老数据可归档到 `checkin_records_archive` 或通过分区转储。

服务端聚合示例：

- 今日完成情况：`WHERE group_id=? AND logical_date=?`
- 本周任务完成：`WHERE group_id=? AND logical_date BETWEEN ? AND ?`
- 成员月历：`WHERE group_id=? AND user_id=? AND logical_date BETWEEN month_start AND month_end`
- 连续天数：查询用户最近 N 天每日是否存在 `daily_devotion`。

## 8. Redis 判断

首期不建议强依赖 Redis，原因：

- NAS 单实例部署下，MySQL 足够承载主要读写。
- JWT 可实现无状态登录，不需要 session 存储。
- 小组配置、当前周计划可以用 Go 进程内缓存，更新后主动失效。
- 引入 Redis 会增加部署和备份复杂度。

后续满足以下条件再引入 Redis：

- 多个 Go 后端实例，需要共享 session 或分布式限流。
- 统计接口压力明显，MySQL 聚合成为瓶颈。
- 需要验证码、登录失败计数、短期一次性 token。
- 文件下载签名 URL、排行榜等热点数据需要短 TTL 缓存。

若引入 Redis，建议只缓存可重建数据，不缓存唯一事实来源。MySQL 仍然是主存储。

## 9. API 设计草案

认证：

- `POST /api/auth/login`：登录失败按 IP 和账号限流。
- `POST /api/auth/logout`
- `GET /api/auth/me`
- `POST /api/auth/switch-group`
- `POST /api/auth/change-password`：账号本人修改自己的登录密码；引导超级管理员首次登录强制走此接口。

首页：

- `GET /api/app/bootstrap`：当前用户、小组、站点配置、今日状态、当前周计划。
- `GET /api/dashboard/summary?from=&to=`
- `GET /api/members`

打卡：

- `POST /api/checkins`
- `DELETE /api/checkins/{id}`：成员只能撤销本人、当前小组、允许撤销窗口内的记录；管理员纠错删除使用管理接口并写审计。
- `GET /api/checkins?from=&to=&user_id=&task_type=&page=&page_size=`
- `GET /api/members/{id}/calendar?month=2026-06`

周计划与任务：

- `GET /api/study-weeks/current`
- `GET /api/study-weeks`
- `POST /api/admin/study-weeks`
- `PUT /api/admin/study-weeks/{id}`
- `DELETE /api/admin/study-weeks/{id}`

资源：

- `GET /api/assets?category=&keyword=&page=`
- `POST /api/admin/assets`
- `GET /api/assets/{id}/download`
- `POST /api/admin/tasks/{id}/assets`

管理（组长 / 小组管理员，作用于当前登录小组）：

- `GET /api/admin/members`：列出本组成员。
- `POST /api/admin/members`：在本组新建组员。`{ create_user: true }` 表示同时创建可登录用户（按拼音生成 `username`）并加入本组；`{ user_id }` 表示把已存在用户加入本组。两种语义二选一，后端据此区分。
- `PUT /api/admin/members/{member_id}`：编辑本组成员信息（`member_id` 为 `group_members.id`）。
- `POST /api/admin/members/{member_id}/admins`：授予该成员本组 `group_admin`（仅组长）。
- `DELETE /api/admin/members/{member_id}/admins`：取消该成员本组 `group_admin`（仅组长）。
- `PUT /api/admin/group/default-password`：修改本组默认密码（仅组长，作用于当前小组）。同步更新仅属于本组、非组长、非超级管理员的成员账号密码；多小组成员与组长账号不被覆盖，页面必须提示影响范围。
- `DELETE /api/admin/checkins/{id}`：管理员纠错删除本组打卡记录，必须写审计日志。
- `GET /api/admin/audit-logs`
- `GET /api/admin/storage/status`

说明：组长创建可登录用户和组员，均限定在当前登录小组内；新建组员账号的初始密码来自本组当前默认密码。

超级管理员（需 `users.is_super_admin = 1`）：

- `GET /api/super-admin/groups`
- `POST /api/super-admin/groups`
- `PUT /api/super-admin/groups/{id}`
- `POST /api/super-admin/groups/{id}/default-password`：修改任意小组默认密码，默认按组长规则影响该组成员。
- `POST /api/super-admin/users/reset-all-passwords`：将所有非超级管理员账号密码重置为同一个统一密码，必须二次确认并写审计日志。
- `GET /api/super-admin/users`
- `POST /api/super-admin/users`：创建组长用户或组员用户，可指定初始小组、角色和初始密码；未指定时使用初始小组默认密码。
- `PUT /api/super-admin/users/{id}`：编辑用户，可重置账号密码、停用账号。
- `POST /api/super-admin/groups/{id}/members`：把已存在用户加入指定小组。
- `POST /api/super-admin/groups/{id}/leaders`：设置某用户为该组组长。
- `DELETE /api/super-admin/groups/{id}/leaders/{user_id}`：取消组长。

## 10. 旧数据迁移

迁移步骤：

1. 初始化数据库 schema，并创建首个超级管理员（`is_super_admin=1`），来源为环境变量种子或 CLI 参数。
2. 为每个现有端口/进程建立一个 study_groups 记录。
3. 从每份 `config.json` 导入小组配置、成员、周计划。
4. 为成员创建 `users` 和 `group_members`。用户名默认使用姓名拼音生成（重名追加后缀）；成员初始密码来自其所在小组默认密码；每个旧小组设置一个默认密码，并写入 `study_groups.default_password_hash`。
5. 从 `records.json` 导入 `checkin_records`：
   - `daily = done` 转为 `task_type = daily_devotion`
   - `book = done` 转为 `task_type = weekly_book`
   - `video = done` 转为 `task_type = weekly_video`
   - `verse = done` 转为 `task_type = weekly_verse`
   - `kind = reflection` 转为 `task_type = reflection`
   - `kind = recite_exam` 同时写入 `checkin_records`（`task_type = recite_exam`）和 `recite_attempts`
6. 扫描 `Book/`、`PPT/`、`Newtestament/`、`Mentor/` 等目录，写入 `assets`，再根据周计划绑定 `task_assets`。
7. 对每个小组核对成员数、周计划数、记录数和最近 30 天统计。

迁移程序建议写成 Go CLI：

```text
backend/cmd/migrate-json
  --group-code agape
  --group-name AGAPE
  --config ./config.json
  --records ./data/records.json
  --assets-root /volume1/agp-data/assets/agape
```

## 11. 实施阶段

建议分阶段推进：

1. 搭建 Go 后端、MySQL schema、登录和小组模型。
2. 实现旧 JSON 迁移 CLI，先保证现有数据可导入。
3. 实现前端新版基础框架、登录、小组首页和打卡。
4. 实现管理后台成员、周计划、资源管理。
5. 实现统计接口、成员日历、默写排行榜。
6. 部署到 NAS，使用一个端口对外服务，灰度迁移一个小组。
7. 迁移其他小组，停掉旧的多端口进程。

## 12. 关键风险与处理

- 数据重复：通过 `uk_one_active_checkin` 约束避免同一人同一天同任务重复有效打卡。
- 跨组越权：后端统一从登录态取 `group_id`，所有查询强制带组条件。
- 弱身份认证：新成员初始密码来自小组默认密码，若成员不改密仍可能被冒充；缓解手段为首次登录改密提示、超级管理员/本人可重置账号密码、登录限流与失败锁定。
- 暴力破解：账号可枚举，登录接口必须做 IP/账号级限流和失败计数（首期进程内，多实例后用 Redis），并始终返回模糊错误。
- 超级管理员引导：首个超级管理员由环境变量/CLI 种子创建，首次登录强制改密，引导变量随后移除。
- 分区失效：`pmax` 兜底分区需配套定时任务提前建分区并归档老分区，否则查询退化为全表扫描。
- 大文件访问：文件下载走后端鉴权，视频可以支持 HTTP Range。
- 资料路径变化：迁移时保留旧 URL 到新 asset 的映射，前端不直接依赖物理路径。
- 统计变慢：首期 SQL 聚合，后续增加日维度汇总表或 Redis 缓存。
- NAS 备份：MySQL 数据卷和 `/volume1/agp-data/assets` 需要纳入 NAS 备份策略。
