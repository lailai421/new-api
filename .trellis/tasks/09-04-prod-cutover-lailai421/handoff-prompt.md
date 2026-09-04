# 交接提示词：执行 09-04-prod-cutover-lailai421

把下面「可复制区」整段粘贴到新的上下文窗口，作为该窗口的第一条用户消息。不要只贴文件名。

规划已于 2026-09-04 由用户审核通过。新窗口的职责是**执行切换**，不是重新设计方案。

---

## 可复制区（从下一行开始）

```text
你是本仓库的执行 agent。立即执行已批准的 Trellis 任务 09-04-prod-cutover-lailai421：把生产机 72.60.29.71 上正在服务团队的 new-api，从官方镜像 calciumion/new-api:latest（v1.0.0-rc.25）切换为 GitHub https://github.com/lailai421/new-api 的 main 在服务器本地构建的镜像。保留全部用户数据。后续更新只跟踪这个 fork。

用中文回复。不要重新做需求调研，不要再问「要不要创建任务 / 要不要开始」。规划已通过。本窗口要做的是：激活任务 → 按 implement.md 在生产机执行 → 按 prd.md 验收 → 汇报结果。

====================
0. 启动约定
====================

仓库根目录：当前工作区（new-api 仓库）。

先读并遵守：
- AGENTS.md
- .agents/skills/trellis-start/SKILL.md
- .agents/skills/trellis-continue/SKILL.md
- .agents/skills/trellis-before-dev/SKILL.md（执行前）
- .agents/skills/trellis-check/SKILL.md（验收时）

然后执行：

```bash
python3 ./.trellis/scripts/get_context.py
python3 ./.trellis/scripts/get_context.py --mode phase
python3 ./.trellis/scripts/task.py start 09-04-prod-cutover-lailai421
python3 ./.trellis/scripts/task.py current --source
```

必须确认 current 指向 `.trellis/tasks/09-04-prod-cutover-lailai421`，status 变为 in_progress。若会话指针仍停在 09-03-prompt-audit 或其他任务，先 start 本任务，不要去改提示词审计代码。

必读任务文档（按顺序，读完再动生产）：
1. .trellis/tasks/09-04-prod-cutover-lailai421/prd.md
2. .trellis/tasks/09-04-prod-cutover-lailai421/design.md
3. .trellis/tasks/09-04-prod-cutover-lailai421/implement.md
4. .trellis/tasks/09-04-prod-cutover-lailai421/research/production-inventory.md
5. .trellis/tasks/09-04-prod-cutover-lailai421/research/cutover-strategy.md

权威执行步骤以 implement.md 为准。prd.md 的 Acceptance Criteria 是完成标准。design.md 是边界与回滚形状。盘点文档是 2026-09-04 的只读快照，执行当天必须重新采集计数和版本，不要沿用旧数字当验收基线。

====================
1. 已锁定的决策（禁止推翻）
====================

- 源码：https://github.com/lailai421/new-api  分支 main。以 GitHub 远程为准，不以本工作区可能落后的 HEAD 为准。执行当天先 git ls-remote 记录目标 SHA。
- 构建方式：在生产服务器 /srv/docker-migration/newapi/src 克隆源码，用仓库 Dockerfile 做 docker compose build，镜像名 lailai421/new-api:local。
- 数据：继续用现有 Postgres（容器 newapi-postgres，volume newapi_pg_data，库名 new-api）。不是 SQLite，不是 /data 目录，不是新建空库。
- 切换形状：先构建（旧容器继续服务），再只替换 new-api 容器。Postgres / Redis / Nginx 以及同机其他业务容器一律不动。
- 提示词审计：代码可以随镜像带上，当天不得开启门禁，不得改变现网拦截行为。
- 版本号：构建前在服务器 src/VERSION 写入 lailai421-<shortsha>，不要把这个 VERSION 提交回 GitHub。仓库里的 VERSION 文件是空的。
- 后续更新：在服务器留下 UPDATE.md（可附 update.sh）：git pull --ff-only → 写 VERSION → docker compose build new-api → docker compose up -d --no-deps new-api。compose 不得再把 calciumion/new-api:latest 当作更新来源。

====================
2. 生产事实（执行前再核对）
====================

- SSH：root@72.60.29.71（hostname srv945124）
- 目录：/srv/docker-migration/newapi
- Compose 项目：newapi
- 容器：new-api / newapi-postgres / newapi-redis
- 当前镜像（规划时）：calciumion/new-api:latest，version v1.0.0-rc.25
- 端口：127.0.0.1:3000 与 172.17.0.1:3000
- 公网：https://wj-api.mobikechip.com 与 https://wj-api.mobikechip.cn ，Nginx 反代到 172.17.0.1:3000
- 网络：newapi_new-api-network + 外部网络 cliproxyapi_default
- extra_hosts：host.docker.internal:host-gateway
- 已有环境变量（不要改值，不要把原文贴进聊天）：SQL_DSN、SESSION_SECRET、REDIS_CONN_STRING、NODE_NAME=new-api-node-1、BATCH_UPDATE_ENABLED=true
- 未单独配置 CRYPTO_SECRET（回退 SESSION_SECRET，保持即可）
- 规划时规模约：库 290MB，users=5，tokens=22，channels=6。这些只作参考，以当天快照为准。

SSH 使用 BatchMode。所有会打印环境变量、compose、日志的命令必须脱敏：把 PASSWORD/SECRET/TOKEN/KEY/DSN/CONN_STRING 的值打成 ***REDACTED***。禁止把 docker-compose.yml 原文、pg_dump、密钥贴进聊天或写进任务文件。

====================
3. 硬性禁止
====================

- docker compose down
- docker compose down -v
- docker volume rm / 删除 newapi_pg_data
- 重启或重建 postgres / redis / nginx
- 修改 SQL_DSN、SESSION_SECRET、REDIS_CONN_STRING 的值
- 改 ports、networks、extra_hosts、environment 其它项
- 触碰同机其它容器（shop、saas、elasticsearch、宿主机 mysql 等）
- 把本地 one-api.db 导入生产
- 开启或调参提示词审计
- 把生产密钥提交到 GitHub
- 在 src/ 里改业务代码并当作发布
- 构建失败后仍然 up 新容器
- 健康检查失败后先恢复 dump（默认只滚回应用镜像）

compose 只允许改 new-api 服务的镜像来源，改成：

```yaml
build:
  context: ./src
  dockerfile: Dockerfile
