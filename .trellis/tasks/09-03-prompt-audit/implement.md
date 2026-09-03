# 提示词审计实施计划

## 实施边界

本计划实现 `prd.md` 与 `design.md` 的完整范围。实施不得引入异步只审计模式，不得让任何包含提示词的协议在审计开启后绕过门禁，不得修改受保护的项目身份信息。本轮规划完成后必须等待用户审核；未获批准前不执行 `task.py start`，不修改业务代码。

## Trellis 子任务执行边界

以下映射是实际执行时的权威边界；后文阶段用于说明每个子任务内部的实施顺序：

1. `09-03-prompt-audit-core`：领域类型、配置语义、加密能力、Manager、Guard 客户端与判定核心。
2. `09-03-prompt-audit-storage-api`：Option 配置适配、事件模型、加密原文持久化、清理任务、启动接线及 Root 管理 API。
3. `09-03-prompt-audit-http-relay`：标准 HTTP Relay 与 Midjourney 的显式文本提取和同步门禁。
4. `09-03-prompt-audit-realtime`：OpenAI Realtime WebSocket 的首文本延迟建连和逐帧门禁。
5. `09-03-prompt-audit-task-plugin`：视频、通用 Task Plugin、Responses Bridge 及 `auditTextPaths` 契约。
6. `09-03-prompt-audit-web-console`：安全与限制菜单、配置页、事件页及七种语言国际化。
7. `09-03-prompt-audit-integration-check`：跨入口验收、敏感数据检查、构建检查和三数据库矩阵。

`controller/relay.go` 同时可能受 HTTP Relay 与 Task Plugin 子任务影响。实现时先完成 HTTP Relay 的公共 Gate 接线，再由 Task Plugin 子任务基于最新版本接入任务分支；不得相互覆盖改动。

## 依赖关系

```text
prompt-audit-core
  └─> prompt-audit-storage-api
        ├─> prompt-audit-http-relay
        ├─> prompt-audit-realtime
        ├─> prompt-audit-task-plugin
        └─> prompt-audit-web-console

前六个实现子任务 ──> prompt-audit-integration-check
```

`prompt-audit-web-console` 只要求管理 API 契约冻结，可与三个协议接入子任务并行。所有子任务共享 `PromptSnapshot`、`Decision`、错误码、配置版本和事件持久化契约；核心与存储接口冻结前不得编写各入口的自定义替代实现。

## 阶段 1：领域配置、加密与 Guard 核心

### 目标

建立不依赖 Controller、Model 和 Relay 的 `service/promptaudit` 领域核心，完成两态配置语义、稳定密钥校验、节点池、分片、严格解析、故障切换和失败关闭决策。

### 计划文件

- 新增 `service/promptaudit/types.go`
- 新增 `service/promptaudit/config.go`
- 新增 `service/promptaudit/manager.go`
- 新增 `service/promptaudit/crypto.go`
- 新增 `service/promptaudit/guard.go`
- 新增 `service/promptaudit/qwen3guard.go`
- 新增 `service/promptaudit/http_client.go`
- 新增对应 `*_test.go`

### 实施步骤

1. 定义配置、PublicConfig、ActiveConfig、Endpoint、ScannerCatalog、Decision、GuardError 和稳定错误码。
2. 实现默认配置：关闭、全分组、完整输入、保存 Pass、永久保留、九类风险全开、priority 策略。
3. 实现配置规范化与确定性校验；分组和 scanner 去重排序，节点维持管理员优先级顺序。
4. 实现 AES-256-GCM 版本化 envelope、随机 nonce 和用途隔离派生；启用前验证稳定 `CRYPTO_SECRET`/`SESSION_SECRET`。
5. 定义 `ConfigStore`、`EventStore`、`Evaluator` 等注入接口，实现 Manager 的原子 ActiveConfig、expected/active version 和 degraded 语义。
6. 实现 Unicode 分片、全局/单节点 bulkhead、有序 failover、整体超时和最高风险聚合。
7. 实现 OpenAI 兼容 Guard 客户端、响应体上限、禁止代理与重定向、严格 Qwen3Guard 解析。
8. 冻结 `ActiveConfig`、`PromptSnapshot`、`Decision`、Store 和 Evaluator 契约后再启动依赖子任务。

### 验收

- 配置冲突稳定返回 409；密钥不稳定时无法启用。
- 配置损坏、密钥变化、启用节点为空时 runtime degraded，门禁返回 503。
- Safe、Controversial、Unsafe、未知分类的结果与设计一致。
- 节点 429/5xx/超时按顺序切换，4xx 与非法响应不被错误重试。
- 测试证明日志不包含 Token、ScanText 和 Guard 响应正文。

