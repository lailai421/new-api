# 提示词审计技术设计

## 1. 设计目标

在不改变现有业务渠道协议和计费语义的前提下，为 `new-api` 的所有提示词入口增加同步、失败关闭的审计门禁。审计关闭时不影响现有链路；审计开启且命中分组范围时，只有 Guard 返回可放行结论且必要事件已成功持久化后，内容才能到达业务上游。

本设计以 `sub2api` 的 Qwen3Guard/OpenAI 兼容实现为行为参考，按 `new-api` 的 Go/Gin/GORM/React 架构重写，不复制 PostgreSQL 专用异步队列。

## 2. 已确认的产品规则

- 模式只有 `off` 和 `blocking`。
- 支持全部分组与指定分组；分组使用 `new-api` 字符串标识。
- 覆盖全部含提示词入口，包括 HTTP Relay、Realtime、Midjourney、视频和 Task Plugin。
- 事件保存完整、未脱敏提示词，授权 Root 管理员可在详情查看。
- 默认永久保留；可配置保留天数；支持手动删除。
- “保存通过事件”默认开启；关闭后只保存 Block 与 Error。
- “仅审计最新用户输入”可配置但默认关闭；默认审计完整请求输入。
- Guard 不可用、超时、响应非法、配置 degraded、插件缺少提取契约时失败关闭。

## 3. 总体架构

```text
客户端请求
  │
  ├─ TokenAuth / 请求解析 / 本地渠道选择
  │
  ├─ 协议提取器 ──> PromptSnapshot
  │                    ├─ FullPrompt（完整原文，仅内存）
  │                    ├─ ScanText（完整或最新轮）
  │                    ├─ Hash / 脱敏预览 / 身份快照
  │                    └─ 协议、路径、模型、阶段
  │
  ├─ PromptAudit Gate
  │    ├─ 配置快照与分组匹配
  │    ├─ 分片
  │    ├─ 有序 Guard 节点故障切换
  │    ├─ 严格结果解析与聚合
  │    └─ 事件加密持久化
  │
  ├─ Allow / Warn ──> token 估算、预扣费、业务上游
  │
  └─ Block / Error ──> 403/503，本地结束，不计费、不请求业务上游
```

### 3.1 分层与文件组织

- `dto/prompt_audit.go`：管理 API 请求/响应 DTO，避免模型直接暴露。
- `model/prompt_audit.go`：事件模型、分页查询、详情查询、单条/批量/过期删除。
- `model/option.go`：审计配置 Option 的 CAS 持久化入口。
- `service/promptaudit/`：稳定领域包，包含配置快照、加密、提取、Guard 调用、结果聚合、事件记录和清理任务。
- `controller/prompt_audit.go`：Root 管理 API。
- `controller/relay.go`：HTTP、Midjourney、Task 统一门禁接线。
- `relay/channel/openai/relay_realtime.go`：Realtime 客户端帧发送上游前的逐帧门禁。
- `pkg/jsplugin/` 与 `docs/plugin-api/v1.schema.json`：Task Plugin 的 `auditTextPaths` 契约。
- `web/src/features/prompt-audit/`：独立配置和事件管理 feature。

`relaykit/` 保持不变。根模块通过具体 DTO 类型分派，不把审计逻辑下沉到独立模块，避免破坏 relaykit 的独立构建能力。

## 4. 配置设计

### 4.1 持久化结构

使用现有 `Option` 表保存一个 JSON 配置，键名为 `PromptAuditConfigSecret`。该键因包含节点密文必须：

1. 在 `controller.GetOptions` 中显式排除；
2. 禁止通过通用 `UpdateOption` 修改；
3. 只能通过 `/api/prompt-audit/config` 读写；
4. PublicConfig 永不返回 Token 或 Token 密文。

内部配置字段：

