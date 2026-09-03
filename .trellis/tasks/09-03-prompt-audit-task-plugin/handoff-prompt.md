# 09-03-prompt-audit-task-plugin 新上下文执行提示词

你正在仓库 `/Users/laiyanfei/code/python/ai-project/github/new-api` 中继续 Trellis
子任务 `09-03-prompt-audit-task-plugin`。请全程使用中文，把该子任务完整实施、验证并推进到
可交付状态；只处理视频、通用 Task Plugin、声明式插件路由和共享协议桥的提示词审计，
不要顺带实现前端、集成验收或父任务其他交付项。

## 一、先恢复上下文，不能直接编码

1. 完整读取仓库根目录 `AGENTS.md`；若实际修改目录存在更深层 `AGENTS.md`，继续完整读取。
   严格遵守工程质量、中文注释、JSON 包装、测试超时、数据库兼容、`relaykit` 独立性、
   敏感数据保护和项目治理规则。
2. 使用 `trellis-continue` Skill 恢复工作流阶段，并运行：

   ```bash
   python3 ./.trellis/scripts/task.py current --source
   ```

   新上下文没有活动任务，或活动任务仍指向父任务/其他子任务，均不代表需要创建新任务。
   始终以现有目录 `.trellis/tasks/09-03-prompt-audit-task-plugin` 为目标；不要重复创建任务，
   也不要启动父任务 `09-03-prompt-audit`。
3. 按以下顺序完整读取权威材料，不能以本提示词摘要替代原文：
   - `.trellis/tasks/09-03-prompt-audit-task-plugin/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-task-plugin/design.md`
   - `.trellis/tasks/09-03-prompt-audit-task-plugin/implement.md`
   - `.trellis/tasks/09-03-prompt-audit-task-plugin/implement.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-task-plugin/check.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-task-plugin/task.json`
   - `.trellis/tasks/09-03-prompt-audit/prd.md`
   - `.trellis/tasks/09-03-prompt-audit/design.md`
   - `.trellis/tasks/09-03-prompt-audit/implement.md`
   - `.trellis/tasks/09-03-prompt-audit/research/codebase-analysis.md`
   - `.trellis/tasks/09-03-prompt-audit-core/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-core/design.md`
   - `.trellis/tasks/09-03-prompt-audit-core/implement.md`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/design.md`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/implement.md`
   - `.trellis/tasks/09-03-prompt-audit-http-relay/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-http-relay/design.md`
   - `.trellis/tasks/09-03-prompt-audit-http-relay/implement.md`
   - `implement.jsonl` 与 `check.jsonl` 引用的全部规范和研究文件。
4. 检查 `git status --short`、当前 HEAD、相关任务状态和最近提交，保留用户及其他任务已有改动，
   不得覆盖、回退或重写无关修改。本提示词生成时工作树干净，HEAD 为 `0f6c3591`，已有：
   - `f01f90d8`：prompt-audit-core 实现；
   - `4e05a70b`：prompt-audit-storage-api 实现；
   - `0a13f3c4`：prompt-audit-http-relay 实现；
   - `0f6c3591`：prompt-audit-realtime 实现。

   这些只是交接时证据，新上下文必须以实际 HEAD、源码、测试和任务文档为准。
5. 核实强依赖是否真实就绪，不能只看 `task.json.status`。本提示词生成时 storage-api 和
   realtime 标记为 `completed`，core 与 http-relay 仍标记为 `in_progress`，但四者均已有实现
   提交。必须检查 `service/promptaudit/` 已落地的 Snapshot、Gate、Evaluator、Manager、
   EventStore、错误码、事件写入语义及 HTTP Relay 对 `controller/relay.go` 的现有修改，并运行
   必要的定向测试。若核心/存储契约缺失、仍在变化或测试失败，报告具体阻塞并停止；不得在本
   子任务中复制临时实现来绕过依赖。