## 阶段 2：配置存储、事件持久化、清理与 Root 管理 API

### 目标

实现领域核心与 new-api 的持久化及管理层适配，建立三数据库兼容的事件表和查询层，并保证必要事件成功写入是放行业务上游的前置条件。

### 计划文件

- 新增 `model/prompt_audit.go`
- 新增 `model/prompt_audit_test.go`
- 修改 `model/option.go`
- 修改 `model/main.go`
- 新增 `service/promptaudit/event.go`
- 新增 `service/promptaudit/cleanup.go`
- 新增对应测试
- 新增 `dto/prompt_audit.go`
- 新增 `controller/prompt_audit.go`
- 新增 `controller/prompt_audit_test.go`
- 修改 `controller/option.go`
- 修改 `router/api-router.go`
- 修改 `main.go`

### 实施步骤

1. 使用 `PromptAuditConfigSecret` Option 实现 `ConfigStore`；在事务内通过 `lockForUpdate(tx)` 完成 `config_version` CAS。
2. 在通用 GetOptions 中显式过滤该键，在通用 UpdateOption 中拒绝该键。
3. 定义 `PromptAuditEvent` 和跨库 LongText 自定义 GORM 类型；MySQL 使用 LONGTEXT，PostgreSQL/SQLite 使用 TEXT。
4. 把事件模型加入 `migrateDB()`；只使用 GORM tag 与方法，不复制数据库方言专用 SQL。
5. 使用 `common.Marshal`/`common.Unmarshal` 编解码 categories、matched scanners、scores、evidence。
6. 实现事件创建、元数据分页、详情读取、单条删除、最多 500 条批量删除和按保留期小批清理。
7. 事件列表 DTO 排除 `full_prompt_ciphertext`；详情 Service 解密后返回 `full_prompt`。
8. 实现 `RecordDecision`：Block/Error 始终落库，Pass/Flag 受开关控制；必要写入失败返回 `prompt_audit_record_failed`。
9. 注册 `/api/prompt-audit` RootAuth 路由组，实现配置、runtime、节点 probe、事件列表/详情和删除接口；详情响应设置 `Cache-Control: no-store`。
10. 限制分页大小、查询时间跨度和批量删除数量；配置与节点响应不得回显 Token 或密文。
11. 在 `main.go` 数据库初始化后启动 Manager，并仅在 master 节点运行小时级自动清理；`retention_days=0` 时不删除。
12. 对未登录、普通用户、Admin 和 Root 验证权限；删除和清理只记录不含原文的管理/结构化日志。

### 验收

- 完整提示词不截断，超过 64 KiB 的原文可写入和解密读取。
- 列表响应和普通 Model JSON 永不暴露密文或完整原文。
- 必要事件写入失败时请求不会放行。
- 保留期边界、分页顺序、筛选和重复清理幂等。
- 配置冲突返回 409，管理 API 错误码和 DTO 契约稳定。
- 只有 Root 能访问配置、运行状态、节点探测及事件接口，只有详情接口返回 `full_prompt`。

## 阶段 3：标准协议提取与 HTTP Relay 门禁

### 目标

覆盖 `controller.Relay` 处理的所有文本协议，在计费与业务上游之前执行一次同步审计。

### 计划文件

- 新增 `service/promptaudit/snapshot.go`
- 新增 `service/promptaudit/extract_relay.go`
- 新增 `service/promptaudit/extract_midjourney.go`
- 新增 `service/promptaudit/extract_relay_test.go`
- 新增 `service/promptaudit/extract_midjourney_test.go`
- 新增 `service/promptaudit/gate.go`
- 修改 `controller/relay.go`
- 新增或扩展 `controller/relay_test.go` 及协议专项测试

### 实施步骤

1. 定义 segment、角色、完整原文拼接、最新轮选择、SHA-256 Hash 和 96-rune 脱敏预览。
2. 对 relaykit 的 OpenAI、Responses、Claude、Gemini、Image、Embedding、Rerank、Alpha Search、Audio DTO 做显式类型分派。
3. 对每种 DTO 只读取明确的文本字段，排除 URL、Data URL、Base64、模型名、回调地址和内部元数据。
4. 无文本的查询/控制请求返回 NoPrompt；未知且可能带文本的格式返回 Unsupported。
5. 对 Midjourney 提交动作读取 prompt/content，对查询、通知、资源操作返回 NoPrompt。
6. 在 `GenRelayInfo` 之后、现有敏感词检查之前调用 Gate。
7. 在 `RelayMidjourney` 调用提交处理前执行 Gate，并返回符合现有 Midjourney 格式的 403/503。
8. Gate 失败构造本地 NewAPIError，设置 SkipRetry；不进入 token 估算、价格计算、预扣费和业务渠道 retry。
9. 事件身份从 TokenAuth/Gin 上下文复制 user、email、token、group、channel 和 request_id 快照。