```text
enabled                    bool
latest_turn_only           bool
store_pass_events          bool（默认 true）
all_groups                 bool（默认 true）
groups                     []string
scanners                   []string（默认九类全选）
retention_days             int（0 表示永久）
strategy                   "priority"
endpoints                  []Endpoint
config_version             int64
updated_at                 int64
updated_by                 int
```

Endpoint 字段：`id`、`name`、`protocol=openai_compatible`、`base_url`、`model`、`token_ciphertext`、`timeout_ms`、`input_limit`、`enabled`。默认模型 `sileader/qwen3guard:0.6b`，默认超时 3000ms，默认输入上限 4000 Unicode 字符。

边界沿用参考实现：超时 100—30000ms，输入上限 128—100000 字符；启用审计时至少有一个启用且凭据可用的节点；指定分组模式至少有一个非空分组；至少启用一个风险分类；节点 ID 唯一。

### 4.2 CAS 与运行时快照

PUT 请求必须携带 `expected_config_version`。Model 层事务内用 `lockForUpdate(tx)` 读取 Option、比较版本、写入 `version + 1`；冲突返回 409 `prompt_audit_config_conflict`。

Service 层持有不可变 `ActiveConfig` 原子快照。保存成功后本实例立即加载，新配置由现有 Option 同步机制传播到其他实例。运行状态暴露 expected version、active version、loaded_at、load_error 和 degraded。不存在配置键时安全默认关闭；配置键存在但解析、解密或激活失败时进入 degraded，审计请求返回 503，不得退化成关闭状态。

### 4.3 密钥保护

节点 Token 和完整提示词使用 AES-256-GCM 加密后持久化。密钥由 `common.CryptoSecret` 通过 SHA-256/HKDF 风格用途隔离派生：节点 Token、提示词正文分别使用不同上下文；每次加密生成随机 nonce；密文采用带版本的 Base64 envelope，便于未来轮换。

启用审计前必须确认部署显式设置了稳定的 `CRYPTO_SECRET` 或 `SESSION_SECRET`。若只使用进程随机默认值，保存启用配置返回 400；已启用实例发现密文不可解密时进入 degraded。PublicConfig 只返回 `has_token` 与 `token_status`。

## 5. 提示词快照与提取契约

### 5.1 PromptSnapshot

```text
request_id、user_id、username、user_email
token_id、token_name、group
request_path、protocol、model、stage
full_prompt、scan_text、prompt_hash、redacted_preview
prompt_length、message_count、audit_scope
channel_id、channel_type（仅作为本地选择快照，不表示已调用上游）
```

`full_prompt` 是本请求所有已定义输入段按稳定顺序拼接的完整原文，不截断；`scan_text` 根据 `latest_turn_only` 选择完整输入或“最新用户轮次 + 最近一轮 assistant/model 输出”。即使只审计最新轮，事件仍保存完整 `full_prompt` 并记录 `audit_scope=latest_turn`。

Hash 使用 SHA-256 计算完整原文；列表预览先移除 Bearer/API Key/Token/Secret/密码、邮箱和电话，再保留最多 96 个 Unicode 字符的少量前缀。原文不能写入 logger、错误对象、指标标签或节点探测结果。

### 5.2 协议提取矩阵

- OpenAI Chat/Completions：`messages` 中 system、developer、user、assistant、tool 的文本内容；传统 completions 的 `prompt`。
- OpenAI Responses/Compaction：`instructions`、`input` 字符串，以及输入项的 text/input_text/output_text 和客户端可控角色内容。
- Claude Messages：`system` 与 messages 中各角色文本块。
- Gemini：`systemInstruction/system_instruction`、contents/content/requests/instances 中的 text/prompt。
- Images/媒体：`prompt`、`negative_prompt` 等协议 DTO 明确定义的文本字段；忽略 URL、Data URL 和 Base64。
- Embeddings/Rerank/Alpha Search：只提取将被业务上游使用的 input/query/documents 文本；不扫描模型名和业务元数据。
- Audio Speech：提取 input 文本；Transcription/Translation 的二进制音频不做“文本提示词”扫描，若存在客户端提供的 prompt 字段则扫描该字段。
- Midjourney：提交类动作的 `prompt` 与确实作为提示指令使用的 `content`；查询/通知类动作跳过。
- Realtime：逐帧提取 session.instructions、工具 description/parameters 中的文本、conversation item text/transcript 和 response.create 的输入文本；忽略音频 Base64 与纯控制字段。
- Task Plugin：从 decode 后的规范化 `task_request` 按插件声明的 `auditTextPaths` 提取。

