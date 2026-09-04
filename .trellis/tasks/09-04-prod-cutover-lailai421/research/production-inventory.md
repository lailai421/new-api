# 生产盘点（只读核查，2026-09-04）

本文记录切换前在 `root@72.60.29.71`（hostname `srv945124`）上的只读核查结果。不包含密码、DSN、SESSION_SECRET 等密钥原文。

## 入口与进程

- 公网域名：`wj-api.mobikechip.com`、`wj-api.mobikechip.cn`
- Nginx 容器反代到 `http://172.17.0.1:3000`
- `new-api` 容器端口绑定：`127.0.0.1:3000` 与 `172.17.0.1:3000`
- 当前镜像：`calciumion/new-api:latest`
- 镜像版本标签：`org.opencontainers.image.version=v1.0.0-rc.25`
- `/api/status` 返回 `version=v1.0.0-rc.25`，`system_name=New API`
- 容器已运行约 2 天，健康检查为 healthy
- 进程：`/new-api --log-dir /app/logs`

## Compose 与目录

- 工作目录：`/srv/docker-migration/newapi`
- Compose 文件：`/srv/docker-migration/newapi/docker-compose.yml`
- 已有备份：`docker-compose.yml.bak_20260902_042742`
- 已有历史 dump：`fresh_new-api.dump`（2026-09-02，约 12MB，不能当作本次切换备份）
- Compose 项目名：`newapi`
- 服务名：`new-api` / `newapi-postgres` / `newapi-redis`
- `new-api` 额外网络：`newapi_new-api-network` + 外部网络 `cliproxyapi_default`
- `extra_hosts`: `host.docker.internal:host-gateway`

## 数据落点

- 主库：PostgreSQL 15，容器 `newapi-postgres`，库名 `new-api`，用户 `root`
- 数据卷：Docker volume `newapi_pg_data` → 容器内 `/var/lib/postgresql/data`
- 应用数据目录：`/srv/docker-migration/newapi/data` → 容器 `/data`（当前几乎为空，主数据在 Postgres）
- 日志目录：`/srv/docker-migration/newapi/logs` → 容器 `/app/logs`
- Redis：容器 `newapi-redis`，带 requirepass，供 `REDIS_CONN_STRING` 使用
- 已设置 `SESSION_SECRET`、`SQL_DSN`、`REDIS_CONN_STRING`、`NODE_NAME=new-api-node-1`、`BATCH_UPDATE_ENABLED=true`
- 未设置 `CRYPTO_SECRET`（运行时回退到 `SESSION_SECRET`）
- 未设置 `LOG_SQL_DSN`（日志与主库共用 Postgres）

## 业务数据规模（切换前快照）

- 数据库体积约 290 MB
- `users` 5 行
- `tokens` 22 行
- `channels` 6 行
- 现有业务表 34 张，**没有** `prompt_audit_events`
- 现有表也尚未出现 `login_encryption_keys`、`task_plugins`（新镜像启动时由 AutoMigrate 增量创建）

## 源码与镜像策略（当前）

- 生产 **没有** 源码仓库，只跑官方 Docker Hub 镜像
- 后续 `docker compose pull` 会继续拉 `calciumion/new-api:latest`，这正是需要切断的路径

## 服务器资源与工具

- 根盘约 394G，剩余约 165G，足够本地构建镜像
- 已安装 `git` 2.39.5、Docker 28.3.3
- 同机还有 nginx、mysql、redis、elasticsearch、其他业务容器；切换不得改动它们

## GitHub 源（本机核查）

- 目标仓库：https://github.com/lailai421/new-api （public，fork 自 QuantumNous/new-api）
- 默认分支：`main`
- 规划时远程 `origin/main` 已包含 `Merge branch 'QuantumNous:main' into main`（本工作区当时落后 13 个提交）
- 仓库内 `VERSION` 文件为空；官方发版靠 CI 写入。自建镜像必须在构建前写入可识别版本号