6. 使用可用的 Serena 做符号级检索；若 Serena 不可用，使用 FastCtx 或 `rg` 降级。编码前至少
   追踪并记录以下真实链路：
   - `pkg/jsplugin/registry.go` 中 `Meta`、`decodeMeta`、`normalizeV1Meta`、`cloneMeta`、注册与
     generation 发布流程；
   - `pkg/jsplugin/routing.go` 中 submit/query/dynamic、`RouteRequestContext.RequestBody`、
     `ProtocolRequestContext`、PinnedRoute/PinnedEndpoint 和不可变 generation；
   - `middleware.PrepareTaskPluginSubmit`、`PrepareTaskPluginRoute`、
     `PrepareTaskPluginEndpoint` 如何产生规范化 `task_request`；
   - `controller.RelayTask` → `ResolveOriginTask` → `executeTaskSubmission` → 业务渠道重试、
     预扣/结算与任务持久化的顺序；
   - `controller.RelayTaskPluginEndpoint` → `serveTaskPluginProtocol` → `deps.submit` 的
     Responses Bridge 独立路径；
   - OpenAI Video 的 JSON/multipart 路径、十个内置插件的 decode 输出形状和全部提示词字段；
   - `/api/prompt-audit/runtime` 当前响应构造，以及插件 registry/runtime status 的现有接口。
7. 特别注意：当前 Responses Bridge 在 `serveTaskPluginProtocol` 中直接生成 RelayInfo、解析
   OriginTask 并调用 `deps.submit`，不会经过 `RelayTask`。只在 `RelayTask` 加 Gate 会留下可绕过
   审计的 `/v1/responses` 入口。必须以当前调用图为准同时覆盖这条路径，并保证一个逻辑请求只
   审计一次。
8. 规划门禁不可跳过。该子任务当前仍是 `planning`。如果没有证据表明用户已在最新规划总结之后
   明确批准实施，先按 `trellis-brainstorm` 输出最终规划总结，必须包含 Goal、In Scope、
   Out of Scope、Acceptance Criteria、Key Decisions、Risks/Deferred Items 和 artifact status，
   然后停止并等待明确批准。不要把本交接提示词或用户最初要求视为实施批准。
9. 用户随后明确批准最新规划总结后，只启动本子任务：

   ```bash
   python3 ./.trellis/scripts/task.py start 09-03-prompt-audit-task-plugin
   ```

   进入 `in_progress` 后按当前 Trellis 工作流执行。主会话默认派发 `trellis-implement`，实现完成
   后派发 `trellis-check`；每个派发提示词第一行必须是：

   ```text
   Active task: .trellis/tasks/09-03-prompt-audit-task-plugin
   ```

   明确告诉实现/检查代理：它不是代码库中唯一的工作者，不得回退或覆盖他人改动；它已经是对应
   Trellis 代理，不得递归派发 `trellis-implement` 或 `trellis-check`。若当前环境明确采用 inline
   流程，则按 breadcrumb 使用 `trellis-before-dev` 后由主会话实施，不要混用两种流程。

## 二、任务目标

为视频、通用 Task Plugin、声明式插件 submit/dynamic 路由和被插件接管的共享协议端点建立
可验证的提示词字段契约与同步失败关闭门禁。

最终必须保证：

- 插件 v1 Meta 通过 `auditTextPaths` 明确声明 decode 后规范化 `requestBody` 中会发送给业务
  上游的文本字段；路径不能演变成“扫描任意 JSON 字符串”。
- 十个内置插件的 prompt、negative prompt、歌词、描述提示词及其他实际会上游的文本均被覆盖，
  模型名、URL、回调地址、凭据、Base64 和文件内容不得误入 Guard。
- 审计开启且命中分组时，submit/dynamic 请求必须先通过 Gate 和必要事件写入，之后才可解析
  OriginTask、选择/调用业务渠道、预扣费或持久化任务。
- 第三方插件缺少有效契约或声明路径与实际值类型不符时返回稳定 503，不能静默放行；审计关闭时
  旧第三方插件仍保持原有行为。
- 被插件接管的 OpenAI Responses 同时审计原始标准协议输入和 decode 后新增/移动的补充文本，
  以稳定顺序合并并去重，不重复送审同一文本。
- Runtime 能向 Root 管理员列出当前启用但缺少有效审计覆盖的提交插件，且不泄漏插件源码、
  请求内容或其他敏感信息。

## 三、准确范围

### 3.1 必须实现

1. 扩展插件 v1 Meta，增加可选的 `auditTextPaths []string` / `auditTextPaths` JSON 字段，并同步：
   - Go Meta 类型、严格 decoder 的允许字段、clone/不可变 generation 复制；
   - `normalizeV1Meta` / `ValidateV1Meta` 的确定性校验；
   - `docs/plugin-api/v1.schema.json`、`v1.d.ts`、`v1.md` 及必要 README；
   - registry、上传插件、factory/override/hot reload 和 generation 测试。