### 验收

- 每个协议的完整/最新轮输入与预期精确相等。
- 显式 0、false 等原请求字段不因审计提取或复用 Body 而丢失。
- Block、Unavailable、Invalid、RecordFailed 均断言业务上游调用 0 次、预扣费 0 次。
- Midjourney Pass/Block 均有回归测试，Block 时 `relay.RelayMidjourneySubmit` 调用次数为 0。
- 审计关闭和分组未命中时原链路行为不变。

## 阶段 4：OpenAI Realtime 逐帧门禁

### 目标

保证 Realtime 握手后的每段文本在写入业务上游前通过审计，并避免在首个文本通过前连接业务上游。

### 计划文件

- 新增 `service/promptaudit/extract_realtime.go`
- 新增 `service/promptaudit/extract_realtime_test.go`
- 修改 `relay/websocket.go`
- 修改 `relay/channel/openai/relay_realtime.go`
- 扩展 Realtime handler 测试

### 实施步骤

1. 实现 Realtime event 显式提取：instructions、tool description/parameters、item text/transcript、response input。
2. 重构客户端读取和上游连接时序，先缓存控制帧，首个含文本帧通过后建立业务上游连接。
3. 上游连接建立后，每个客户端文本帧在 `helper.WssString(targetConn, ...)` 前执行 Gate。
4. Block 发送 `prompt_guard_blocked` error event，丢弃该帧并保持客户端连接。
5. Unavailable、Invalid、ConfigDegraded、RecordFailed、Unsupported 发送 error event 后关闭连接。
6. 给每帧 Guard 调用设置有界 context，客户端断开时及时取消。
7. 确保并发协程只由单一写入器操作各 WebSocket，避免违反 gorilla/websocket 并发约束。

### 验收

- 首个危险文本帧时业务上游连接次数为 0。
- 后续危险帧不会到达已建立的业务上游；下一次安全输入可继续。
- 纯音频 Base64 不进入文本 Guard；transcript 必须进入。
- 错误 event 不包含原文、节点响应或内部错误。

## 阶段 5：视频与 Task Plugin 全覆盖

### 目标

覆盖视频与通用 Task Plugin 提交入口，并为任意 Task Plugin 建立可验证的提示词路径契约。Midjourney 归入阶段 3 的 HTTP Relay 子任务。

### 计划文件

- 新增 `service/promptaudit/extract_task.go`
- 新增对应测试
- 修改 `controller/relay.go`
- 修改 `pkg/jsplugin/registry.go`
- 修改 `pkg/jsplugin/routing.go`
- 修改 `pkg/jsplugin/registry_test.go`
- 修改 `docs/plugin-api/v1.schema.json`
- 修改 `docs/plugin-api/` 下相关说明
- 修改 `plugins/tasks/*/plugin.js`
- 扩展 `middleware/task_plugin_test.go`、`controller/relay_task_plugin_test.go` 和路由测试

### 实施步骤

1. 在插件 Meta 和 JSON Schema 中增加 `auditTextPaths`，实现受限 JSON Pointer 校验与取值。
2. 所有内置插件声明规范化 `requestBody` 的提示词路径；补齐 prompt、negative prompt、歌词和描述提示词。
3. 在 `RelayTask` 的 `executeTaskSubmission` 前审计 `task_request`；OpenAI Responses 插件桥同时合并标准协议提取结果。
4. 审计开启时，提交类插件缺少有效契约或路径类型不正确则失败关闭；关闭时不改变插件兼容性。
5. runtime 状态计算未覆盖插件列表，并阻止管理员在存在未覆盖的已启用提交插件时无提示地启用全局审计。

### 验收

- 每个内置插件至少有一组 Pass/Block 测试。
- Block 时 `relay.RelayTaskSubmit` 调用次数为 0。
- Base64 文件、URL、回调地址、模型名不被误扫。
- 第三方插件缺少契约时明确 503，不会静默放行。

## 阶段 6：安全与限制下的前端管理页

### 目标

在 `/system-settings/security/prompt-audit` 提供完整、可访问、国际化的配置与事件管理体验。

### 计划文件