提取器必须按具体协议/DTO 编写确定逻辑。默认分支不得递归收集任意字符串。已知提示词入口解析失败或无法建立提取契约时返回 `prompt_audit_unsupported_protocol` 并失败关闭；确认无文本输入的查询/控制请求返回 NoPrompt 并跳过事件。

### 5.3 Task Plugin 契约

在插件 v1 Meta 增加可选 `auditTextPaths: []string`，路径使用受限 JSON Pointer，指向 decode 后的规范化 `requestBody` 字符串或字符串/内容块数组。禁止通配任意对象所有值；路径数量、深度和提取总量受请求体既有上限保护。

- 内置插件全部声明其 `prompt`、`negative_prompt`、`gpt_description_prompt`、lyrics 等路径。
- 上传插件在注册时校验路径格式。
- 审计开启后，submit/dynamic 路由的规范化请求包含可发送文本但插件没有有效路径时，返回 503 `prompt_audit_unsupported_protocol`。
- 运行状态返回未覆盖插件列表，管理员在启用前即可发现问题。
- 审计关闭时不要求该契约，不影响现有第三方插件。

## 6. Guard 执行与判定

### 6.1 节点调用

每个节点调用 `{base_url}/v1/chat/completions`：

```json
{
  "model": "sileader/qwen3guard:0.6b",
  "messages": [{"role": "user", "content": "<chunk>"}],
  "temperature": 0,
  "max_tokens": 64,
  "seed": 42
}
```

所有 JSON 编解码使用 `common.Marshal`、`common.Unmarshal` 或 `common.DecodeJson`。HTTP Client 不继承代理、不跟随跨主机重定向、限制响应体 256 KiB、TLS 最低 1.2；BaseURL 只允许 HTTP(S)，禁止 userinfo/query/fragment。节点由 Root 管理员配置，因此允许访问私网和 loopback 以支持自托管 Guard。

### 6.2 分片、并发和故障切换

- 以 Unicode rune 而非字节分片，优先把最新用户段放入首块。
- 节点按配置顺序尝试；429、5xx、网络失败和超时切换下一节点；4xx 配置错误与非法 Guard 响应不盲目重试。
- 全局并发上限默认 64，单节点默认 16；容量耗尽视为 unavailable。
- 一个请求只执行一次完整审计，不随业务渠道重试重复执行。
- 任一分片 Block 立即终止；全部分片结果聚合为最高风险结论。

### 6.3 严格解析与结果语义

必须且只能接受一个 `Safety:` 和一个 `Categories:` 主字段；Safety 只允许 Safe、Controversial、Unsafe。未知或缺失主字段视为非法响应。

- Safe → Pass / Low / Allow。
- Controversial → Flag / Medium / Warn；命中 jailbreak、pii、suicide_and_self_harm 时升级 Block。
- Unsafe → 命中启用分类、出现未知分类或未给出已知分类时 Block；只命中被管理员禁用的已知分类时 Flag / High / Warn。
- Allow 与 Warn 均属于审计通过，可以进入业务上游；Block 不通过。

九类 scanner ID 与参考实现保持一致，避免配置和事件语义漂移。

## 7. 门禁接线

### 7.1 HTTP Relay

在 `controller.Relay` 的 `GenRelayInfo` 成功后调用 `promptaudit.CheckRelayRequest`，位置必须早于敏感词检查、token 估算、价格计算和预扣费。门禁返回 Block/Error 时构造本地 `types.NewAPIError`，加 `SkipRetry`，不得进入业务渠道 retry。

