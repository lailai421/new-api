# 提示词审计存储与管理 API 技术设计 (09-03-prompt-audit-storage-api)

## 1. 架构定位与分层原则

本子任务负责将 `service/promptaudit/` 领域核心接入 `new-api` 的持久化层、业务服务层、HTTP 控制器层与启动生命周期。整体遵循 Router → Controller → Service → Model 经典分层：

```text
HTTP 客户端 (Root 管理员)
       │
       ▼
router/api-router.go: /api/prompt-audit/* (RootAuth 拦截)
       │
       ▼
controller/prompt_audit.go (参数绑定、DTO 转换、管理审计日志、HTTP 状态码)
       │
       ▼
service/promptaudit/ (领域服务、Manager 状态机、ConfigStore/EventStore 实现、加密解密、探测、清理)
       │
       ▼
model/prompt_audit.go + model/option.go (GORM 实体、跨库数据类型、事务 lockForUpdate CAS、三库查询/删除)
       │
       ▼
主数据库 (SQLite / MySQL / PostgreSQL)
```

## 2. 详细设计与关键契约

### 2.1 数据模型与跨库兼容 (`model/prompt_audit.go`)

- **跨库大文本类型 `PromptCiphertext`**：
  实现 `schema.GormDataTypeInterface` 与 `schema.GormDBDataTypeInterface`：
  - `GormDataType() string` 返回 `"text"`。
  - `GormDBDataType(db *gorm.DB, field *schema.Field) string`：
    - MySQL 驱动下返回 `"longtext"`（突破 MySQL `TEXT` 64 KiB 上限，支持超长提示词无截断加密留存）。
    - PostgreSQL / SQLite 驱动下返回 `"text"`。
  - 实现 `driver.Valuer` 与 `sql.Scanner`，确保值始终以 `string` 写入并在读取时支持 `[]byte` 和 `string`（遵循 `json_column_test.go` 保护契约）。
- **`PromptAuditEvent` 结构**：
  - 严格包含父任务 `design.md` 第 8 节所有 34 个字段。
  - `FullPromptCiphertext PromptCiphertext` 标记 `gorm:"column:full_prompt_ciphertext" json:"-"`，确保普通 JSON 序列化永远不可见。
  - JSON 类字段（`CategoriesJSON`, `MatchedScannersJSON`, `ScannerScoresJSON`, `ScannerEvidenceJSON`）使用 `gorm:"type:text"` 存储，编解码全部调用 `common.Marshal` / `common.Unmarshal`。
  - 主键 `ID int64` 交给 GORM 自动生成，不手工指定自增语句。
  - 复合排序列 `(created_at, id)`，以及各筛选列索引。
  - 无任何 GORM 布尔默认 tag（如 `default:true`），避免 MySQL 与 PG 差异引发的反复 ALTER TABLE。

### 2.2 配置持久化与 CAS (`model/option.go` & `service/promptaudit/config_store.go`)

- **Option 表适配**：
  使用键名 `PromptAuditConfigSecret` 保存单条 JSON。
- **Model 层 CAS 原子性**：
  在 `model.SavePromptAuditConfigCAS(ctx context.Context, cfgJSON string, expectedVersion int64, updatedBy int) (int64, error)` 中：
  1. 开启事务 `tx := DB.WithContext(ctx).Begin()`；
  2. 使用 `lockForUpdate(tx).Where(commonKeyCol+" = ?", OptionKeyPromptAuditConfigSecret).First(&option)` 加锁读取；
  3. 若记录不存在：
     - 若 `expectedVersion == 0` 或 `expectedVersion == 1`，创建新 Option，设置 `config_version = 1`，提交事务；
     - 否则返回版本冲突错误。
  4. 若记录存在：
     - 解析当前配置的 `config_version`；
     - 若 `currentVersion != expectedVersion`，回滚事务，返回冲突错误；
     - 若匹配，递增版本为 `expectedVersion + 1`，更新 `updated_at`、`updated_by`，保存 Option，提交事务。
- **防止通用接口绕过**：
  - `controller.GetOptions` 显式判断 `k == promptaudit.OptionKeyPromptAuditConfigSecret` 则跳过。
  - `controller.UpdateOption`、`controller.UpdateOptionsBulk` 在更新前显式拦截该键，返回 403 / 400 拒绝通用修改。
  - `model.UpdateOption` 与 `model.UpdateOptionsBulk` 同样显式拒绝该键。

### 2.3 EventStore 落库与加解密集成 (`service/promptaudit/event_store.go`)

- 实现 `promptaudit.EventStore` 接口：
  ```go
  Record(ctx context.Context, snapshot PromptSnapshot, decision *Decision, storePassEvents bool) error
  ```