2. 路径采用受限 JSON Pointer 语义，作用对象只能是 decode 后的规范化 `requestBody`。具体边界以
   父设计和仓库事实落地，但至少满足：
   - 只允许绝对、确定路径；正确处理 JSON Pointer 转义；
   - 禁止 `*`、递归下降、过滤表达式和“遍历所有对象值”；
   - 对路径数量、单路径长度/深度、数组索引和最终提取总量设置与现有请求体限制一致的明确边界；
   - 重复路径规范化或拒绝，行为必须确定；
   - 最终值只接受已批准的字符串、字符串数组或确定的 text 内容块结构；对象、数字、布尔、
     URL/File/Base64 容器及错误类型不能被宽松字符串化；
   - 缺失路径、空字符串、空数组、数组越界和类型错误分别有确定、可测试的 NoPrompt/
     Unsupported 行为。审计开启时不能把契约错误当作 NoPrompt 放行。
3. 实现 Task Plugin 专用提取器，输入至少包含 pinned plugin 元数据、规范化 `task_request`、
   协议/路由阶段及现有 RelayInfo/Gin 身份快照；复用已落地的 `PromptSegment`、
   `BuildPromptSnapshot`、Manager、Evaluator、EventStore 和统一 Gate，不在 middleware/controller
   中重新实现 Guard 判定或事件存储。
4. 逐个审计十个内置插件的真实 decode 输出并补齐路径：`alibaba`、`doubao`、`google`、
   `hailuo`、`jimeng`、`kling`、`sora`、`sunoapi`、`vertex-ai`、`vidu`。不能只假设所有插件都
   使用 `/prompt`；必须覆盖实际的 negative prompt、lyrics、`gpt_description_prompt` 和其他
   确实会上送的文本字段，并明确排除图片/视频 URL、Data URL、Base64、上传文件、callback、
   model、凭据和业务控制字段。
5. 覆盖全部提交入口：
   - `/v1/tasks/:key` legacy submit；
   - 声明式 `submit` route；
   - 声明式 `dynamic` route 解析后实际为 submit 的分支；
   - OpenAI Video 的 JSON 与 multipart create；
   - 被插件接管的 OpenAI Responses create（stream/sync/background）。

   query、retrieve、content、任务状态读取及二进制结果代理属于确定的 NoPrompt，不创建审计事件，
   也不得因缺少 `auditTextPaths` 被误阻断。
6. 在普通 Task 流程中，将 Gate 放在 `RelayTask` 完成 `GenRelayInfo` 和必要 action 快照之后、
   `ResolveOriginTask`、`ApplyOriginTaskAffinity`、`executeTaskSubmission`、渠道重试、计费和业务
   submit 之前。审计失败必须通过现有 TaskError/`respondTaskSubmissionError` 安全返回。
7. 在 Responses Bridge 中单独覆盖 `serveTaskPluginProtocol` 的独立提交路径：在
   `ResolveOriginTask` 和 `deps.submit` 之前完成一次合并审计；错误映射为协议兼容的 OpenAI
   Responses 错误。不得因为桥接路径最终复用 `executeTaskSubmission` 就在普通路径重复审计。
8. Responses Bridge 的合并规则：
   - 从 `ProtocolRequestContext` 保存的原始 OpenAI Responses body 复用标准 Responses 提取语义；
   - 从 decode 后 `task_request` 按 `auditTextPaths` 提取插件补充文本；
   - 保留确定顺序、角色/来源和 stage；仅对语义相同的重复片段稳定去重；
   - 原始输入即使被 decode 移动或删除也不能漏审，decode 新增的上游提示文本也不能漏审；
   - `latest_turn_only` 只改变实际 `ScanText`，事件 `FullPrompt` 仍保存本次定义范围内的完整文本。
9. 审计开启且命中范围时：
   - 没有 `auditTextPaths` 的提交插件返回 503 `prompt_audit_unsupported_protocol`；
   - 路径声明非法应在插件注册/更新时拒绝；
   - 路径在当前规范化请求上出现不允许的类型或无法证明安全覆盖时失败关闭；
   - Block 返回 403 `prompt_guard_blocked`；Guard/配置/事件等基础设施错误按统一稳定 503 映射；
   - 所有本地审计错误设置为不可进行业务渠道 retry，不能触发渠道禁用。
