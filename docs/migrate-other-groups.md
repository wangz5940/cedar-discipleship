# 其他组数据迁移方案

这个方案用于首套平台已经上线后，把其他组的旧数据继续迁入当前平台。

独立脚本入口：

```bash
./scripts/migrate-group.sh
```

它和首次部署脚本解耦，适合后续按组逐批迁移。

## 适用前提

先确保平台已经在目标环境运行：

```bash
docker compose -p agp -f deploy/docker-compose.separated.yml ps
```

至少要保证：

- `mysql` 已启动
- `backend` 已启动

## 迁移原则

每个组单独迁移，单独指定：

- `GROUP_CODE`
- `GROUP_NAME`
- `config.json`
- `records.json`

这样可以做到：

1. 每组独立 dry-run
2. 每组独立导入
3. 出问题时只影响当前组
4. 迁移报告能按组留档

## 标准流程

### 1. 准备源数据

建议每个待迁移小组准备一套目录，例如：

```text
/migration-inputs/agape-b/
  ├── config.json
  └── records.json
```

其中：

- `config.json`：该组的成员、周任务、学习配置
- `records.json`：该组历史打卡

### 2. 先 dry-run

```bash
cd /path/to/agp

GROUP_CODE='agape-b' \
GROUP_NAME='AGAPE B组' \
CONFIG_PATH='/migration-inputs/agape-b/config.json' \
RECORDS_PATH='/migration-inputs/agape-b/records.json' \
GROUP_DEFAULT_PASSWORD='Abc12345' \
./scripts/migrate-group.sh
```

默认只执行 dry-run，不会写数据库。

### 3. 检查迁移报告

报告默认输出到：

```text
data/migration-reports/
```

重点检查：

- `generated_usernames`
- `warnings`
- `failures`
- 周任务、资源、成员数是否符合预期

### 4. 正式导入

```bash
GROUP_CODE='agape-b' \
GROUP_NAME='AGAPE B组' \
CONFIG_PATH='/migration-inputs/agape-b/config.json' \
RECORDS_PATH='/migration-inputs/agape-b/records.json' \
GROUP_DEFAULT_PASSWORD='Abc12345' \
EXECUTE_IMPORT=true \
./scripts/migrate-group.sh
```

脚本会自动执行：

1. dry-run
2. 正式导入

## 推荐的批量迁移节奏

不要多个组并发导入。建议串行：

1. A 组 dry-run
2. A 组正式导入
3. A 组登录验收
4. B 组 dry-run
5. B 组正式导入
6. B 组登录验收

这样更容易定位问题。

## 账号生成策略

为了避免不同组之间账号冲突，建议遵循：

1. 先执行 dry-run，检查迁移报告中的 `generated_usernames`
2. 平台账号优先使用内置用户名映射和标准化规则生成
3. 账号一旦导入，后续不要重复改动

## 冲突处理建议

### 1. 组编码冲突

现象：

- `GROUP_CODE` 已存在

处理：

- 不要覆盖旧组
- 改成新的稳定编码，例如 `agape-b`

### 2. 成员账号冲突

现象：

- 自动生成的用户名不符合预期

处理：

- 先做 dry-run，检查 `generated_usernames`
- 如需调整，修正源数据后重新导入

### 3. 重复打卡

默认行为：

- 重复有效打卡会跳过

如果你想保留重复记录为软删除历史：

```bash
ALLOW_DUPLICATE_AS_DELETED=true
```

### 4. 自动生成用户名不可信

如果你希望一旦需要自动生成用户名就直接失败：

```bash
FAIL_ON_GENERATED_USERNAMES=true
```

## 验收清单

正式迁移后，建议逐项确认：

1. 小组已出现在管理后台
2. 成员名单正确
3. 管理员 / 组长角色正确
4. 学习内容配置正确
5. 周任务和挂载资源正确
6. 月历和历史打卡记录数量合理
7. 资源打开路径正常

## 回滚建议

迁移前先做一次数据库备份：

```bash
mkdir -p data/backups/mysql
docker exec agp-mysql mysqldump -uagp -pagp agp > data/backups/mysql/agp-before-group-import-$(date +%F-%H%M%S).sql
```

如果某个组导入结果不对，建议优先：

1. 停止新的迁移
2. 保留迁移报告
3. 基于备份回滚数据库
4. 修正源数据后重新导入