- 新增 `web/src/features/prompt-audit/api.ts`
- 新增 `web/src/features/prompt-audit/types.ts`
- 新增 `web/src/features/prompt-audit/schema.ts`
- 新增 `web/src/features/prompt-audit/hooks/`
- 新增 `web/src/features/prompt-audit/components/`
- 新增 `web/src/features/prompt-audit/prompt-audit-page.tsx`
- 新增相邻 `*.test.tsx`
- 修改 `web/src/features/system-settings/security/section-registry.tsx`
- 修改 `web/src/i18n/locales/en.json`
- 修改 `web/src/i18n/locales/zh.json`
- 修改 `web/src/i18n/locales/zh-TW.json`
- 修改 `web/src/i18n/locales/fr.json`
- 修改 `web/src/i18n/locales/ru.json`
- 修改 `web/src/i18n/locales/ja.json`
- 修改 `web/src/i18n/locales/vi.json`

### 实施步骤

1. 定义专用 API 类型和 Zod 表单 schema，复用项目 api client。
2. 用 React Query 实现 config/runtime/events/detail/probe/delete 查询与 mutation；使用精确 query key 失效策略。
3. 注册 `prompt-audit` 安全设置 section。
4. 实现运行状态、策略、节点池、事件四个 Tabs；复用 Card、Tabs、Table、Form、Switch、Select、Badge、Alert、Dialog/Sheet、Pagination、Skeleton、Sonner。
5. 节点 Token 输入保持 write-only；已有 Token 只显示 configured/invalid/missing。
6. 事件列表只展示脱敏预览；点击详情时按需获取完整原文，关闭详情后移除对应 query cache。
7. 删除增加 destructive 二次确认；批量删除最多选择 500 条。
8. 完成七种语言翻译和 i18n 同步，不修改受保护品牌文案。

### 验收

- 菜单位置、URL、刷新与深链接均正确。
- 表单条件校验、配置冲突、degraded、节点探测、分页筛选和删除可操作。
- 完整提示词不会预加载或出现在列表缓存。
- 键盘、焦点、Label、错误提示和非颜色风险表达满足现有 UI 规范。

## 阶段 7：质量检查与三数据库验证

### 后端检查

1. 对受影响包执行 `gofmt`。
2. 按包执行 Go 单元/集成测试，每条后台测试命令控制在 60 秒内。
3. 执行根模块构建和静态检查。
4. 检索并确认新增业务代码未直接调用 `encoding/json` 的 Marshal/Unmarshal。
5. 检索并确认日志语句没有提示词、Token、Guard Body/Response。
6. 检查 `relaykit/` 无改动；若出现改动，执行 `cd relaykit && GOWORK=off go build ./...`。

### 前端检查

在 `web/` 使用 Bun 执行：

```bash
bun run test
bun run typecheck
bun run lint
bun run format:check
bun run i18n:sync
bun run build
```

### 数据库矩阵

分别记录 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 的精确版本和命令，执行：

1. 新数据库迁移两次；
2. 最新发布版代表性数据库升级并重复迁移；
3. 创建超过 64 KiB 的完整提示词事件并解密校验逐字符相等；
4. 配置 CAS 并发更新；
5. 事件分页/筛选；
6. 保留期清理、单条删除和批量删除；
7. 校验索引、已有数据、唯一性和其他业务表未受影响。

任何数据库未完成真实实例验证时，不得宣称功能完成或三库兼容。

## 代码审查清单

- [ ] 所有提示词入口都有显式提取器或明确 NoPrompt 判定。
- [ ] Unsupported 在审计开启时失败关闭。
- [ ] Guard 与事件写入均发生在计费和业务上游之前。
- [ ] Realtime 首个文本通过前未连接业务上游，后续帧逐次审计。
- [ ] Pass/Flag/Block/Error 的持久化规则与开关一致。
- [ ] 完整原文不截断、加密落库、仅 Root 详情接口解密返回。
- [ ] Token、原文、Guard 请求/响应未进入日志、列表或通用 Option。
- [ ] Option CAS、运行时版本和 degraded 状态正确。
- [ ] Task Plugin 内置插件全部有 auditTextPaths，第三方缺失时不绕过。
- [ ] SQLite、MySQL、PostgreSQL 迁移与行为验证完成。
- [ ] 前端七种语言、类型检查、测试、lint、格式和构建通过。

## 回滚策略

代码回滚前先在管理页关闭提示词审计，确保旧版本不会读取新配置。代码回滚保留 `prompt_audit_events` 表与 `PromptAuditConfigSecret` Option，不执行破坏性降级迁移；旧版本忽略这些数据。重新部署新版本后可以继续读取历史事件。若密钥配置错误，恢复原稳定密钥，不重写密文。