10. 为 Root Prompt Audit runtime 响应增加稳定排序、去重的未覆盖启用提交插件列表。优先在现有
    controller/DTO/registry 适配边界组合数据，避免让领域核心反向依赖 `pkg/jsplugin` 或形成包循环。
    列表按当前已发布且可路由的 generation 计算，不把已禁用、被拒绝发布、纯查询插件误报为未覆盖。
    不擅自改变“允许保存启用配置”的产品语义；父设计要求的是启用前可见和请求期失败关闭。
11. 插件 generation 是请求级 pin：审计提取必须使用本请求已 pinned 的同一 `LoadedPlugin.Meta`，
    不能在请求中途重新从 DefaultRegistry 读取新 generation，避免热更新导致 decode 使用旧契约、
    Gate 使用新契约。

### 3.2 明确不包含

- 不实现标准非插件 HTTP Relay、Midjourney 或 OpenAI Realtime WebSocket 门禁。
- 不实现 Prompt Audit 配置/事件表、Guard 核心、加密、保留清理或前端页面。
- 不审核任务上游生成结果，不审核图片、音频、视频或上传文件的二进制内容。
- 不增加异步只记录后放行、队列、Worker、Redis 原文载荷或补偿审计。
- 不通过递归遍历任意 JSON 字符串“提高覆盖率”。
- 不修改数据库模型、迁移或查询；本子任务正常不需要三数据库矩阵。
- 不修改 `relaykit/`，也不得让 `relaykit/` 依赖根模块；标准 Responses 提取应从根模块现有
  `service/promptaudit` 能力复用。
- 不做与本子任务无关的插件系统重构、依赖升级、全仓格式化或品牌修改。
- 不修改、删除或替换任何受保护的 new-api / QuantumNous 项目身份、品牌、元数据或归属信息。

## 四、必须保持的行为与安全不变量

1. 审计模式只有关闭和同步阻断；不得出现“先提交任务，再异步补审计”的路径。
2. 审计关闭或分组未命中时，不要求第三方插件声明 `auditTextPaths`，原 legacy、native route、
   OpenAI Video 和 Responses Bridge 行为不变。
3. 审计命中时，每一条业务提交路径必须满足：

   ```text
   规范化请求 + pinned 契约
     -> 明确提取 / Unsupported
     -> Prompt Audit Gate
     -> 必要事件成功写入
     -> ResolveOriginTask / 渠道选择与重试 / 计费
     -> 业务上游 submit
     -> 任务持久化与结算
   ```

4. Block、Unavailable、Invalid、ConfigDegraded、RecordFailed、Unsupported 时，直接观察到的
   业务上游调用次数、task submit 次数和预扣费次数均必须为 0；不能只断言内部 Decision 或状态码。
5. 一个逻辑请求只执行一次完整审计，不能随渠道 retry、Responses 观察循环或 presenter 重复审计。
6. 只有实际 submit/dynamic-submit 请求需要契约。query/retrieve/content 等无新增文本的已知操作
   不审计、不写事件、不因第三方插件缺少契约失败。
7. 完整提示词不截断；超长文本交给核心按 Unicode 字符分片全部扫描。不得用路径数量或输入上限
   只扫描前 N 个字符；边界超限必须以稳定错误失败关闭。
8. 完整提示词、`ScanText`、Guard 请求体/原始响应、节点 Token/地址、插件凭据、Base64、文件内容
   和内部异常不得进入日志、错误响应、指标标签、runtime 状态或测试失败输出。客户端只得到稳定
   错误码、安全文案和 request_id。
9. 审计读取不能消费、重排或改写原请求/规范化请求。optional scalar 的显式 `0`、`0.0`、`false`
   以及 multipart 文本/文件边界不得因审计而变化。
10. 新增 Go 业务代码的 JSON 编解码必须使用 `common.Marshal`、`common.Unmarshal`、
    `common.UnmarshalJsonStr` 或 `common.DecodeJson`；不得直接调用 `encoding/json` 的 Marshal/
    Unmarshal。允许使用 `json.RawMessage` 等类型。
11. 遵守 Router → Controller → Service → Model 分层。插件契约解析放在稳定的 jsplugin/
    promptaudit 边界，Controller 只负责接线和协议错误适配，不能自行解释 Qwen3Guard 响应。