### 7.2 Midjourney

在 `RelayMidjourney` 识别提交类 relay mode 后，通过 `common.UnmarshalBodyReusable` 读取 DTO、构造快照并审计，再进入 `relay.RelayMidjourneySubmit`。查询、通知和资源读取不审计。

### 7.3 Task 与视频

在 `RelayTask` 生成 RelayInfo 后、`ResolveOriginTask` 和 `executeTaskSubmission` 前，从 Gin 上下文的 `task_request` 与 pinned plugin 元数据构造快照。审计失败通过 `respondTaskSubmissionError` 返回本地 TaskError，禁止选择重试和业务提交。

OpenAI Responses 由 Task Plugin 接管时，优先使用标准 Responses 提取器审计原始协议输入；插件规范化后的补充提示词字段再通过 `auditTextPaths` 合并去重，防止 decode hook 引入或移动文本后漏审。

### 7.4 Realtime

Realtime 需要延迟业务上游连接：

1. 本地完成客户端 WebSocket Upgrade；
2. 缓存尚未送上游的 session/control 帧；
3. 读到首个含文本事件后先审计；
4. 通过后才建立业务上游连接并按原顺序发送缓存帧；
5. 连接存续期间，每个新增含文本客户端事件在写 `targetConn` 前逐帧审计。

Block 帧返回 OpenAI Realtime `error` 事件，code 为 `prompt_guard_blocked`，不转发该帧，连接可继续接收下一次输入。Unavailable、Invalid、配置 degraded 或事件落库失败返回对应 error 后关闭连接，避免在基础设施异常期间反复尝试。纯控制/音频帧可在上游连接已建立后直接转发；在首个文本事件之前不得借由缓存帧携带未提取文本。

## 8. 事件模型与持久化

### 8.1 表结构

新增 `PromptAuditEvent` 并加入 `migrateDB()`：

```text
id                     int64 primary key
request_id             varchar(128), index
user_id                int, index
username_snapshot      varchar(255)
user_email_snapshot    varchar(320)
token_id               int, index
token_name_snapshot    varchar(255)
group                  varchar(64), index
channel_id             int
channel_type           int
request_path           varchar(255)
protocol               varchar(64)
model                  varchar(255)
stage                   varchar(32)
audit_scope             varchar(32)
prompt_hash             char/varchar(64), index
redacted_preview        text
full_prompt_ciphertext  跨库 LongText
prompt_length           int
message_count           int
decision               varchar(32), index
risk_level              varchar(32), index
action                  varchar(32)
categories_json         text
matched_scanners_json   text
scanner_scores_json     text
scanner_evidence_json   text
scanner_backend         varchar(64)
scanner_version         varchar(128)
guard_endpoint_id       varchar(128), index
policy_id               varchar(128)
policy_version          int
config_version          int64
chunk_total             int
latency_ms              int64
error_code              varchar(64)
created_at              int64, composite index with id
```

完整提示词不截断；使用自定义 GORM 数据类型在 MySQL 映射 LONGTEXT、PostgreSQL/SQLite 映射 TEXT，避免 MySQL TEXT 的 64 KiB 限制。JSON 字段保存为 TEXT，Scanner/Valuer 只调用 `common.*` 包装器。

不建立外键，身份使用快照并保留数字 ID，避免删除用户或 Token 时丢失历史审计。禁止 GORM 布尔默认 tag；业务默认由配置规范化完成。

### 8.2 原子门禁规则

Guard 得出结果后，在允许业务上游前完成必要事件写入：

- Block 与 Error 始终写事件。
- Pass/Flag 在 `store_pass_events=true` 时写事件。
- 必须写入但数据库失败时返回 503 `prompt_audit_record_failed`，不放行。
- `store_pass_events=false` 时 Pass/Flag 无需写库即可放行。

