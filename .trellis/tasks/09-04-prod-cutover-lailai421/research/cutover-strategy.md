# 切换策略研究

## 问题一句话

把正在服务团队的生产 `new-api` 从官方镜像换成 `lailai421/new-api`，用户数据必须留下，以后更新也只跟这个 fork。

## 不可变事实

- 用户、令牌、渠道、额度、日志都在 Postgres 主库，不在 SQLite，也不在 `/data`。
- 应用容器可替换；`newapi-postgres` 数据卷和 `SESSION_SECRET` 不可替换。
- 新代码启动会 `AutoMigrate`，会**加表/加列**，不会按设计删业务表。
- 提示词审计是新能力；生产库里还没有 `prompt_audit_events`。默认配置应保持关闭，避免切换当天改变请求拦截行为。
- GitHub 仓库公开，服务器可直接 `git clone` / `git pull`，不必先做私有部署密钥。

## 方案对比

### A. 服务器克隆源码 + compose build（推荐）

- 在 `/srv/docker-migration/newapi/src` 克隆 `https://github.com/lailai421/new-api.git`
- 把 compose 的 `image: calciumion/new-api:latest` 改成 `build: ./src` + 本地镜像名 `lailai421/new-api:local`
- 先 build、后 `up --no-deps new-api`，postgres/redis 不重建
- 以后更新：`git pull --ff-only` → 写入 VERSION → `docker compose build new-api` → `up -d --no-deps new-api`

好处：密钥仍只留在服务器 compose；后续更新路径清晰；不依赖 Docker Hub 官方镜像。
代价：首次构建要拉 bun/golang 基础镜像，约数分钟到十几分钟；构建期间旧容器继续服务。

### B. GitHub Actions 推镜像，服务器只 pull

好处：构建不占生产 CPU。
代价：要额外的镜像仓库账号、token、权限；本次目标是尽快切过去并固定更新源，收益不够。

### C. 直接 docker 跑二进制 / 不用 compose

会打散现有 postgres/redis/网络/nginx 约定，否决。

## 停机形状

推荐「先构建、再只替换应用容器」：

1. 旧容器继续用 rc.25 提供服务
2. 构建新镜像
3. `docker compose up -d --no-deps new-api` 替换应用容器
4. 健康检查通过后开放确认

预期中断：应用容器重启那几十秒到两分钟。Nginx 与 Postgres 不停。

禁止：`docker compose down -v`、删除 `newapi_pg_data`、改 `SQL_DSN` 指向新空库、覆盖 `SESSION_SECRET`。

## 回滚形状

切换前给当前镜像打标签 `calciumion/new-api:pre-cutover-rc25`，并备份 compose 与最新 `pg_dump`。

若新容器起不来或健康检查失败：把 compose 的 image 改回 `calciumion/new-api:pre-cutover-rc25`，去掉 build，再 `up -d --no-deps new-api`。

AutoMigrate 新增的空表/新列，旧 rc.25 一般会忽略。只有确认数据被写坏时才用 `pg_dump` 恢复；默认回滚不还原数据库。

## 版本号

Dockerfile 用 `$(cat VERSION)` 打进二进制。仓库里 VERSION 为空，直接构建会得到空版本或 `v0.0.0`，不便核对是否已切换。

构建前在服务器工作副本写入：

```text
lailai421-<shortsha>
```

只改构建目录，不把这个 VERSION 提交回 GitHub。
