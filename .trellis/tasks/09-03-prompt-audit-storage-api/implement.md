# 提示词审计存储与管理 API 实施记录 (09-03-prompt-audit-storage-api)

## 状态：COMPLETED (已全部完成并验证)

## 一、前置输入与依赖核查

- 强依赖 `service/promptaudit/` 领域核心（提交 `f01f90d8`），已通过定向测试验证并保持与 `Config`、`PublicConfig`、`ActiveConfig`、`PromptSnapshot`、`Decision`、`RuntimeState`、`ConfigStore`、`EventStore`、`Encryptor`、`Manager`、`GuardError` 契约严格兼容。
- 严格遵守仓库根目录 `AGENTS.md`：
  - 严禁触碰/修改 `new-api` 和 `QuantumNous` 保护名称。
  - 绝不直接调用 `encoding/json`，全量走 `common.Marshal` / `common.Unmarshal`。
  - `relaykit` 模块完全独立验证通过 (`cd relaykit && GOWORK=off go build ./...`)。
  - 真实三数据库矩阵验证通过（SQLite-3, MySQL 8.0.46, PostgreSQL 18.6）。
  - 没有顺带实现协议门禁提取器或前端 UI，职责边界清晰。

---

## 二、完成的实施清单与代码交付

### Step 1: Option CAS 持久化与防绕过隔离
- `model/option.go`:
  - 注册 `OptionKeyPromptAuditConfigSecret = "PromptAuditConfigSecret"` 与 `ErrPromptAuditConfigConflict`。
  - `validateOptionValue` 与 `updateOptionMap` 强制拦截该 Key，禁止通过普通 `UpdateOption` 或 `UpdateOptionsBulk` 绕过版本控制。
  - `GetPromptAuditConfigRaw(ctx)` 与 `SavePromptAuditConfigCAS(ctx, expectedVersion, mutateFn)` 在事务内通过 `lockForUpdate(tx)` 行级锁保护原子检查版本与递增版本，并同步触发 `promptAuditConfigSyncHook`。
  - `getKeyCol()` 健壮获取转义 key 列名（防止 dialect 差异导致空列名语法错误）。
- `controller/option.go`:
  - `GetOptions` 显式排除 `OptionKeyPromptAuditConfigSecret`，防止敏感配置泄露至全量 Options 响应。
  - `UpdateOption` 拦截并返回 403。
- `model/option_prompt_audit_test.go`:
  - 覆盖首次创建（0/1 成功，其他报冲突）、CAS 更新、版本不一致 409 冲突、并发竞争仅一个成功、同步 Hook 触发。

### Step 2: PromptAuditEvent 模型与跨库长文本
- `model/prompt_audit.go`:
  - 定义 `PromptCiphertext` 自定义类型，在 MySQL 方言映射为 `longtext`，在 PostgreSQL / SQLite 映射为 `text`，实现 `driver.Valuer` 与 `sql.Scanner`。
  - 定义 `PromptAuditEvent` 结构体（包含 34 个审计字段，`FullPromptCiphertext` 标记 `json:"-"`，主键与 `CreatedAt` 复合索引）。
  - 实现 CRUD：`CreatePromptAuditEvent`、`GetPromptAuditEventByID`、`DeletePromptAuditEventByID`、`BatchDeletePromptAuditEvents`（限制 1..500 条且去重）。
  - 实现 `getPromptAuditListSelectCols()` 动态列投影，在运行时引用 `commonGroupCol`，显式排除 `full_prompt_ciphertext` 与 `scanner_evidence_json`。
  - 实现多维条件分页查询 `GetPromptAuditEvents` 及有界小批次清理 `CleanupPromptAuditEvents`。
- `model/main.go`:
  - 导出 `InitCol()`，并在 `migrateDB()` 的 `AutoMigrate` 注册列表中加入 `&PromptAuditEvent{}`。
- `model/prompt_audit_test.go`:
  - 覆盖方言类型映射、>64 KiB (~72 KiB) 密文读写往返、分页多维筛选与列投影排除、批量删除边界、有界分批清理。

