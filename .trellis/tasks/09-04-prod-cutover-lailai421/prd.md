# 将生产 new-api 切换到 lailai421 仓库并保留用户数据

## Goal

把 `72.60.29.71` 上正在服务团队的 new-api，从官方镜像 `calciumion/new-api:latest` 切换为 GitHub 仓库 `lailai421/new-api` 的 `main` 构建结果。现有用户、令牌、渠道、额度、日志和登录会话密钥必须保留。切换完成后，这条生产环境的后续更新只跟踪 `lailai421/new-api`，不再拉取官方镜像。

## Background

- 目标机：`root@72.60.29.71`，hostname `srv945124`。
- 公网入口：Nginx 反代 `wj-api.mobikechip.com` 与 `wj-api.mobikechip.cn` 到 `172.17.0.1:3000`。
- 当前运行：Docker Compose 项目 `newapi`，目录 `/srv/docker-migration/newapi`，应用镜像 `calciumion/new-api:latest`（`v1.0.0-rc.25`）。
- 主数据在 PostgreSQL 15（容器 `newapi-postgres`，volume `newapi_pg_data`），库体积约 290MB；现有 `users=5`、`tokens=22`、`channels=6`。
- 应用容器挂载 `/srv/docker-migration/newapi/data` 和 `logs`；真正的用户数据不在 SQLite，也不在该 data 目录。
- 已配置 `SESSION_SECRET`、`SQL_DSN`、`REDIS_CONN_STRING`；未单独配置 `CRYPTO_SECRET`。
- 代码源：https://github.com/lailai421/new-api （公开 fork，默认分支 `main`）。规划时该远程已合并 QuantumNous 上游，并包含提示词审计等本地功能。
- 本工作区 `main` 在规划时落后 GitHub `origin/main` 13 个提交。生产切换以 GitHub `main` 为准，不以本工作区未拉取的 HEAD 为准。
- 团队正在使用该环境。切换必须可回滚，且不得清空或重建数据库。

## Requirements

- R1. 生产应用进程改为运行由 `https://github.com/lailai421/new-api` 的 `main` 在服务器上构建出的镜像，而不是 `calciumion/new-api:latest`。
- R2. 保留现有 Postgres 数据卷、Redis、`SQL_DSN`、`SESSION_SECRET`、`REDIS_CONN_STRING`、端口绑定、Nginx 反代和 `cliproxyapi` 外部网络。禁止新建空库替换。
- R3. 切换前必须留下可回滚材料：当前官方镜像本地标签、compose 文件副本、最新 Postgres dump。
- R4. 切换采用「先构建新镜像、再只重启 `new-api` 容器」。Postgres 与 Redis 容器不得 `down -v`，不得删除 `newapi_pg_data`。
- R5. 新进程启动后 `/api/status` 必须可访问，版本字段能识别为 `lailai421-<git-sha>`，不再显示官方 `v1.0.0-rc.25`。
- R6. 切换后现有用户可登录，现有令牌、渠道、分组和额度记录数量与切换前快照一致（允许 AutoMigrate 新增空表/新列，不允许业务行丢失）。
- R7. 提示词审计代码可以随镜像带上，但切换当天不得把审计门禁改成开启；现有请求转发行为不得因为这次切换而被强制拦截。
- R8. 后续更新流程必须文档化并落在服务器：从 `lailai421/new-api` `git pull --ff-only`，本地 build，再只替换 `new-api` 容器。compose 中不得再出现会 `pull` 官方 `calciumion/new-api:latest` 的默认 image。
- R9. 若新容器健康检查失败或登录/核心 API 不可用，能在不恢复 dump 的情况下把应用容器滚回 `pre-cutover-rc25` 镜像。
- R10. 任务文档与操作记录不得写入数据库密码、SESSION_SECRET、DSN 等密钥原文。

## Acceptance Criteria

- [x] AC1. `docker inspect new-api` 的镜像不再是未加本地标签的 `calciumion/new-api:latest` 作为运行来源；运行中的镜像由 `/srv/docker-migration/newapi/src` 对应的 `lailai421/new-api` 提交构建。
- [x] AC2. 公网 `https://wj-api.mobikechip.cn` 的 `/api/status` 返回 `success=true`，且 version 含 `lailai421-` 与构建时的 git short SHA（`lailai421-6651c38`；`.com` 域名已确认废弃）。
- [x] AC3. 切换后 Postgres 中 `users` / `tokens` / `channels` 行数与切换前快照一致；`newapi-postgres` 容器与 `newapi_pg_data` 未被重建。
- [ ] AC4. 使用现有管理员账号可以登录后台；随机抽查至少 1 个现有令牌调用仍指向原渠道配置，而不是空配置（数据库配置与映射已确认完好，待管理员浏览器实际登录复核）。
- [x] AC5. 服务器上存在：`calciumion/new-api:pre-cutover-rc25` 镜像标签、带时间戳的 compose 备份、带时间戳的 `pg_dump`。
- [x] AC6. `/srv/docker-migration/newapi/docker-compose.yml` 的 `new-api` 服务使用 `build` 指向 `./src`，不再把官方 Docker Hub latest 当作更新来源。
- [x] AC7. `/srv/docker-migration/newapi` 中有可执行的更新说明或脚本：pull `lailai421/new-api` main → 写入 VERSION → build → `up -d --no-deps new-api`。
- [x] AC8. 提示词审计保持关闭（或等效于不拦截现有分组流量）；后台可以看到审计页面，但不会因为默认开启而阻断团队正在使用的请求。
- [x] AC9. 回滚步骤已写明；若切换失败，应用容器能回到 rc.25 并恢复 `/api/status`。
- [x] AC10. Trellis 任务文档、服务器操作备注和聊天回复中不出现密钥原文。

## Out of Scope

- 不在本次切换中开启或调参提示词审计策略。
- 不修改 Nginx 域名、证书、反代目标以外的 Web 站点。
- 不迁移或重启同机其他业务容器（shop、saas、elasticsearch 等）。
- 不更换 Postgres/Redis 密码，不拆分 `LOG_SQL_DSN`。
- 不把生产密钥提交进 GitHub。
- 不建设独立镜像仓库或 CI 自动发布流水线（可列为后续）。
- 不把本工作区未推送的本地修改当作生产源；以 GitHub `main` 为准。
- 不清理或改写历史请求日志内容。

## Technical Notes

- 新镜像启动会 AutoMigrate，预期新增 `prompt_audit_events`、`login_encryption_keys`、`task_plugins` 等表；这是增量迁移，不是换库。
- `SESSION_SECRET` 必须保持不变，否则已登录会话与部分签名会失效。
- `CRYPTO_SECRET` 继续缺省回退到 `SESSION_SECRET`，与当前生产一致。
- 仓库 `VERSION` 文件为空；构建前在服务器工作副本写入 `lailai421-<shortsha>`，不要把该文件提交回 GitHub。