image: lailai421/new-api:local
```

改完必须 diff 对照备份文件，确认只有 build/image 相关行变化。

====================
4. 执行顺序（严格按 implement.md）
====================

1) 切换前快照：STAMP、/api/status、users/tokens/channels 计数、当前镜像 ID。写入 /srv/docker-migration/newapi/backups/$STAMP/
2) 回滚点 A：docker tag calciumion/new-api:latest calciumion/new-api:pre-cutover-rc25 ；复制 compose 为 docker-compose.yml.bak_$STAMP ；pg_dump -Fc 到 backups/$STAMP/new-api.dump 。确认 dump 非空、镜像标签存在。2026-09-02 的 fresh_new-api.dump 不能当本次备份。
3) clone 或更新 src 到 GitHub main，记录 SHA，写入 VERSION=lailai421-<shortsha>
4) 修改 compose 镜像来源（见上）
5) docker compose build new-api 。失败则停止，旧 rc.25 继续服务。
6) docker compose up -d --no-deps new-api 。等 healthcheck。2 分钟内不健康则回滚。
7) 按第 5 节验收。
8) 写入 /srv/docker-migration/newapi/UPDATE.md（可附 update.sh，脚本不得含 down -v 或 pull 官方 latest）。
9) 只有失败时才回滚：把 compose 恢复为使用 calciumion/new-api:pre-cutover-rc25（去掉 build），再 up -d --no-deps new-api。确认 /api/status 回到 v1.0.0-rc.25。不要先恢复 dump。

====================
5. 验收（必须全部核对）
====================

对照 prd.md AC1–AC10：

- 运行中镜像来自 lailai421/new-api:local，由 /srv/docker-migration/newapi/src 的 GitHub 提交构建
- 两个公网域名 /api/status 均为 success=true，version 含 lailai421-<构建时 short sha>，不再是 v1.0.0-rc.25
- users / tokens / channels 行数与当天切换前快照一致；postgres 容器和 volume 未被重建
- 现有管理员能登录后台；至少抽查 1 个现有令牌/渠道配置仍在。若你没有管理员密码，用只读 SQL 确认用户和渠道行还在，并明确请用户做登录验收，不要编造「已登录成功」
- 存在 pre-cutover-rc25 标签、compose bak、pg_dump
- compose 的 new-api 使用 build: ./src
- 服务器上有 UPDATE.md
- 提示词审计保持关闭，现网请求不被新门禁拦截。允许 AutoMigrate 创建 prompt_audit_events 空表
- 聊天和任务文件中无密钥原文

可用命令（输出记得脱敏）：

```bash
docker compose ps
docker inspect new-api --format '{{.Config.Image}}'
docker exec new-api wget -q -O - http://localhost:3000/api/status
git -C /srv/docker-migration/newapi/src log -1 --oneline
docker exec newapi-postgres psql -U root -d new-api -c "SELECT (SELECT count(*) FROM users) AS users, (SELECT count(*) FROM tokens) AS tokens, (SELECT count(*) FROM channels) AS channels;"
docker exec newapi-postgres psql -U root -d new-api -c "\dt prompt_audit_events"
```

公网再查：
https://wj-api.mobikechip.com/api/status
https://wj-api.mobikechip.cn/api/status
只提取 success 与 version，不要把完整 status 里可能出现的敏感配置贴出来。

====================
6. 完成后怎么收尾
====================

- 把当天 SHA、镜像 ID、计数前后对比、备份路径、UPDATE.md 路径、未完成的人工登录项写进任务 notes 或一份 .trellis/tasks/09-04-prod-cutover-lailai421/execution-log.md（仍然禁止密钥）。
- 对照 prd.md 把已满足的 AC 勾掉。
- 不要 archive、不要 git commit 生产机上的 compose（compose 含密钥，不进 GitHub）。
- 本仓库若只有 handoff/任务文档变更，询问用户是否提交；生产切换本身不是应用代码 PR。
- 用中文给用户一份结果报告：成功或已回滚、版本号、计数对比、备份位置、后续如何更新、还需要用户亲自确认的登录项。

现在开始：先读文档并 task.py start，再按 implement.md 执行。遇到构建失败或健康检查失败立刻回滚并停止，不要自行扩大范围。
```

---

## 使用说明

1. 新开一个上下文窗口，工作目录仍是本仓库。
2. 复制「可复制区」里代码块的全部文字（不含外层 markdown 围栏）。
3. 作为第一条用户消息发送。
4. 执行窗口应自行 `task.py start`，不必再等一次「是否开始」的确认。
5. 若该窗口无法 SSH 到 `root@72.60.29.71`，应停止并报告，不要改成本地模拟切换。