12. `controller/relay.go` 是共享热点。修改前后检查实际 diff，保留 HTTP Relay、Realtime 及其他
    并行改动；禁止用旧版本整体覆盖文件。

## 五、实施要求

具体文件和符号以当前源码及冻结契约为准，预计至少涉及：

- `pkg/jsplugin/registry.go`、必要的 `routing.go` 及相邻测试；
- `docs/plugin-api/v1.schema.json`、`v1.d.ts`、`v1.md` 和必要 README；
- `plugins/tasks/*/plugin.js` 十个内置插件；
- 新增 `service/promptaudit/extract_task.go` 及测试，必要时增加 Task/Plugin 专用 Gate 适配；
- `middleware/task_plugin.go` 的请求上下文边界测试，但避免把领域 Gate 塞进解析 middleware；
- `controller/relay.go`、`controller/plugin_protocol.go` 及相邻 Task/协议桥测试；
- `/api/prompt-audit/runtime` 的 controller/DTO/registry 组合边界和测试。

实现前先形成一份字段覆盖清单：每个内置插件、每条 submit/dynamic/protocol 路径、decode 前字段、
规范化后字段、`auditTextPaths`、明确排除字段、对应测试。该清单应写入子任务设计/实施记录，不能
只存在于临时分析中。

代码保持直接、可读、低嵌套，优先早返回和清晰分支；不要增加只有一个调用点且没有稳定业务含义
的包级 helper。必要注释使用中文，只解释插件契约、失败关闭、generation pin、桥接去重和敏感
数据边界，不复述代码。

## 六、测试与质量门禁

新增或大幅重写的 Go 测试使用 `github.com/stretchr/testify/require` 处理前置和致命断言，使用
`github.com/stretchr/testify/assert` 处理非致命比较。采用确定性表驱动测试、Gin/httptest fake、
内存 Store 和可观察 submit/billing 计数器；禁止随机输入、大循环、真实 sleep、时间性能比较、
日志式断言或只为覆盖率存在的测试。

至少覆盖：

1. Meta decoder/schema/types/docs 一致；合法受限 JSON Pointer、转义、重复、数量/长度/深度/
   数组索引边界，以及 wildcard/递归/相对路径/畸形转义的拒绝。
2. registry 的 factory、管理员上传/override、hot reload 和 generation clone 均保留同一契约；
   请求使用 pinned generation，不受并发发布的新版本影响。
3. 字符串、字符串数组、已知 text 内容块、Unicode、空值、缺失路径、数组越界和错误类型的精确
   提取结果；证明对象不会被递归扫、非字符串不会被宽松转换。
4. 十个内置插件逐个验证所有实际提示字段被覆盖，并验证 URL、Data URL、Base64、callback、
   model、凭据、文件 metadata/bytes 不被提取。每个内置插件至少有一条 Pass 和一条 Block 路径。
5. legacy `/v1/tasks/:key`、native submit、dynamic→submit、OpenAI Video JSON、OpenAI Video
   multipart、Responses stream/sync/background 均有入口级 Gate 测试。
6. query、retrieve、content、dynamic→query 等已知 NoPrompt 路径不审计、不写事件、不中断原流程。
7. 第三方插件：无契约时审计关闭可用、开启且命中时 503；非法契约在注册时拒绝；当前请求路径
   类型错误时失败关闭。
8. Responses Bridge 原始标准输入与规范化补充文本都被提取，顺序和去重稳定；decode 删除、移动、
   复制或新增文本时不漏审、不重复送审。
9. Pass/Flag 时 Gate 只执行一次后进入原业务链；业务渠道 retry、Responses 观察和 presenter 不重复
   审计。`store_pass_events` 的事件行为复用核心语义。
10. Block、Unavailable、Invalid、ConfigDegraded、RecordFailed、Unsupported 在所有提交入口均断言：
    业务上游/submit 0 次、预扣费 0 次、OriginTask/后续副作用按批准时序未发生；错误码和协议格式稳定。
11. Runtime 未覆盖列表只包含当前已发布、启用、具有提交能力且缺少有效契约的插件，结果稳定排序、
    去重，并覆盖 disabled/rejected/query-only/hot-reload 边界。
12. 请求和日志敏感信息防泄漏使用 canary 值断言，不把真实提示词或密钥打印到测试输出。