### Step 3: EventStore、加解密与生命周期服务
- `service/promptaudit/config_store.go`:
  - 实现核心 `ConfigStore` 接口：`Load(ctx)` 与 `Save(ctx, cfg, expectedVersion)`，调用 `model.SavePromptAuditConfigCAS`，将冲突确定性映射为 `GuardError(409, ErrorCodeConfigConflict)`。
- `service/promptaudit/event_store.go`:
  - 实现核心 `EventStore` 接口：根据 `Decision` 判定落库决策（Block/Unavailable/Invalid 必写；Allow/Flag 依据 `storePassEvents` 判定）；调用 `Encryptor.EncryptPrompt` 对原文强制加密；写入失败返回 `GuardError(503, ErrorCodeRecordFailed)` 保证失败关闭。
- `service/promptaudit/cleanup.go`:
  - 实现 `RunCleanupOnce`（支持参数化 `nowUnix` 与批大小）；实现 `StartRetentionCleanup`，仅在 `common.IsMasterNode` 启动后台 Ticker 自动按保留天数清理历史数据。
- `service/promptaudit/init.go`:
  - 单例管理与组装：使用 `common.CryptoSecret` 派生 `AESGCMEncryptor`，组装 `GormConfigStore`、`GormEventStore`、`OpenAICompatibleScanner`、`GuardEvaluator` 与 `Manager`；挂载 `model.SetPromptAuditConfigSyncHook`；Master 节点启动保留期清理。

### Step 4: DTO 与 Root 管理 Controller
- `dto/prompt_audit.go`:
  - 定义管理 API 交互 DTO：配置更新（支持单个端点明文 Token 增量写入或 `DeleteToken` 清空）、端点探测请求/响应、列表多维筛选、列表单项（脱敏无原文）、详情（解密原文）、批删。
- `controller/audit.go`:
  - 注册 4 个管理审计操作模板：`prompt_audit.config_update`、`prompt_audit.endpoint_probe`、`prompt_audit.event_delete`、`prompt_audit.event_batch_delete`。
- `controller/prompt_audit.go`:
  - `GetPromptAuditConfig`: 脱敏回显，包含可用扫描器目录定义与 `config_version`。
  - `UpdatePromptAuditConfig`: 基于 `expected_config_version` 的 CAS 更新，Token 密文存储，记录管理审计日志。
  - `GetPromptAuditRuntime`: 返回运行时降级状态、激活版本与端点状态。
  - `ProbePromptAuditEndpoint`: 执行连通性探测，绝不泄露 Token。
  - `GetPromptAuditEvents`: 分页查询，列表绝不返回完整密文或原文。
  - `GetPromptAuditEvent`: 响应注入 `Cache-Control: no-store` 标头，解密回显 `full_prompt`。
  - `DeletePromptAuditEvent` / `BatchDeletePromptAuditEvents`: 1..500 范围限制，记录管理审计。
- `controller/prompt_audit_test.go`:
  - 覆盖 RootAuth 鉴权门禁（未认证 401、普通用户 403、Admin 403、Root 200）；
  - 覆盖配置获取脱敏、CAS 首次写入、CAS 冲突 409、Token 删除清空；
  - 覆盖运行时状态查询；
  - 覆盖列表无原文、详情原文解密回显与 `Cache-Control: no-store` 标头校验；
  - 覆盖批量删除 0 条与 >500 条拦截。

### Step 5: 路由注册与应用启动接线
- `router/api-router.go`:
  - 注册 `/api/prompt-audit` 路由组，绑定 `middleware.RootAuth()`，详情挂载 `middleware.DisableCache()`。
- `main.go`:
  - 在 `InitResources()` 中紧随 `model.InitOptionMap()` 之后调用 `promptaudit.InitPromptAudit()`。

---

## 三、验证记录与三数据库实测证据

### 1. relaykit 独立构建验证
```bash
cd relaykit && GOWORK=off go build ./...
# 退出码: 0 (构建成功，无外部依赖)
```