这保证“页面上应有记录”与“请求已被允许”之间不存在静默缺口。

### 8.3 保留与删除

`retention_days=0` 表示永久。大于 0 时，仅 master 实例启动小时级清理循环，按 `created_at < cutoff` 查询最多 500 个 ID，再按 ID 批量删除，循环设置单次上限，避免长期占锁。多实例仍使用数据库锁/主节点机制防止重复工作。

手动删除支持单条和最多 500 条批量 ID；删除前后记录管理日志，只记录 actor、数量、事件 ID/筛选条件和 request_id，不记录完整提示词。删除为不可恢复操作，前端必须二次确认。

## 9. 错误契约

- 403 `prompt_guard_blocked`：Guard 明确阻断。
- 503 `prompt_guard_unavailable`：全部节点不可用、超时、并发隔离满。
- 503 `prompt_guard_invalid_response`：Guard 响应无法严格解析。
- 503 `prompt_audit_config_degraded`：启用意图存在但配置未激活。
- 503 `prompt_audit_record_failed`：必要事件未能持久化。
- 503 `prompt_audit_unsupported_protocol`：提示词入口缺少确定提取契约。
- 409 `prompt_audit_config_conflict`：管理配置版本冲突。

HTTP Relay 按现有 OpenAI/Claude/Gemini 响应适配返回；Task 使用 TaskError；Realtime 使用 error event。客户端消息不包含命中提示词、节点地址、节点响应原文或内部异常，只附带 request_id。

## 10. 管理 API

统一路由前缀 `/api/prompt-audit`，全部使用 `middleware.RootAuth()`：

- `GET /config`：返回脱敏 PublicConfig、scanner catalog 和 config_version。
- `PUT /config`：校验、加密 Token、CAS 保存并激活。
- `GET /runtime`：有效模式、active/expected version、degraded、加载错误码、启用节点数、未覆盖插件列表。
- `POST /endpoints/probe`：探测待保存或已保存节点，返回延迟、模型和规范化错误；不回显 Token/响应原文。
- `GET /events`：游标或 page/limit 分页，支持时间、用户、Token、分组、模型、协议、decision、risk、category、Guard 节点、request_id、prompt_hash 筛选；只返回脱敏预览。
- `GET /events/:id`：按需解密并返回完整提示词。
- `DELETE /events/:id`：删除单条。
- `POST /events/batch-delete`：按 ID 批量删除，最多 500 条。

配置写入、节点探测、事件删除接入现有管理日志，任何日志内容均不包含 Token、密文或提示词原文。

## 11. 前端设计

### 11.1 菜单与路由

在 `web/src/features/system-settings/security/section-registry.tsx` 注册：

```text
id: prompt-audit
titleKey: Prompt Audit
path: /system-settings/security/prompt-audit
```

section 内容使用独立 `PromptAuditPage`，不依赖通用 `SecuritySettings` Option 数据即可请求专用 API。

### 11.2 页面结构

页面采用响应式 Tabs：

1. 运行状态：启用状态、有效模式、配置版本、节点可用性、degraded 告警、未覆盖插件。
2. 策略配置：总开关、完整/最新轮、保存 Pass、全部/指定分组、九类风险、保留期。
3. 节点池：有序增删改、启停、Token 状态、超时、输入上限、单节点探测。
4. 审计事件：筛选栏、分页表格、风险 Badge、详情 Sheet/Dialog、完整原文复制、单条/批量删除确认。

使用项目现有 Base UI/shadcn 组件，不增加新 UI 依赖。React Hook Form + Zod 做同构前端校验，React Query 管理配置、状态、事件缓存；完整提示词仅由详情 query 返回，关闭详情时清除对应缓存，列表数据绝不包含该字段。

### 11.3 可访问性与国际化