每条后台测试命令自身设置不超过 60 秒，并按受影响包或 `-run` 测试集拆分。根据实际文件调整，
至少执行并记录与以下等价的检查：

```bash
gofmt -w <本子任务实际修改的 Go 文件>
go test -count=1 -timeout 55s ./pkg/jsplugin
go test -count=1 -timeout 55s ./service/promptaudit
go test -count=1 -timeout 55s ./middleware
go test -count=1 -timeout 55s ./controller
go vet ./pkg/jsplugin ./service/promptaudit ./middleware ./controller
```

同时运行仓库已有的插件 schema/fixture/lint/format 校验；先检查 `package.json`、`web/package.json`、
脚本和 CI 后再选用真实命令，不要臆造脚本，也不要擅自更换包管理器。检查新增业务代码是否绕过
`common.*` JSON 包装，并检索日志/响应/runtime 路径确认敏感字段不泄漏。

本子任务正常不涉及数据库行为，因此不要自行声称完成三数据库验证；若实际修改影响模型、迁移、
查询、事务或驱动行为，必须按根 `AGENTS.md` 完成 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 的真实
实例矩阵，未完成时列为阻塞。本任务原则上不修改 `relaykit/`；若经重新审批后实际触及，必须执行：

```bash
cd relaykit && GOWORK=off go build ./...
```

## 七、检查、文档与交付

1. 实现完成后使用 `trellis-check` 对本子任务全范围检查并直接修复发现的问题，不能只看最后一批
   diff。检查必须覆盖子任务全部验收项、父任务失败关闭/安全约束、核心与存储契约，以及：
   - client → middleware decode → pinned audit contract → Gate → EventStore → upstream；
   - legacy/native/dynamic/OpenAI Video/Responses Bridge 的全部分支；
   - 错误流、计费流、渠道 retry、任务持久化流和 runtime 覆盖报告；
   - 敏感数据泄漏、漏扫、误扫、重复审计和 generation 竞态。
2. 检查后运行 `trellis-update-spec`，判断并记录值得长期沉淀的插件审计契约、generation pin、
   Responses Bridge 双源提取或失败关闭经验；没有可沉淀内容时也明确记录判断，不能伪造更新。
3. 更新 task-plugin 的 `prd.md`、`design.md`、`implement.md`，记录最终路径语法、内置插件覆盖矩阵、
   实际 Gate 位置、Responses 合并/去重规则、runtime 契约、验证命令和结果。若实际实现改变公共契约
   或发现父/子设计冲突，先同步受影响父子任务规划并重新请求用户审批。
4. 按根 `AGENTS.md` 把关键约束、最佳实践和经验写入 Serena memory；Serena 不可用时明确说明
   降级，不能声称已经写入。
5. 检查 `git diff` 和 `git status`，确认没有敏感数据、无关改动、占位符、TODO、旧兼容分支或
   被覆盖的并行工作。
6. 不自动执行 `git commit` 或 `git push`。按 Trellis 工作流先向用户展示分批提交计划、每批文件
   和所有未识别脏文件，等待明确确认后才能提交；禁止 push。提交后再按 `trellis-finish-work`
   完成收尾提醒。

只有以下条件全部满足，才可声明 task-plugin 子任务完成：

- 子任务 PRD 每项验收标准都有实现和直接测试证据；
- 十个内置插件的实际文本字段已形成审计矩阵，schema/types/docs/插件声明保持同步；
- legacy、native、dynamic、OpenAI Video 与 Responses Bridge 均不存在审计绕过；
- Block/失败关闭时上游、submit 和预扣均为 0，Pass 路径不重复审计；
- 第三方缺失/错误契约和 runtime 未覆盖报告均已验证；
- 定向测试、静态检查、插件校验和敏感信息检查全部通过；
- 没有越过子任务范围或覆盖其他任务改动；
- Trellis 全范围质量检查已通过。

最终交付用中文简洁报告：已覆盖的插件/入口矩阵、最终 `auditTextPaths` 契约、Gate 与 Responses
合并位置、关键安全/兼容决策、可点击的绝对文件路径及关键行号、执行过的精确命令和结果、
未执行/失败验证及影响、是否触及 `relaykit` 或数据库行为，以及是否已满足父任务
integration-check 的启动条件。任何必要验证未执行、依赖未冻结或验收项缺失时，必须明确列为
阻塞，不能宣称“全部完成”。
