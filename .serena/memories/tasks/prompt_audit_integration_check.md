# Prompt Audit Integration & Three-Database Verification (09-03-prompt-audit-integration-check)

## 1. 核心架构与严格时序不变量
- **执行时序**: 客户端输入 -> TokenAuth/身份提取 -> 显式协议提取 (`PromptSnapshot`) -> 同步 Guard 判定 (`Evaluator.Evaluate`) -> 必要事件加密写入 (`EventStore.Record`) -> 计费预扣 (`PreConsumeBilling`) -> 业务上游拨号/请求。
- **失败关闭原则**: 无论 Block、Guard 不可用、响应非法、配置 degraded、无协议契约还是事件写入失败，均在此处阻断并返回对应 403/503。业务上游调用次数与预扣费次数严格为 0，且错误设置 `SkipRetry` / `LocalError: true`，不触发渠道重试或业务结算。
- **两态模型**: 仅支持 `off` 与 `blocking` 两态，绝无异步记录后放行分支。

## 2. 三数据库兼容与测试隔离最佳实践
- **跨库长文本 (>64 KiB)**: MySQL `TEXT` 存在 64 KiB 上限。必须采用自定义 GORM 类型（如 `PromptCiphertext`），在 MySQL 映射为 `longtext`，在 PostgreSQL / SQLite 映射为 `text`。
- **全局测试状态隔离 (Mandatory Cleanup)**: 运行多数据库（SQLite/MySQL/PostgreSQL）矩阵测试时，切换全局 `model.DB`、`model.LOG_DB` 及 `common.SetDatabaseTypes` 必须在开头捕获并使用 `t.Cleanup` 恢复：
  ```go
  previousDB, previousLogDB := DB, LOG_DB
  previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
  t.Cleanup(func() {
      DB, LOG_DB = previousDB, previousLogDB
      common.SetDatabaseTypes(previousMainType, previousLogType)
      InitCol()
  })
  ```
  否则将导致同一进程内的其他包或控制器测试被污染而失败。

## 3. 敏感配置与数据隔离边界
- **Option 隔离**: `PromptAuditConfigSecret` 必须在通用 `GetOptions` 中显式剔除，在通用 `UpdateOption` 中返回 403 拦截。更新只能通过专用 Root 管理接口，并在事务内通过 `lockForUpdate(tx)` 执行 CAS 版本控制。
- **凭据 write-only**: 节点 Token 仅在提交时接收明文并立即经 AES-256-GCM 加密，对外只暴露 `has_token` 和状态，配置读取、日志、探测接口绝不回显。
- **原文保护**: 事件列表 DTO 强制排除完整原文与密文；详情接口仅 Root 用户解密并强制注入 `Cache-Control: no-store`。
- **动态日志零泄漏**: 采用严格字段白名单机制，过滤所有非白名单键，严禁将 Prompt 原文、Token、Guard 请求/响应报文打印入应用日志。
- **前端缓存物理擦除**: 前端查看详情时按需请求，关闭抽屉、切换事件或组件卸载时，立即通过 `queryClient.removeQueries` 抹除 React Query 内存缓存。

## 4. 真实数据库验证与质量命令
- 必须基于真实 SQLite、MySQL 8.0 (`127.0.0.1:13306`)、PostgreSQL 18 (`127.0.0.1:15432`) 验证初始建表、二次迁移幂等性、长文本加解密比对、CAS 并发与增量升级测试。
- 单条 Go 后端后台测试命令必须设置不超过 55 秒的超时。
- 业务代码严禁直接调用 `encoding/json`，全量走 `common.*` 包装。
- 根模块改动不得侵入 `relaykit/`，验证命令：`cd relaykit && GOWORK=off go build ./...`。