- **门禁与持久化时序**：
  1. 判定是否需持久化：
     - `decision.Kind == DecisionBlock || decision.Kind == DecisionUnavailable || decision.Kind == DecisionInvalid || decision.ErrorCode != ""` → 必存；
     - `decision.Kind == DecisionAllow || decision.Kind == DecisionFlag` → 仅在 `storePassEvents == true` 时保存。
     - 若无需保存，直接返回 `nil`。
  2. 敏感数据处理：
     - 调用 `encryptor.EncryptPrompt(snapshot.FullPrompt)` 得到 `ciphertext`；
     - 若加密失败，返回错误（后续映射为 `prompt_audit_record_failed`）；
     - 清除 `snapshot.ScanText`，不落库送审切片；
     - 构造 `PromptAuditEvent`，仅将 `ciphertext` 赋给 `FullPromptCiphertext`。
  3. 执行 `model.CreatePromptAuditEvent(ctx, event)`：
     - 若 DB 写入失败，返回错误，确保门禁不放行。

### 2.4 查询、详情解密与删除 (`model/prompt_audit.go` & `service/promptaudit/event.go`)

- **列表查询**：
  - 动态构建 GORM 查询：时间范围、用户、Token、分组、模型、协议、判定、风险等级、节点、hash 等。
  - 显式通过 DTO 投影，排除密文字段，仅保留脱敏预览 `redacted_preview`。
  - 稳定分页排序 `ORDER BY created_at DESC, id DESC`。
- **详情查询**：
  - 按 ID 读取数据库记录。
  - 调用 `encryptor.DecryptPrompt(event.FullPromptCiphertext)` 还原原文。
  - 若解密失败返回稳定安全错误，不向客户端透露密文或底层异常堆栈。
- **删除与审计**：
  - 单条删除：`model.DeletePromptAuditEventByID(ctx, id)`。
  - 批量删除：`model.BatchDeletePromptAuditEvents(ctx, ids)`，严格限制 `len(ids) <= 500`，对入参 ID 去重。
  - 调用 `controller.recordManageAudit` 记录操作者、数量和 ID 范围，不记录原文。

### 2.5 Master-only 自动清理 (`service/promptaudit/cleanup.go`)

- **保留期与截止时间**：
  - `retention_days <= 0`：永久保留，不执行任何删除。
  - `retention_days > 0`：`cutoff = time.Now().Unix() - int64(retention_days)*86400`。
- **有界小批次删除**：
  - 单次循环中，分页查询满足 `created_at < cutoff` 的记录 ID（`LIMIT 500`）；
  - 若查询为空，终止本次清理；
  - 若查询到 ID，按 ID 列表执行批量删除；
  - 限制单次清理任务最大批次数（如 10 批，最多 5000 条），避免独占数据库连接或长时间持锁。
- **可测试性设计**：
  - 清理函数签名为 `RunCleanupOnce(ctx context.Context, retentionDays int, nowUnix int64, batchSize int, maxBatches int) (int64, error)`，可在单元测试和数据库测试中以任意时间戳和批大小确定性调用。
  - 周期调度仅在 `common.IsMasterNode == true` 时启动。

### 2.6 Root 管理 API (`controller/prompt_audit.go` & `dto/prompt_audit.go`)

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/prompt-audit/config` | RootAuth | 获取脱敏 PublicConfig 与 ScannerCatalog |
| PUT | `/api/prompt-audit/config` | RootAuth | CAS 保存新配置，支持 Token write-only 变更，冲突返 409 |
| GET | `/api/prompt-audit/runtime` | RootAuth | 获取当前运行状态、版本号、degraded 标识、可用节点数 |
| POST | `/api/prompt-audit/endpoints/probe` | RootAuth | 节点连通性与 Qwen3Guard 响应探测 |
| GET | `/api/prompt-audit/events` | RootAuth | 分页多维筛选脱敏事件列表 |
| GET | `/api/prompt-audit/events/:id` | RootAuth | 解密详情，设置 `Cache-Control: no-store` |
| DELETE | `/api/prompt-audit/events/:id` | RootAuth | 单条删除，记录管理审计日志 |
| POST | `/api/prompt-audit/events/batch-delete` | RootAuth | 批量删除（<= 500 条），记录管理审计日志 |

### 2.7 启动与接线顺序 (`main.go` & `service/promptaudit/`)

1. `model.InitDB()`：执行主库 `migrateDB()`，创建/升级 `prompt_audit_events` 表与索引。
2. `model.InitOptionMap()`：加载基础 Option。
3. `service.InitPromptAudit()`：
   - 检查 `common.CryptoSecret` / `HasStableCryptoSecret()`；
   - 实例化 `AESGCMEncryptor`、`GormConfigStore`、`GormEventStore`、`Manager`；
   - 首次执行 `Manager.Reload(ctx)`；
   - 若当前为 Master 节点，启动定时保留清理任务。
4. `model.SyncOptions()` 定期同步回调中触发 `Manager.Reload(ctx)`，确保多实例最终一致。
5. 注册 `/api/prompt-audit` 路由组并挂载 `middleware.RootAuth()`。


