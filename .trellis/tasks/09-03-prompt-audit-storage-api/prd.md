# 提示词审计存储与管理 API (09-03-prompt-audit-storage-api)

## Goal

为 `new-api` 实现提示词审计的配置持久化、CAS 并发更新控制、三数据库兼容的审计事件存储、完整提示词应用层加密落库与 Root 按需解密、master 节点周期性自动清理与批量删除，以及受 RootAuth 保护的专用管理 API（`/api/prompt-audit/*`），并将管理 API 契约和事件持久化语义冻结，为后续 HTTP Relay、Realtime、Task Plugin 接入和前端控制台提供可靠的底层存储与生命周期管理支撑。

## Background & Confirmed Facts

1. **上游核心已就绪并冻结**：提交 `f01f90d8` 已在 `service/promptaudit/` 实现并冻结领域契约，包含 `Config`、`PublicConfig`、`ActiveConfig`、`PromptSnapshot`、`Decision`、`NormalizedResult`、`RuntimeState`、`ConfigStore`、`EventStore`、`Encryptor`、`Manager`、`GuardError` 及稳定错误码。
2. **两态运行模型**：提示词审计只有 `off` 与 `blocking` 两态，无异步只审计模式。
3. **安全与敏感数据边界**：
   - 节点 Token 是 write-only 输入；数据库只存密文，对外只暴露 `has_token` 和 `token_status`（configured / missing / invalid）。
   - 完整提示词（`FullPrompt`）必须无截断保存，但必须经应用层 AES-256-GCM 加密后落库。列表接口只返回脱敏预览和元数据；只有 Root 详情接口按需解密并返回完整原文，且响应头包含 `Cache-Control: no-store`。
   - `ScanText`（实际送审片段）不得持久化；普通日志、通用 Option、SQL 结构化日志、探测响应严禁泄漏任何 Token、密文、FullPrompt 或 ScanText。
4. **三数据库兼容**：必须同时支持 SQLite、MySQL >= 5.7.8 和 PostgreSQL >= 9.6。所有 DDL 与查询均使用 GORM 统一方法与标准 Tag，避免方言特性（无外键、无 JSONB、无 TIMESTAMPTZ、无 BIGSERIAL、无 SKIP LOCKED、无方言 CHECK）。
5. **JSON 包装器规范**：必须使用 `common.Marshal`、`common.Unmarshal`、`common.UnmarshalJsonStr`、`common.DecodeJson`，严禁在业务代码中直接调用 `encoding/json`。

## Requirements

### 1. 配置持久化与 CAS (ConfigStore)
- **存储载体**：复用主库 `Option` 表，键名为 `PromptAuditConfigSecret`（`promptaudit.OptionKeyPromptAuditConfigSecret`）。
- **CAS 机制**：
  - 更新配置必须携带 `expected_config_version`。
  - Model 层在同一事务中通过 `lockForUpdate(tx)` 读取现有 Option、比对版本：
    - 若现有 Option 不存在，初始版本必须匹配期望版本（`expectedVersion == 0` 或 `expectedVersion == 1`），写入新 Option 并设版本为 1。
    - 若现有 Option 存在，版本与 `expected_config_version` 严格一致时方可递增 `config_version = expectedVersion + 1` 并保存。
    - 版本不匹配时回滚并稳定返回 409 冲突（错误码 `prompt_audit_config_conflict`）。
  - 服务端强制生成 `updated_at`、`updated_by` 和递增的新版本，拒绝客户端篡改。
- **Token 变更管理**：
  - 节点 Token 采用 write-only 设计。若请求提供新明文 Token，调用 core `EncryptToken` 加密后持久化。
  - 若更新未提供新 Token，保留已有节点的 `token_ciphertext`。
  - 若显式指定删除 Token，清空该节点的密文。
- **访问隔离与防绕过**：
  - `controller.GetOptions` 必须显式排除该键，不得返回给通用配置接口。
  - `controller.UpdateOption`、`controller.UpdateOptionsBulk`、`model.UpdateOption`、`model.UpdateOptionsBulk` 必须显式拒绝修改该键，防止绕过校验、加密和 CAS 机制。
