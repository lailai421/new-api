# 生产切换执行日志 (2026-09-04)

任务：`09-04-prod-cutover-lailai421`  
目标机：`root@72.60.29.71` (`srv945124`)  
执行时间：2026-09-04 15:34 ~ 15:46 (UTC+8) / 07:34 ~ 07:46 (UTC)

---

## 1. 关键版本与提交信息

- **源码仓库**: `https://github.com/lailai421/new-api.git` (分支: `main`)
- **目标提交 SHA**: `6651c38d71d5a3222831ad88b0619fe3bd65dcf1` (Short SHA: `6651c38`)
- **写入 VERSION**: `lailai421-6651c38`
- **构建生成镜像**:
  - 镜像标签: `lailai421/new-api:local`
  - 镜像 ID: `sha256:9a05704402a602db97a1d363c33ddd663a7de3f4d07bda50a55fdb6cfa572e3c`
- **切换前官方镜像**:
  - 原镜像标签: `calciumion/new-api:latest`
  - 备份本地标签: `calciumion/new-api:pre-cutover-rc25`
  - 镜像 ID: `sha256:db3342eeec7e8132f0a0c162f77b68f9786f4ad7e6901c4714a9307842f60b0d`

---

## 2. 数据前后对比（Postgres）

| 监控项 | 切换前快照 (07:34:48 UTC) | 切换后核查 (07:40:44 UTC) | 状态 |
|---|---|---|---|
| `users` | 5 | 5 | 一致 |
| `tokens` | 22 | 22 | 一致 |
| `channels` | 6 | 6 | 一致 |
| `prompt_audit_events` | 无此表 | 存在（0 行） | AutoMigrate 增量创建成功 |
| `newapi-postgres` 容器 | 运行中 (Up 2 days) | 保持运行中 (未重启/未重建) | 容器与 volume 零中断 |
| `newapi-redis` 容器 | 运行中 (Up 2 days) | 保持运行中 (未重启/未重建) | 零中断 |

---

## 3. 备份路径

所有回滚备份均保存在生产机 `/srv/docker-migration/newapi/` 及其备份子目录下：

- **Docker 镜像备份标签**: `calciumion/new-api:pre-cutover-rc25`
- **Compose 配置备份**: `/srv/docker-migration/newapi/docker-compose.yml.bak_20260904-073448`
- **Postgres 数据库 Dump**: `/srv/docker-migration/newapi/backups/20260904-073448/new-api.dump` (13MB, pg_dump -Fc)
- **切换前状态快照**: `/srv/docker-migration/newapi/backups/20260904-073448/status-before.json`
- **切换前计数快照**: `/srv/docker-migration/newapi/backups/20260904-073448/counts-before.txt`

---

## 4. Compose 变更与更新路径

- **Compose 变更文件**: `/srv/docker-migration/newapi/docker-compose.yml`
  - `new-api` 服务镜像源由 `image: calciumion/new-api:latest` 更改为：
    ```yaml
    build:
      context: ./src
      dockerfile: Dockerfile
    image: lailai421/new-api:local
    ```
  - 环境变量、端口、卷挂载、网络、外部网络 `cliproxyapi` 等均无改动。
- **更新文档**: `/srv/docker-migration/newapi/UPDATE.md`
- **快捷更新脚本**: `/srv/docker-migration/newapi/update.sh` (已赋可执行权限)

---

## 5. 服务与公网验证结果

- **容器内部状态**:
  - `docker compose ps`: `new-api` 状态 `Up (healthy)`
  - `http://localhost:3000/api/status`: `success=true`, `version=lailai421-6651c38`
- **公网入口**:
  - `https://wj-api.mobikechip.cn/api/status`: 返回 HTTP 200，`success=true`，`version="lailai421-6651c38"`
  - 说明：`wj-api.mobikechip.com` 用户已确认已停用（无公共 DNS 解析）。
- **业务流量**:
  - 切换后容器实时日志显示团队 `/v1/responses`、`/v1/models` 等转发请求持续正常返回 200 OK，额度扣减与日志记录正常。

---

## 6. 待人工核验项

- [ ] 请管理员使用现有账号登录 `https://wj-api.mobikechip.cn` 管理后台，确认管理界面正常。
- [ ] 随机测试 1 个常用客户端 API Token，确认渠道调用符合预期。