- 表单控件都有可见 Label、错误描述和焦点定位。
- 风险不能只靠颜色表达，Badge 同时显示文本。
- 删除确认可键盘操作，危险按钮使用既有 destructive 样式。
- 长提示词使用可滚动、保留换行的只读区域，并提供明确的复制按钮。
- 新文案覆盖 en、zh、zh-TW、fr、ru、ja、vi；英语原文作为 flat JSON key。

## 12. 可观测性

日志只记录结构化元数据：request_id、event、protocol、stage、config_version、endpoint_id、chunk_total、latency_ms、decision、error_code、upstream_dispatched=false、billing_preconsumed=false。禁止记录 ScanText、FullPrompt、Guard 请求体、Guard 完整响应和节点 Token。

运行指标至少包括：决策计数、按结果延迟、节点故障切换、超时、并发隔离拒绝、事件写入失败、配置 degraded、协议未覆盖。若项目暂无统一指标后端，先复用 perf metrics/结构化日志，不引入新的外部依赖。

## 13. 安全与边界条件

- 审计节点是独立安全上游，不得使用业务渠道重试、计费或自动禁用逻辑。
- 请求取消应取消 Guard HTTP 调用；Realtime 每帧使用有界 context。
- NUL 等数据库不兼容字符在加密前保留，因为密文为 Base64；解密后完整还原。
- 超长提示词按 Unicode 分片全部扫描，不能只扫前 N 字符。
- 明确区分 NoPrompt 与 Unsupported：确实无文本的控制/查询请求跳过；无法证明无文本则失败关闭。
- 分类、分组和节点列表进行排序/去重规范化；配置 Hash/版本不受 map 顺序影响。
- 管理事件详情禁用浏览器和中间代理缓存；API 响应添加 `Cache-Control: no-store`。

## 14. 测试与验收设计

### 后端单元与集成测试

- 配置默认值、边界、CAS 冲突、密钥缺失/变化、PublicConfig 脱敏、degraded fail-closed。
- 九类分类、别名、严格解析、未知分类、Safe/Controversial/Unsafe 判定。
- Unicode 分片、最高风险聚合、有序 failover、429/5xx/超时、全局/单节点 bulkhead。
- 每种协议的精确提取、完整原文留存、latest-turn scan 范围、Base64/URL 排除、Unsupported/NoPrompt。
- HTTP Relay、Midjourney、Task Plugin、Realtime 的 Block/Unavailable/Pass；断言 Block 和 Error 时业务上游调用次数为 0，计费预扣次数为 0。
- 必要事件写入失败时不放行；关闭 Pass 留存时允许放行且不写 Pass。
- RootAuth、列表不返回原文、详情解密、删除审计、分页和筛选。
- 自动保留清理边界、批量大小和重复执行幂等。

新测试使用 `testify/require` 做前置与致命断言，`testify/assert` 做非致命值比较；不添加随机循环、sleep 或只为覆盖率存在的测试。

### 数据库矩阵

SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 均需验证：

1. 空库启动与 AutoMigrate；
2. 从最新发布版代表性数据库升级；
3. 连续启动/迁移两次证明幂等；
4. 超过 64 KiB 的加密完整提示词写入与读取；
5. 索引、分页、筛选、CAS、自动/手动删除；
6. 既有数据和其他表不受影响。

### 前端

- 配置表单默认值、条件校验、CAS 冲突刷新、密钥/degraded 告警。
- 事件列表不出现 full_prompt，详情按需获取并清缓存。
- 删除确认、筛选、分页、节点探测、错误状态。
- `bun run test`、`bun run typecheck`、`bun run lint`、`bun run format:check`、`bun run i18n:sync`、`bun run build`。

### 构建

- 根模块 `go test` 针对受影响包，单次命令限制 60 秒。
- `go test ./...` 或项目等价完整检查分批执行，避免单次后台测试超过 60 秒。
- 因设计不修改 `relaykit/`，无需独立构建；若实施中实际触及 `relaykit/`，必须额外执行 `cd relaykit && GOWORK=off go build ./...`。
