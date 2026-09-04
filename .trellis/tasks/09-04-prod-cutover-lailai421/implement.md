# 执行清单

审核通过并 `task.py start` 之后，按顺序执行。未审核前不要 SSH 做任何写入。

## 0. 前置

- 确认本任务仍是 planning 且方案已获用户明确批准。
- 确认 GitHub `lailai421/new-api` `main` 的目标 SHA（执行当天再 `git ls-remote` 一次）。
- 通知团队将有约 1–2 分钟 API 中断。
- 所有命令在 `root@72.60.29.71` 的 `/srv/docker-migration/newapi` 进行。
- 输出重定向到备份目录时，对日志做密钥脱敏；禁止把 compose 原文贴进聊天。

## 1. 切换前快照

```bash
STAMP=$(date +%Y%m%d-%H%M%S)
mkdir -p /srv/docker-migration/newapi/backups/$STAMP
docker exec new-api wget -q -O - http://localhost:3000/api/status > /srv/docker-migration/newapi/backups/$STAMP/status-before.json
docker exec newapi-postgres psql -U root -d new-api -c "SELECT (SELECT count(*) FROM users) AS users, (SELECT count(*) FROM tokens) AS tokens, (SELECT count(*) FROM channels) AS channels;"
docker inspect new-api --format '{{.Config.Image}} {{.Image}}'
```

把 users/tokens/channels 三个数字记入 `backups/$STAMP/counts-before.txt`。

## 2. 备份（回滚点 A）

```bash
docker tag calciumion/new-api:latest calciumion/new-api:pre-cutover-rc25
cp -a docker-compose.yml docker-compose.yml.bak_$STAMP
docker exec newapi-postgres pg_dump -U root -d new-api -Fc > backups/$STAMP/new-api.dump
ls -lh backups/$STAMP/new-api.dump
```

确认 dump 非空，且 `docker images` 能看到 `pre-cutover-rc25`。

## 3. 接入源码

```bash
git clone --branch main --depth 50 https://github.com/lailai421/new-api.git src
# 若 src 已存在：cd src && git fetch origin && git checkout main && git pull --ff-only
cd src
git rev-parse --short HEAD
echo "lailai421-$(git rev-parse --short HEAD)" > VERSION
cd ..
```

核对 `src` 远程为 `lailai421/new-api`，当前提交与计划 SHA 一致。

## 4. 改 compose（只动 new-api 的镜像来源）

在 `docker-compose.yml` 的 `new-api` 服务中：

- 删除或不再使用 `image: calciumion/new-api:latest` 作为更新来源
- 增加：

```yaml
build:
  context: ./src
  dockerfile: Dockerfile
image: lailai421/new-api:local
```

不得改 environment、ports、networks、volumes、postgres、redis。改完后 `diff` 对照 `docker-compose.yml.bak_$STAMP`，确认只有 build/image 相关行变化。

## 5. 构建（旧容器仍在跑）

```bash
docker compose build new-api
```

构建失败则停止，不进入第 6 步。旧环境保持 rc.25。

## 6. 切换应用容器（回滚点 B）

```bash
docker compose up -d --no-deps new-api
docker compose ps
docker logs --tail 200 new-api
```

等待 healthcheck 变为 healthy。若 2 分钟内不健康，执行「回滚」。

禁止：`docker compose down`、`docker compose down -v`、`docker volume rm`、重启 postgres/redis。

## 7. 验证

```bash
docker exec new-api wget -q -O - http://localhost:3000/api/status
docker exec newapi-postgres psql -U root -d new-api -c "SELECT (SELECT count(*) FROM users) AS users, (SELECT count(*) FROM tokens) AS tokens, (SELECT count(*) FROM channels) AS channels;"
docker exec newapi-postgres psql -U root -d new-api -c "\dt prompt_audit_events"
```

人工验收：

- 浏览器打开两个域名的登录页，现有管理员能登录
- `/api/status` 的 version 为 `lailai421-<sha>`
- 计数与第 1 步一致
- 提示词审计为关闭，现网请求不被新门禁拦截
- `docker inspect new-api` 显示镜像 `lailai421/new-api:local`

## 8. 固定后续更新

写入 `/srv/docker-migration/newapi/UPDATE.md`，内容为 design.md 中的 pull → VERSION → build → `up -d --no-deps new-api`。可附一个 `update.sh`，但脚本内不得含 `down -v` 或 `compose pull` 官方镜像。

## 9. 失败回滚

```bash
# 恢复 compose 中 new-api 的 image 为 calciumion/new-api:pre-cutover-rc25，并去掉 build
cp docker-compose.yml.bak_$STAMP docker-compose.yml
# 若 bak 仍是 latest 名称，先确保 pre-cutover-rc25 标签存在，再把 image 写成该标签
docker compose up -d --no-deps new-api
```

确认 `/api/status` 回到 `v1.0.0-rc.25` 后再排查构建日志。不要先恢复 dump。

## 验证命令汇总

- `docker compose ps`
- `docker exec new-api wget -q -O - http://localhost:3000/api/status`
- 公网 curl 两个域名 `/api/status`（只看 success/version）
- Postgres 三个计数
- 登录后台
- `git -C src log -1 --oneline`

## 风险文件 / 操作

- `/srv/docker-migration/newapi/docker-compose.yml`：只允许改 new-api 的 build/image
- Docker volume `newapi_pg_data`：只备份，不删除
- 同机其它 compose 项目：不触碰

## `task.py start` 前确认

- [ ] 用户已审核并批准本任务最新的 prd/design/implement
- [ ] 仍以 GitHub `lailai421/new-api` `main` 为源，而不是落后的本地工作区
- [ ] 不在本次开启提示词审计
- [ ] 执行窗口已和团队说清楚（短中断）