- **热更新与降级**：
  - 保存成功后本实例立即执行 `Manager.Reload(ctx)`。
  - 其他实例通过已有的 `model.SyncOptions` 定期同步并重载。
  - 配置缺失时安全默认关闭；配置存在但解析/解密/校验失败或启用状态无可用节点时，Manager 进入 `degraded` 状态，后续审计请求必须失败关闭（返回 503）。

### 2. 审计事件模型与三数据库兼容 (EventStore)
- **数据表**：主库新增 `PromptAuditEvent` 表（表名 `prompt_audit_events`），加入 `model/main.go:migrateDB()` 的 `AutoMigrate` 清单。
- **字段定义**（完全对齐父任务 `design.md` 第 8 节）：
  - 标识与身份：`id` (int64 PK), `request_id` (varchar 128), `user_id` (int), `username_snapshot` (varchar 255), `user_email_snapshot` (varchar 320), `token_id` (int), `token_name_snapshot` (varchar 255), `group` (varchar 64)。
  - 路由与元数据：`channel_id` (int), `channel_type` (int), `request_path` (varchar 255), `protocol` (varchar 64), `model` (varchar 255), `stage` (varchar 32), `audit_scope` (varchar 32)。
  - 提示词数据：`prompt_hash` (varchar 64), `redacted_preview` (text), `full_prompt_ciphertext` (跨库 LongText: MySQL LONGTEXT, PG/SQLite TEXT), `prompt_length` (int), `message_count` (int)。
  - 判定结果与证据：`decision` (varchar 32), `risk_level` (varchar 32), `action` (varchar 32), `categories_json` (text), `matched_scanners_json` (text), `scanner_scores_json` (text), `scanner_evidence_json` (text), `scanner_backend` (varchar 64), `scanner_version` (varchar 128)。
  - Guard 与策略：`guard_endpoint_id` (varchar 128), `policy_id` (varchar 128), `policy_version` (int), `config_version` (int64), `chunk_total` (int), `latency_ms` (int64), `error_code` (varchar 64), `created_at` (int64)。
- **索引设计**：
  - 复合主排序列：`(created_at, id)`。
  - 常用单列索引：`request_id`, `user_id`, `token_id`, `group`, `decision`, `risk_level`, `guard_endpoint_id`, `prompt_hash`。
- **跨库长文本保证**：`full_prompt_ciphertext` 使用自定义 GORM 类型，确保在 MySQL 上创建为 `LONGTEXT`（支持 > 64 KiB），在 PostgreSQL 和 SQLite 上创建为 `TEXT`。模型打 `json:"-"` 标签防普通序列化泄露。
- **落库门禁策略**：
  - Block、Unavailable、Invalid 以及携带错误码的失败事件必须写库。
  - Allow / Flag 事件当且仅当 `store_pass_events == true` 时写库；若 `store_pass_events == false`，跳过落库并允许业务继续。
  - 必须写库但加密或写入失败时，向调用方返回明确错误，映射为 `prompt_audit_record_failed`（HTTP 503）并失败关闭。

### 3. 查询、详情与删除
- **列表查询 (`GET /api/prompt-audit/events`)**：
  - 支持多维筛选：时间范围 (`start_time`, `end_time`)、`user_id`、`token_id`、`group`、`model`、`protocol`、`decision`、`risk_level`、`category`、`guard_endpoint_id`、`request_id`、`prompt_hash`。
  - 分页参数：`page`, `page_size`（默认 20，上限 100），稳定排序 `created_at DESC, id DESC`。
  - 列表 DTO 包含脱敏预览 `redacted_preview`，绝不包含 `full_prompt`、`full_prompt_ciphertext` 或 `scanner_evidence`。
- **详情查询 (`GET /api/prompt-audit/events/:id`)**：
  - Root 权限专属，按 ID 查询事件，解密 `full_prompt_ciphertext` 并作为 `full_prompt` 返回。
  - 响应头强制设置 `Cache-Control: no-store`。
  - 解密失败返回稳定安全错误，不回显内部密文或密钥。
