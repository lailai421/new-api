# 09-03-prompt-audit-storage-api 新上下文执行提示词

你正在仓库 `/Users/laiyanfei/code/python/ai-project/github/new-api` 中继续 Trellis
子任务 `09-03-prompt-audit-storage-api`。请全程使用中文，把该子任务真正推进到可交付状态；
只实现提示词审计的存储、生命周期、启动接线和 Root 管理 API，不要顺带实现协议门禁或前端。

## 一、先恢复上下文，不要直接编码

1. 完整读取仓库根目录 `AGENTS.md`，遵守其中全部工程、安全、测试、数据库和项目治理约束。
2. 使用 `trellis-continue` Skill 恢复工作流阶段，并明确把目标锁定为
   `09-03-prompt-audit-storage-api`。先运行 `python3 ./.trellis/scripts/task.py current`
   核对当前任务；不要因为当前指针仍指向父任务或 core 子任务而误做其他范围。
3. 完整读取以下权威资料，不能只依赖本提示词摘要：
   - `.trellis/tasks/09-03-prompt-audit/prd.md`
   - `.trellis/tasks/09-03-prompt-audit/design.md`
   - `.trellis/tasks/09-03-prompt-audit/implement.md`
   - `.trellis/tasks/09-03-prompt-audit/research/codebase-analysis.md`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/design.md`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/implement.md`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/implement.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/check.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-core/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-core/design.md`
   - `.trellis/tasks/09-03-prompt-audit-core/implement.md`
4. 检查 `git status`、最近提交和现有改动，保留用户及其他任务的工作，不得覆盖或回退无关修改。
   编写本提示词时，core 实现位于提交 `f01f90d8`，但新上下文必须以实际 HEAD 和工作区为准。
5. 核实强依赖是否真的可用：`service/promptaudit/` 应已实现并冻结 `Config`、
   `PublicConfig`、`PromptSnapshot`、`Decision`、`RuntimeState`、`ConfigStore`、`EventStore`、
   `Encryptor` 和 `Manager` 契约。当前 core 的 `task.json` 元数据可能仍为 `in_progress`，
   不能仅凭状态字段判断，也不能在本任务中擅自完成或重写 core；应以源码、core 验收记录和测试
   结果共同确认。若核心接口缺失、未冻结或无法通过定向测试，先报告依赖阻塞，不得另造一套实现。
6. 使用可用的 Serena 做符号级检索；若 Serena 不可用，使用 FastCtx 或 `rg` 降级。编码前至少
   核实已有 Option 读写、`lockForUpdate`、GORM 迁移、RootAuth 路由、管理日志、分页响应、
   后台清理任务、数据库测试夹具和启动顺序。
7. 规划门禁不可跳过。父任务和 storage-api 子任务目前仍是 `planning`。如果最新规划总结尚未由
   用户在后续消息中明确批准，先按 `trellis-brainstorm` 输出 Goal、In Scope、Out of Scope、
   Acceptance Criteria、Key Decisions、Risks/Deferred Items 和 artifact status 的最终规划总结，
   然后停止并等待明确批准。不要把本交接提示词本身视为实施批准。
8. 获得用户对最新规划总结的明确批准后，执行：

   ```bash
   python3 ./.trellis/scripts/task.py start 09-03-prompt-audit-storage-api
   ```

   随后使用 `trellis-before-dev` 注入项目规范，再开始修改业务代码。

## 二、任务目标与交付边界

把已冻结的 `service/promptaudit` 领域核心接入 `new-api` 的 Model、Service、Controller、Router
和启动流程，交付以下能力：

- `PromptAuditConfigSecret` 专用 Option 持久化、脱敏读取和基于 `config_version` 的 CAS 更新。
- 三数据库兼容的 `PromptAuditEvent` 表、索引、CRUD、筛选、分页与清理。
- 完整提示词经 core `Encryptor` 加密后无截断落库；只有 Root 详情接口按需解密返回。
- Block/Error 必存，Pass/Flag 受 `store_pass_events` 控制；必要事件写入失败时可稳定失败关闭。
- 默认永久保留、有限天数自动清理、单条删除及最多 500 条批量删除。
- `/api/prompt-audit` 下配置、运行状态、节点探测和事件管理 API，全部由 RootAuth 保护。
- 应用启动时初始化并加载 Prompt Audit Manager，多实例配置能够刷新，清理仅在允许的 master
  执行路径运行。

本子任务完成后必须冻结管理 API JSON 契约和事件存储语义，供 HTTP Relay、Realtime、Task
Plugin 与 web-console 子任务使用。

## 三、必须遵守的核心契约

先阅读实际源码，不要凭此摘要猜签名。当前 core 的关键事实如下：

- 配置键常量为 `promptaudit.OptionKeyPromptAuditConfigSecret`。
- `ConfigStore` 当前签名为：

  ```go
  type ConfigStore interface {
      Load(ctx context.Context) (*Config, error)
      Save(ctx context.Context, cfg *Config, expectedVersion int64) error
  }
  ```

- `EventStore` 当前签名为：

  ```go
  type EventStore interface {
      Record(
          ctx context.Context,
          snapshot PromptSnapshot,
          decision *Decision,
          storePassEvents bool,
      ) error
  }
  ```

- `Encryptor` 已提供 Guard Token 与完整 Prompt 的分域加解密，不得在 storage-api 再实现第二套
  AES、nonce 或密钥派生逻辑。
- `Config.Serialize`、`ParseConfig`、`NormalizeAndValidate`、`ToPublic`、`ToActive` 和
  `Manager.Reload` 已存在，应复用而不是复制配置语义。
- `DecisionKind` 包含 Allow、Flag、Block、Unavailable、Invalid；稳定错误码包含
  `prompt_guard_blocked`、`prompt_guard_unavailable`、`prompt_guard_invalid_response`、
  `prompt_audit_config_degraded`、`prompt_audit_record_failed`、
  `prompt_audit_unsupported_protocol`、`prompt_audit_config_conflict` 和
  `prompt_audit_encryption_key_required`。
- `PromptSnapshot.FullPrompt` 是敏感完整原文，`ScanText` 不得持久化；`Redacted()` 只清除
  `ScanText`，不会清除 `FullPrompt`，因此落库映射时仍必须显式只保存密文，不能直接序列化模型。

如果实现中发现冻结接口确实无法表达父任务的已确认规则，先写清冲突、影响范围和最小修正方案，
同步更新父任务及受影响子任务规划，并重新请求用户审批；不得静默改变 core 公共接口或产品语义。

## 四、严格范围

### 4.1 配置存储与 CAS

- 为 core `ConfigStore` 提供具体适配，使用主库 `Option` 表保存单个 JSON 值，键名只能使用
  `OptionKeyPromptAuditConfigSecret`。
- Model 层负责数据库事务和锁；CAS 必须在同一事务内使用 `lockForUpdate(tx)` 读取 Option、比较
  `expected_config_version`、写入新配置并递增版本。不得使用 GORM v1 的
  `gorm:query_option`，不得在调用点复制 `clause.Locking`。
- 首次保存要明确处理“不存在 Option”的版本语义；并发创建和并发更新都不能静默覆盖。
- CAS 冲突稳定映射为 HTTP 409 和 `prompt_audit_config_conflict`，不能只返回模糊数据库错误。
- 保存前对配置做规范化和校验；服务端生成 `updated_at`、`updated_by` 和新版本，不能信任客户端
  直接提交这些审计字段。
- 节点 Token 是 write-only 输入：持久化前调用 core `EncryptToken`；读取配置只返回
  `has_token` / `token_status`。更新未提供新 Token 时保留已有密文；明确删除 Token 时使用清晰
  的专用 DTO 语义，不能把空字符串含混地解释为既“保留”又“删除”。
- `controller.GetOptions` 必须显式排除该键；通用 `controller.UpdateOption` 和底层通用 Option
  更新入口都必须拒绝该键，避免绕过专用校验、加密和 CAS。不要只依赖键名带 `Secret` 的通用
  后缀过滤。
- 专用密文不得进入普通配置响应、PublicConfig、管理日志、错误响应或调试日志。若仍需借助
  `common.OptionMap` 的同步机制，必须保证读取和传播路径不会把值暴露给通用消费者，并为此写测试。
- 保存成功后本实例立即 `Manager.Reload`；其他实例按项目既有 Option 同步频率重新加载。
  配置不存在时安全默认关闭；配置存在但解析、解密、校验或激活失败时 runtime 必须 degraded，
  不得退化为关闭并放行。

### 4.2 事件模型与三数据库兼容

- 新增 `model/prompt_audit.go`，定义 `PromptAuditEvent` 及必要的查询/删除参数，不让 Controller
  直接返回 GORM 模型。
- 模型至少覆盖父设计中的身份快照、请求元数据、Prompt hash/预览/长度、加密正文、Decision、
  scanner 结果、Guard/策略/config 版本、分片/延迟、错误码和 `created_at`。
- 完整事件字段以父任务 `design.md` 第 8 节为权威，不得为了省事删减会影响后续审计核查的字段。
- `full_prompt_ciphertext` 使用自定义 GORM 数据类型：MySQL 映射 `LONGTEXT`，PostgreSQL 与
  SQLite 映射 `TEXT`。必须通过真实迁移和超过 64 KiB 原文往返证明不会被 MySQL `TEXT` 截断。
- JSON 类字段存为 `TEXT`，所有编解码调用 `common.Marshal`、`common.Unmarshal`、
  `common.UnmarshalJsonStr` 或 `common.DecodeJson`。业务代码不得直接调用 `encoding/json` 的
  Marshal/Unmarshal；可以仅引用 `json.RawMessage` 等类型。
- 不建立外键，不使用 JSONB、BIGSERIAL、TIMESTAMPTZ、SKIP LOCKED、方言专用 CHECK、
  `AUTO_INCREMENT` 或 `SERIAL`。主键生成交给 GORM。
- 使用 GORM tag 与方法实现三库共同语义。索引围绕 `(created_at, id)`、request_id、user_id、
  token_id、group、decision、risk_level、guard_endpoint_id 和 prompt_hash 设计，并核实索引名和
  长度在 MySQL、PostgreSQL、SQLite 都合法。
- 将事件模型加入主库 `migrateDB()` 的 `AutoMigrate`；不要误加到可选日志库或 ClickHouse
  `LOG_DB`。迁移必须可重复执行，不得破坏已有业务表。
- GORM 模型的 JSON 标签必须保证 `full_prompt_ciphertext` 永不被普通序列化；列表查询还应
  使用显式 Select/DTO 映射，形成双重保护。

### 4.3 EventStore、查询、详情与删除

- 实现 core `EventStore.Record`。先根据 Decision 决定是否需要记录：
  - Block、Unavailable、Invalid 以及携带错误码的失败事件必须记录；
  - Allow/Flag 只在 `store_pass_events=true` 时记录；
  - `store_pass_events=false` 时跳过 Pass/Flag 写库并允许后续阶段继续。
- 需要记录时，使用 core `EncryptPrompt` 加密 `snapshot.FullPrompt`，只保存密文和安全元数据；
  不得保存 `ScanText` 明文，也不得把 `FullPrompt` 放进错误包装、SQL 日志或结构化日志字段。
- 必要事件的加密或数据库写入失败必须向调用者返回可识别错误，使后续 Gate 映射为
  `prompt_audit_record_failed` 并拒绝业务上游。不得吞错、异步补记或失败后继续放行。
- 对 nil Decision、缺少 Result 的错误 Decision、空字符串、Unicode、NUL、超长正文和损坏密文
  给出确定行为及测试。不要用 panic 或无界内存复制处理异常输入。
- 列表接口只能返回脱敏预览和元数据，绝不返回 `full_prompt`、密文、Guard Token、ScanText 或
  可能包含原文的 scanner evidence。详情接口由 Service 按事件 ID 查询、解密并返回完整原文；
  解密失败返回稳定的安全错误，不回显密文或内部密钥信息。
- 列表支持父设计要求的时间、用户、Token、分组、模型、协议、decision、risk、category、
  Guard 节点、request_id 和 prompt_hash 筛选。分页大小、时间跨度、游标或 page/limit 语义应
  结合现有 API 惯例确定并写入 DTO 测试；排序必须稳定，推荐 `created_at DESC, id DESC`。
- 单条删除和批量删除只删除明确 ID；批量上限 500，空列表、重复 ID、不存在 ID 和超限请求均
  有确定响应。删除不可恢复，Controller 契约应支持前端二次确认，但后端不能依赖前端保证安全。
- 删除前后记录现有管理审计日志，只允许 actor、请求 ID、事件 ID、数量和安全筛选条件；禁止
  写入提示词、预览、密文、Token 或 Guard 原始响应。

### 4.4 保留期与后台清理

- `retention_days=0` 表示永久，不执行自动删除；大于 0 时按当前已激活配置计算截止时间，并能
  在配置修改后按新策略处理既有数据。
- 清理按 `created_at < cutoff` 选择最多 500 个明确 ID，再按 ID 批量删除。为单次运行设置有界
  批次数，避免长事务、长时间占锁和无界循环。
- 清理函数本身应幂等，重复执行不会误删截止时间等于 cutoff 或尚未过期的数据。
- 自动循环按小时运行，并遵循 `common.IsMasterNode` 及项目现有后台任务启动惯例；多 master
  环境不得依赖进程内锁保证全局唯一。优先复用已有数据库租约/系统任务机制，若采用幂等批删
  容忍重复执行，必须在设计和测试记录中解释安全性，不要引入新外部基础设施。
- 测试禁止真实 sleep；将当前时间、批大小或单次执行逻辑设计为可注入/可直接调用的确定性边界。
- 自动清理和手动删除日志都不能包含提示词原文。自动清理失败应记录安全元数据并等待下一周期，
  不得影响主服务启动或请求处理。

### 4.5 Root 管理 API

在 `router/api-router.go` 注册 `/api/prompt-audit` 路由组，并统一使用
`middleware.RootAuth()`：

- `GET /config`：返回脱敏 `PublicConfig`、scanner catalog 和 `config_version`。
- `PUT /config`：接收专用 DTO、`expected_config_version` 和 write-only Token 变更，校验、加密、
  CAS 保存并立即激活；冲突返回 409。
- `GET /runtime`：返回有效模式、expected/active version、loaded_at、degraded、加载错误码、
  启用节点数。未覆盖 Task Plugin 列表属于后续 task-plugin 子任务的数据来源：本任务只能冻结
  清晰的 DTO/注入边界，不得虚构“全部已覆盖”，也不要越界修改插件系统。
- `POST /endpoints/probe`：探测待保存节点或已保存节点，复用 core 的安全 HTTP Client 与
  Qwen3Guard 解析；返回延迟、模型和规范化错误，不回显 Token、密文、请求提示词、Guard 原始
  响应或内部错误。探测不得保存配置，不得调用业务渠道或计费。
- `GET /events`：分页、筛选，只返回脱敏列表 DTO。
- `GET /events/:id`：Root 按需解密并返回完整 `full_prompt`，响应必须设置
  `Cache-Control: no-store`，并避免中间缓存。
- `DELETE /events/:id`：删除单条并记录不含原文的管理日志。
- `POST /events/batch-delete`：按明确 ID 批量删除，最多 500 条。

所有接口只接受专用 `dto/prompt_audit.go` 类型。参数绑定、ID、分页、时间范围、数组长度和配置
边界必须验证；不要把数据库错误或敏感内部状态直接返回客户端。权限测试至少覆盖未登录、普通用户、
Admin 和 Root，证明只有 Root 能访问，且只有详情接口返回完整原文。

### 4.6 启动与运行时接线

- 在不会造成包循环的位置建立单一 Prompt Audit Manager/Encryptor/ConfigStore/EventStore 组合。
  Model 层保持纯持久化边界，避免 `model` 反向依赖 `service/promptaudit` 后又由该包依赖 model。
- 初始化必须发生在主数据库迁移、Option 初始化和环境密钥加载之后，并在 Router/Controller 使用
  之前完成首次 Reload。关闭配置不存在时允许正常启动；已有启用配置损坏时 Manager 必须进入
  degraded，服务可启动但审计请求后续必须失败关闭。
- 配置同步应让每个实例最终刷新 Manager，而自动保留清理只在允许的 master 执行路径启动。
- 暴露给后续协议子任务的是稳定、线程安全的 Manager/EventStore 访问边界，不得让每个 Controller
  自己创建 Manager、重复解密配置或直接操作 Option。
- 仅当必要时修改 `main.go`；遵守现有初始化次序，避免在数据库、logger 或环境变量尚未可用时
  启动 goroutine。

### 4.7 明确不实现

- 不实现具体协议 PromptSnapshot 提取器。
- 不修改 `controller/relay.go`，不接入 HTTP Relay、Midjourney、Realtime 或 Task Plugin 门禁。
- 不修改插件 Meta、`auditTextPaths`、内置插件或 Responses Bridge。
- 不实现前端页面、菜单、React Query 或国际化文案。
- 不修改 `relaykit/`，也不能让 `relaykit/` 依赖根模块。
- 不引入异步只审计、事件队列、Worker、Redis 原文载荷、租约重试或审计失败后放行语义。
- 不修改任何受保护的 new-api / QuantumNous 项目身份、品牌、元数据或归属信息。

## 五、实现前必须核实的仓库证据

至少检查并记录下列现状，再决定具体文件和符号：

- `model/option.go`：`Option` 结构、`AllOption`、`loadOptionsFromDatabase`、`UpdateOption`、
  `UpdateOptionsBulk` 和 `common.OptionMap` 更新方式。
- `controller/option.go`：`GetOptions` 的敏感键过滤、`UpdateOption` 拒绝模式和
  `recordManageAudit` 调用方式。
- `model/locking.go`：`lockForUpdate` 在 SQLite、MySQL、PostgreSQL 的实际行为。
- `model/main.go:migrateDB()`：主库 AutoMigrate 清单、数据库类型分支和迁移幂等惯例。
- `router/api-router.go`：Root 专用路由组、CriticalRateLimit/DisableCache 等中间件复用方式。
- `main.go:InitResources()` 与主启动流程：环境、DB、Option、日志、Router 和后台服务的先后顺序。
- `service/auth_cleanup.go`、`service/system_task.go` 等：master-only 清理、数据库租约和可测试调度
  模式；选择与父设计一致的最小机制。
- 现有列表/详情/删除 Controller 与 DTO：统一成功/错误响应、分页字段、404、参数校验和管理日志。
- 现有数据库测试工具、Docker/CI 配置和最低支持版本，避免另建互不兼容的测试框架。
- `service/promptaudit/` 实际公共 API 与 core 定向测试，不得根据旧规划文档复制已经变化的签名。

## 六、质量与测试要求

- 遵守 Router → Controller → Service → Model 分层。代码直接、可读、低嵌套，使用早返回和清晰
  名称；不要新增只有一个调用点且没有稳定领域意义的包级 helper。
- 必要注释使用中文，重点解释敏感数据边界、CAS、三库差异、失败关闭和清理幂等，不复述代码。
- 新增或大幅改写的 Go 测试使用 `testify/require` 做前置与致命断言，使用 `testify/assert`
  做非致命断言。采用确定性表驱动测试，不添加随机压力、sleep、计时比较或只为覆盖率的测试。
- 测试至少覆盖：
  - Option 首次创建、正常 CAS、旧版本冲突、并发更新、通用读写接口绕过防护。
  - Token 保留/替换/明确删除、PublicConfig 脱敏、保存后 Reload、损坏配置和错误密钥 degraded。
  - Allow/Flag/Block/Unavailable/Invalid 的持久化规则，以及必要加密/写库失败的稳定错误。
  - Unicode、NUL、空文本、损坏密文与超过 64 KiB 原文逐字符加解密相等。
  - 列表永不含原文/密文/evidence，详情只有 Root 可读并带 `Cache-Control: no-store`。
  - 所有筛选、稳定排序、分页边界、404、单删、批删上限与重复 ID。
  - retention=0、cutoff 前/等于/后、批量上限、单次运行上限和重复清理幂等。
  - 未登录、普通用户、Admin、Root 的完整权限矩阵。
  - 节点 probe 成功、超时、4xx/5xx、非法响应和敏感信息不回显。
  - 启动首次加载、跨实例刷新边界及非 master 不启动自动清理。
- 每条后台测试命令的超时不超过 60 秒。按实际受影响包拆分执行并记录，例如：

  ```bash
  gofmt -w model/prompt_audit*.go dto/prompt_audit*.go controller/prompt_audit*.go service/promptaudit/*.go
  go test -count=1 -timeout 55s ./service/promptaudit
  go test -count=1 -timeout 55s ./model
  go test -count=1 -timeout 55s ./controller
  go test -count=1 -timeout 55s ./router
  go vet ./service/promptaudit ./model ./controller ./router
  ```

  文件不存在或包测试过大时按实际情况调整为更精确的命令；不要为了照抄示例制造无关改动。
- 检索并确认新增业务代码没有直接调用 `encoding/json` 的 Marshal/Unmarshal；检索日志、响应和
  DTO，确认 Token、密文、FullPrompt、ScanText、Guard Body/Response 不会泄漏。
- 本子任务明确涉及数据库行为，必须使用真实 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 完成：
  1. 空库迁移并连续运行两次；
  2. 从最新发布版代表性数据库升级并再次重复迁移；
  3. 超过 64 KiB 密文写入、读取、解密逐字符相等；
  4. CAS 并发更新；
  5. 索引、分页和筛选；
  6. retention 清理、单删和批删；
  7. 既有数据、约束及其他业务表不受影响。
- 记录精确数据库版本、启动方式、验证命令和结果。任何一个真实数据库或升级矩阵未执行，都必须
  在交付中列为阻塞项，不能宣称任务完整、迁移安全或“三数据库兼容”。
- 本任务原则上不得触及 `relaykit/`；若实际出现改动，必须额外执行：

  ```bash
  cd relaykit && GOWORK=off go build ./...
  ```

## 七、执行纪律与完成标准

1. 编码前先列出精确的数据模型、DTO、API 响应、CAS 语义、初始化顺序及拟修改文件；技术细节
   可以根据仓库事实确定，涉及产品范围或安全取舍的冲突必须先回到规划审批。
2. 小步实现并持续运行定向测试；不要修改无关文件，不做顺手重构，不引入新依赖，除非现有能力
   确实无法满足且已说明必要性。
3. 不得让 `model`、`service/promptaudit`、`controller` 形成导入环；不要通过全局可变明文配置规避
   正确分层。
4. 实现后使用 `trellis-check` 做独立质量检查，修复所有范围内问题并重新验证。
5. 检查 `git diff` 和 `git status`，确认没有敏感数据、无关改动、占位符、TODO、调试端点或旧兼容
   路径残留。
6. 更新 storage-api 的 `prd.md`、`design.md`、`implement.md`，记录最终 API 契约、事件字段、CAS
   语义、初始化边界、验证命令和结果；若冻结契约影响下游，必须同步相关子任务文档中的依赖说明。
7. 按根 `AGENTS.md` 要求把关键约束、最佳实践和经验写入 Serena memory；若 Serena 不可用，
   明确说明降级情况，不得伪造已写入。
8. 不执行 `git commit` 或 `git push`，除非用户另行明确授权。
9. 只有以下条件全部满足，才可声明 storage-api 子任务完成：
   - 子任务每项验收标准都有实现和测试证据；
   - core 接口未被私自复制或产生语义分叉；
   - 管理 API 与事件存储契约已冻结，足以支撑后续四个子任务；
   - 敏感原文只以密文落库，只有 Root 详情按需解密；
   - 必要事件失败关闭、CAS、清理和权限行为均已验证；
   - SQLite、MySQL、PostgreSQL 的真实新建、升级、重复迁移和行为矩阵全部完成；
   - 定向测试、静态检查和 Trellis 质量门禁全部通过；
   - 没有越过本子任务范围。

最终交付请用中文简洁报告：完成内容、冻结的管理 API/存储契约、关键文件、测试和静态检查结果、
三数据库精确版本与验证结果、未完成项或风险、是否已满足 HTTP Relay/Realtime/Task Plugin/
web-console 子任务启动条件。任何未执行的验证必须明确标为阻塞项，不得用代码审查或 SQLite
单库结果替代真实三数据库验证。