### 2. 静态检查与格式化
```bash
go vet ./model ./service/promptaudit ./controller
# 退出码: 0 (无任何 vet 报警)
gofmt -l <modified_files>
# 全部格式化合规
```

### 3. 三数据库真实矩阵实测验证 (`model/prompt_audit_matrix_test.go`)
实测测试集：`TestPromptAudit_ThreeDatabaseMatrix` 与 `TestPromptAudit_UpgradeMatrix`
- **SQLite**:
  - 版本: SQLite 3.x (内存/文件模式)
  - 初始建表、再次建表(幂等性): PASS
  - 字段类型: `text` PASS
  - >64 KiB (~72 KiB) 密文读写字节级一致性: PASS
  - Option CAS (初始 0->1, 更新 1->2, 冲突 1 拒绝): PASS
  - 列表筛选与投影剥离、批量删除、保留期分批清理: PASS
  - 旧库升级迁移测试: PASS
- **MySQL**:
  - 版本: MySQL Community Server 8.0.46 (Linux aarch64 Docker)
  - DSN: `root:root@tcp(127.0.0.1:13306)/new_api_test`
  - 初始建表、再次建表(幂等性): PASS
  - 字段类型: `longtext` (从 `INFORMATION_SCHEMA.COLUMNS` 校验) PASS
  - >64 KiB (~72 KiB) 密文读写字节级一致性: PASS
  - Option CAS (行锁 `lockForUpdate`、冲突拒绝): PASS
  - 列表筛选、批量删除、保留期清理: PASS
  - 旧库升级迁移测试: PASS
- **PostgreSQL**:
  - 版本: PostgreSQL 18.6 (Alpine Linux aarch64 Docker)
  - DSN: `host=127.0.0.1 port=15432 user=postgres dbname=new_api_test`
  - 初始建表、再次建表(幂等性): PASS
  - 字段类型: `text` (从 `information_schema.columns` 校验) PASS
  - >64 KiB (~72 KiB) 密文读写字节级一致性: PASS
  - Option CAS (行锁 `lockForUpdate`、冲突拒绝): PASS
  - 列表筛选、批量删除、保留期清理: PASS
  - 旧库升级迁移测试: PASS

**实测输出片段**：
```
=== RUN   TestPromptAudit_ThreeDatabaseMatrix
=== RUN   TestPromptAudit_ThreeDatabaseMatrix/SQLite-3
=== RUN   TestPromptAudit_ThreeDatabaseMatrix/MySQL-8.0
=== RUN   TestPromptAudit_ThreeDatabaseMatrix/PostgreSQL-18
--- PASS: TestPromptAudit_ThreeDatabaseMatrix (0.62s)
    --- PASS: TestPromptAudit_ThreeDatabaseMatrix/SQLite-3 (0.01s)
    --- PASS: TestPromptAudit_ThreeDatabaseMatrix/MySQL-8.0 (0.17s)
    --- PASS: TestPromptAudit_ThreeDatabaseMatrix/PostgreSQL-18 (0.44s)
=== RUN   TestPromptAudit_UpgradeMatrix
=== RUN   TestPromptAudit_UpgradeMatrix/SQLite-3-Upgrade
=== RUN   TestPromptAudit_UpgradeMatrix/MySQL-8.0-Upgrade
=== RUN   TestPromptAudit_UpgradeMatrix/PostgreSQL-18-Upgrade
--- PASS: TestPromptAudit_UpgradeMatrix (0.22s)
    --- PASS: TestPromptAudit_UpgradeMatrix/SQLite-3-Upgrade (0.00s)
    --- PASS: TestPromptAudit_UpgradeMatrix/MySQL-8.0-Upgrade (0.08s)
    --- PASS: TestPromptAudit_UpgradeMatrix/PostgreSQL-18-Upgrade (0.14s)
PASS
ok  	github.com/QuantumNous/new-api/model	1.578s
```

### 4. 领域与控制层单元测试
- `model`: PASS (100%)
- `service/promptaudit`: PASS (100%, 34 个测试用例全部通过)
- `controller`: PASS (100%, 5 个综合测试用例全部通过)