- **删除能力**：
  - 单条删除：`DELETE /api/prompt-audit/events/:id`。
  - 批量删除：`POST /api/prompt-audit/events/batch-delete`，接收明确 ID 列表（去重校验，最多 500 条）。
  - 删除前后调用 `recordManageAudit` 记录管理员审计日志（只记 actor、ID、数量等安全元数据，禁止记入提示词内容）。

### 4. 保留期与 Master-only 自动清理
- **保留期策略**：`retention_days = 0` 表示永久保留，不触发自动删除；`retention_days > 0` 时按 `created_at < cutoff` 计算过期。
- **清理批次控制**：每批最多查询 500 个明确 ID，按 ID 批量删除；单次执行设置最大循环批次数（例如 10 批，最多 5000 条），防止长事务和全表长时间锁死。
- **执行环境**：仅在 `common.IsMasterNode` 节点上启动后台小时级 Ticker。
- **测试友好**：清理核心逻辑接受注入的 `now` / `cutoff`，可直接同步调用验证，严禁在测试中依赖真实 `time.Sleep`。

### 5. Root 管理 API 契约 (`/api/prompt-audit`)
- 全部接口由 `middleware.RootAuth()` 拦截。
- `GET /config`：返回脱敏 `PublicConfig`、scanner catalog 和当前 `config_version`。
- `PUT /config`：提交专用 DTO，校验合法性、派生加密 Token、CAS 保存并立即调用 `Manager.Reload`，冲突返 409。
- `GET /runtime`：返回运行时模式、版本、加载时间、加载错误码、`degraded` 标识、可用节点数。
- `POST /endpoints/probe`：独立探测指定节点（支持已配置节点或新提交的测试参数），使用 core 的安全 HTTP Client 与 Qwen3Guard 解析器，返回延迟与状态，不回显凭据或请求内容。
- `GET /events`：脱敏列表与筛选。
- `GET /events/:id`：按需解密详情。
- `DELETE /events/:id`：单条删除。
- `POST /events/batch-delete`：批量删除。

## Acceptance Criteria

- [ ] `PromptAuditConfigSecret` 在通用 `GetOptions` 中被显式过滤，在通用 `UpdateOption` / `UpdateOptionsBulk` 中被显式拒绝。
- [ ] 配置更新采用事务内 `lockForUpdate` 实现基于 `config_version` 的 CAS，版本冲突稳定返回 409 `prompt_audit_config_conflict`。
- [ ] 节点 Token 仅作为 write-only 接收，正确加解密；PublicConfig 仅展示 `has_token` 和 `token_status`。
- [ ] `PromptAuditEvent` 结构完整，通过 `AutoMigrate` 在 SQLite、MySQL、PostgreSQL 迁移成功且连续两次运行幂等。
- [ ] `full_prompt_ciphertext` 在 MySQL 映射为 LONGTEXT，在 PostgreSQL / SQLite 映射为 TEXT；写入并读取超过 64 KiB 的提示词逐字符解密还原无截断。
- [ ] EventStore 严格执行落库策略：Block/Error 必存，Pass/Flag 受 `store_pass_events` 控制；必要事件写库失败稳定返回 `prompt_audit_record_failed`（HTTP 503）。
- [ ] 列表 API 绝不包含 `full_prompt` 或密文；详情 API 仅限 Root 并返回完整解密原文，附带 `Cache-Control: no-store`。
- [ ] 事件筛选覆盖时间、用户、Token、分组、模型、协议、decision、risk、category、节点、request_id、hash，排序稳定。
- [ ] 单条删除与最多 500 条批量删除行为确定，管理审计日志不含任何提示词内容。
- [ ] 保留天数大于 0 时可按 cutoff 分批清理，保留天数为 0 时不清理，清理逻辑幂等。
- [ ] 权限矩阵严格生效：未登录、普通用户、普通 Admin 均无法访问，仅 Root 允许访问。
- [ ] 完成真实的 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 实例新建、升级与行为验证。

## Out of Scope

- 不实现具体请求协议的提取器（留给 HTTP Relay、Realtime、Task Plugin 子任务）。
- 不修改 `controller/relay.go`，不接入 Relay 门禁。
- 不编写前端 React 页面、菜单或国际化文案（留给 web-console 子任务）。
- 不修改 `relaykit/`。

