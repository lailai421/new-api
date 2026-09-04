# 生产切换设计

## 边界

本次只替换 Compose 服务 `new-api` 的**应用镜像来源**，并固定后续更新跟踪 `lailai421/new-api`。

保持不动：

- Postgres 容器、数据卷 `newapi_pg_data`、库名 `new-api`
- Redis 容器与密码
- Nginx 容器及 `wj-api.mobikechip.com` / `wj-api.mobikechip.cn` 反代
- 端口 `127.0.0.1:3000`、`172.17.0.1:3000`
- 网络 `newapi_new-api-network` 与外部网络 `cliproxyapi_default`
- `extra_hosts: host.docker.internal:host-gateway`
- 环境变量里已有的 `SQL_DSN`、`SESSION_SECRET`、`REDIS_CONN_STRING`、`NODE_NAME`、`BATCH_UPDATE_ENABLED`

## 目标拓扑

```
Internet
  → nginx (443)
    → 172.17.0.1:3000
      → container new-api
           image: lailai421/new-api:local   # 由 ./src Dockerfile 构建
           env: 现有 SQL_DSN / SESSION_SECRET / REDIS
           volume: ./data, ./logs
      → container newapi-postgres (volume newapi_pg_data)
      → container newapi-redis
```

服务器目录约定：

```
/srv/docker-migration/newapi/
  docker-compose.yml          # 保留密钥，只改 new-api 的 build/image
  docker-compose.yml.bak_*    # 切换前备份
  backups/YYYYMMDD-HHMMSS/    # pg_dump 与状态快照
  src/                        # git clone https://github.com/lailai421/new-api.git
  data/
  logs/
  UPDATE.md                   # 后续更新步骤
```

`src/` 是生产构建用的只读跟踪副本，不在里面改业务代码。密钥只留在 compose，不进 `src/`。

## 数据流与兼容

1. 旧镜像 rc.25 与新 fork 都通过 `SQL_DSN` 连同一 Postgres。
2. 新进程启动走 `model.InitDB()` → `AutoMigrate`。预期行为是加表加列：
   - `prompt_audit_events`
   - `login_encryption_keys`
   - `task_plugins`
   - 以及上游合并可能带来的其它空列
3. `InitializeUserAuthVersions` 对 `auth_version` 空值回填为 1，幂等。
4. `InitPasswordEncryption` 在没有 `login_encryption_keys` 时生成新密钥并写入；这是浏览器登录加密信封密钥，不是用户密码哈希。用户表里的密码哈希不改。前端每次登录会取当前公钥，因此允许首次生成。
5. 提示词审计默认关闭，不改变现网转发。

不允许的兼容操作：手工改表、把 SQLite `one-api.db` 导入生产、用本地开发库覆盖生产。

## Compose 变更形状

`new-api` 服务从：

```yaml
image: calciumion/new-api:latest
```

改为：

```yaml
build:
  context: ./src
  dockerfile: Dockerfile
image: lailai421/new-api:local
```

其余 `container_name`、`command`、`ports`、`volumes`、`environment`、`depends_on`、`networks`、`healthcheck`、`extra_hosts`、`restart` 保持原样。

postgres / redis 服务定义不改。

## 切换顺序

1. 只读快照：用户/令牌/渠道计数、`/api/status` 版本、compose 哈希。
2. 备份：打镜像标签 `calciumion/new-api:pre-cutover-rc25`；复制 compose；`pg_dump` 到 `backups/`。
3. 克隆或更新 `src/` 到 GitHub `main`，记录 SHA。
4. 在 `src/VERSION` 写入 `lailai421-<shortsha>`（不 commit）。
5. 修改 compose 为 build 模式。
6. `docker compose build new-api`（此时旧容器仍在服务）。
7. `docker compose up -d --no-deps new-api`。
8. 等 healthcheck；核对 `/api/status`、登录、计数、审计默认关闭。
9. 写入 `UPDATE.md`。

## 回滚

失败且新容器不健康：

1. 把 compose 的 `build` 去掉，`image` 改回 `calciumion/new-api:pre-cutover-rc25`
2. `docker compose up -d --no-deps new-api`
3. 确认 `/api/status` 回到 `v1.0.0-rc.25`

只有在确认新代码写坏了业务行时，才停止写入并用 dump 恢复 Postgres。默认回滚不碰数据库。

## 后续更新

`UPDATE.md` 规定唯一更新路径：

```bash
cd /srv/docker-migration/newapi/src
git fetch origin
git checkout main
git pull --ff-only
echo "lailai421-$(git rev-parse --short HEAD)" > VERSION
cd /srv/docker-migration/newapi
docker compose build new-api
docker compose up -d --no-deps new-api
```

禁止：`docker compose pull new-api` 去拉官方 latest；禁止在 `src/` 直接热改后当正式发布。

## 风险

| 风险 | 处理 |
|---|---|
| AutoMigrate 启动变慢或失败 | 先备份；失败则滚回 rc.25 镜像 |
| 新代码比 rc.25 新，增加列后旧镜像难读 | 回滚应用通常仍可运行；若无法启动再评估 dump |
| 构建时间长 | 旧容器继续服务，构建完成再切 |
| VERSION 为空导致无法确认是否切成功 | 构建前写入 `lailai421-<sha>` |
| 误执行 `down -v` | 操作清单明确禁止；脚本不包含 `-v` |
| 提示词审计被误开 | 切换后检查配置为关闭 |
| 同机其它容器受影响 | 只操作 compose 项目 `newapi` 的 `new-api` 服务 |

## 权衡

选用服务器本地 build，而不是 CI 推镜像：少一个密钥和一个仓库，满足「以后以我这套 GitHub 为准」。代价是生产机构建一次。磁盘剩余约 165G，可接受。
