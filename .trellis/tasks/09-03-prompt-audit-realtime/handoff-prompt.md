# 09-03-prompt-audit-realtime 新上下文执行提示词

你正在仓库 `/Users/laiyanfei/code/python/ai-project/github/new-api` 中继续 Trellis
子任务 `09-03-prompt-audit-realtime`。请全程使用中文，将该子任务完整实施、验证并推进到
可交付状态；只处理 OpenAI Realtime WebSocket 的提示词审计，不要顺带实现 Task Plugin、
前端或父任务的其他交付项。

## 一、先恢复上下文，不能直接编码

1. 完整读取仓库根目录 `AGENTS.md`；若实际修改目录存在更深层 `AGENTS.md`，继续完整读取。
   严格遵守工程质量、中文注释、JSON 包装、测试超时、`relaykit` 独立性、敏感数据保护和
   项目治理规则。
2. 使用 `trellis-continue` Skill 恢复工作流阶段，并运行：

   ```bash
   python3 ./.trellis/scripts/task.py current --source
   ```

   新上下文没有活动任务，或活动任务仍指向父任务，均不代表需要创建新任务。始终以现有目录
   `.trellis/tasks/09-03-prompt-audit-realtime` 为目标，不要重复创建任务，也不要启动父任务。
3. 完整读取以下权威材料，不能用本提示词的摘要替代原文：
   - `.trellis/tasks/09-03-prompt-audit-realtime/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-realtime/design.md`
   - `.trellis/tasks/09-03-prompt-audit-realtime/implement.md`
   - `.trellis/tasks/09-03-prompt-audit-realtime/implement.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-realtime/check.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-realtime/task.json`
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
   - `implement.jsonl` 和 `check.jsonl` 引用的全部规范、研究文件。
4. 检查 `git status --short`、当前 HEAD 和相关任务的最近提交，保留用户及其他任务已有改动，
   不得覆盖、回退或重写无关修改。本提示词生成时工作树干净，HEAD 为 `0a13f3c4`；已存在：
   - `f01f90d8`：prompt-audit-core 实现；
   - `4e05a70b`：prompt-audit-storage-api 实现；
   - `0a13f3c4`：prompt-audit-http-relay 实现。

   这些只是交接时证据，新上下文必须以实际 HEAD、源码、测试和任务文档为准。
5. 核实强依赖是否真实就绪，不能只看 `task.json.status`：本提示词生成时 storage-api 标记为
   `completed`，core 仍标记为 `in_progress`，但 core 已有实现提交。必须检查实际
   `service/promptaudit/` 公共契约、相关验收记录和定向测试。若 Snapshot、Evaluator、Manager、
   EventStore、稳定错误码或事件写入语义缺失/仍在变化，报告具体阻塞并停止；不得在 Realtime
   子任务里复制一套临时核心实现绕过依赖。
6. 使用可用的 Serena 做符号级检索；若 Serena 不可用，使用 FastCtx 或 `rg` 降级。编码前至少
   追踪以下真实调用链和符号：
   - `controller.Relay` 的客户端 WebSocket Upgrade、审计旁路、计费与 `relay.WssHelper` 调用顺序；
   - `relay.WssHelper` 的 adaptor 初始化、`DoRequest` 上游拨号和 `DoResponse` 生命周期；
   - `relay/channel.DoWssRequest` 的目标 URL、Header、拨号及错误语义；
   - `relay/channel/openai.OpenaiRealtimeHandler` 的两个读协程、直接 WebSocket 写入、token/usage
     统计和结算；
   - `relay/helper.WssString`、`WssObject`、`WssError` 的写入行为；
   - `relaykit/dto.RealtimeEvent` 的当前字段覆盖；
   - `service/promptaudit` 已落地的 `PromptSegment`、`BuildPromptSnapshot`、Manager、Evaluator、
     EventStore、全局访问器和 HTTP Gate 模式。
7. 规划门禁不可跳过。Realtime 子任务当前仍为 `planning`。如果没有证据表明用户已经在最新
   规划总结之后明确批准实施，先按 `trellis-brainstorm` 输出最终规划总结，必须包含 Goal、
   In Scope、Out of Scope、Acceptance Criteria、Key Decisions、Risks/Deferred Items 和
   artifact status，然后停止并等待用户明确批准。不要把本交接提示词或用户最初要求视为实施批准。
8. 用户随后明确批准最新规划总结后，只启动本子任务：

   ```bash
   python3 ./.trellis/scripts/task.py start 09-03-prompt-audit-realtime
   ```

   进入 `in_progress` 后按当前 Trellis 工作流执行。主会话默认派发 `trellis-implement`，实现完成
   后派发 `trellis-check`；每个派发提示词第一行必须是：

   ```text
   Active task: .trellis/tasks/09-03-prompt-audit-realtime
   ```

   明确告诉实现/检查代理：它不是代码库中唯一的工作者，不得回退或覆盖他人改动；它已经是对应
   Trellis 代理，不得递归派发 `trellis-implement` 或 `trellis-check`。若当前运行环境明确采用
   inline 流程，则按 breadcrumb 使用 `trellis-before-dev` 后由主会话实施，不要混用两种模式。

## 二、任务目标

为 OpenAI Realtime WebSocket 建立真正的同步失败关闭门禁：审计开启且命中用户分组时，在
首个可能发送给业务上游的文本事件通过审计前不得建立业务上游 WebSocket；连接建立后，每个
新增客户端文本事件必须在写入上游前逐帧审计。

最终必须保证：

- 首个危险文本事件被阻断时，业务上游拨号次数和收到的帧数均为 0；
- 已建立连接后的危险帧不会到达上游，Block 只丢弃该帧，客户端连接仍可接收后续安全输入；
- Guard、配置、协议或必要事件持久化异常时，返回安全的 Realtime error event 后关闭连接；
- 审计关闭或分组未命中时，现有 Realtime 行为、计费和协议兼容性保持不变；
- 所有提示词、Guard 原始响应、节点 Token/地址和内部异常均不进入日志或客户端错误。

## 三、准确范围

### 3.1 必须实现

1. 在 `service/promptaudit/` 中新增或补齐 Realtime 专用的确定性事件提取与 Gate 边界，复用已
   冻结的 `PromptSegment`、`BuildPromptSnapshot`、Manager、Evaluator、EventStore 和错误码。
2. 显式覆盖父任务批准的客户端输入：
   - `session.update` 中的 `session.instructions`；
   - 工具定义中明确会发送上游的描述和参数模式文本；
   - `conversation.item.create` 中的 item text、input text、transcript、工具结果等明确文本；
   - `response.create` 中协议实际支持且会发送上游的 instructions、输入文本和工具定义文本；
   - 当前 Realtime 协议版本中其他已知、确实由客户端控制并会发送业务上游的文本字段。
3. 只按已知事件和已知字段提取。禁止对任意 JSON 做“递归收集所有字符串”；工具参数若是 JSON
   Schema，只能按明确、可测试的 schema 文本契约提取，不得把字段名、模型名、控制值、URL、
   Base64 或无关元数据一并扫描。
4. 明确区分：
   - `NoPrompt`：已知纯控制事件、纯音频 Base64 帧或已知结构中本次没有新文本；
   - `Unsupported`：未知且可能携带上游文本的事件、结构非法、字段类型不符合契约，或无法证明
     没有文本。审计开启且命中范围时必须失败关闭。
5. 重构 Realtime 上游连接时序：
   - 下游客户端 WebSocket 可先完成 Upgrade；
   - 审计启用且命中分组时进入 `awaiting_first_text`；
   - 在该状态缓存已完成明确 NoPrompt 判定的控制/音频帧，并限制帧数量、单帧/总字节；
   - 首个文本事件先完成 Gate；通过后才拨号业务上游，并按原始顺序发送此前缓存帧和当前已通过帧；
   - 首帧 Block 时不拨号、不转发该帧，仍等待后续事件；后续安全文本可触发一次上游连接；
   - 配置关闭或分组未命中时不得无故等待首文本，保持现有即时连接行为；
   - degraded 或其他已确定的失败关闭状态必须在拨号前终止，不能退化为关闭后放行。
6. 上游连接建立后，每个客户端事件都必须先完成“解析 → 分类 → 必要审计 → 必要事件写入”，
   之后才允许 token/usage 统计和写入 `targetConn`。危险或失败帧不得被计入可计费输入，也不得触发
   与该帧相关的预扣、结算、渠道重试或渠道禁用。
7. 逐帧 Gate 必须遵守核心持久化语义：Block/Error 必要事件先成功写入；Pass/Flag 是否写入由
   `store_pass_events` 控制；必要写入失败返回 `prompt_audit_record_failed` 并关闭连接。不要在
   Realtime handler 里自行解析 Qwen3Guard 结果或重新定义判定规则。
8. 每个事件建立稳定 `PromptSnapshot`：身份、分组、请求路径、模型和渠道信息来自现有 Gin/
   RelayInfo；`Protocol` 明确为 Realtime；`Stage` 能区分事件类型/阶段；`FullPrompt` 保存该次事件
   依契约提取的完整原文，`ScanText` 遵守 `latest_turn_only` 的既有语义，不得截断完整原文。
9. 按帧使用有界 context；客户端断开、上游断开或基础设施错误必须取消正在进行的 Guard 请求，
   并有序回收 reader、writer、连接、channel 和 goroutine。
10. 对客户端连接和业务上游连接分别保证 gorilla/websocket 的单写入器约束。本地 error event、
    上游响应转发、close frame 不能从多个协程并发写同一连接。使用清晰的 writer ownership、
    `context.CancelFunc`、`sync.Once`/等价关闭协议，避免重复 close、send-on-closed-channel 和
    goroutine 泄漏。
11. 保持业务上游响应透传、Realtime session 状态更新、token 计数、usage 累计和既有结算语义。
    审计只改变客户端输入到达业务上游前的顺序和失败行为；不得审计或阻断业务上游输出内容。
12. 若首文本前缓冲达到上限，返回父设计允许的稳定失败关闭错误，发送不含内部细节的 error event
    后关闭连接；不得继续缓存、拨号上游或静默丢帧。边界值应依据仓库已有请求/消息限制确定并记录。
13. Block 使用 `prompt_guard_blocked` 的 OpenAI Realtime `error` 事件，只丢弃当前帧并保持连接。
    `prompt_guard_unavailable`、`prompt_guard_invalid_response`、
    `prompt_audit_config_degraded`、`prompt_audit_record_failed`、
    `prompt_audit_unsupported_protocol` 等基础设施/契约错误发送对应安全 error event 后关闭连接。
14. 审查 `controller.Relay` 现有 Realtime 计费时序。父任务要求失败关闭时业务上游调用和预扣费
    次数均为 0；如果现有通用预扣在首帧审计前发生，必须做范围受控的 Realtime 特化，使首次
    Gate 失败不产生预扣，且不能破坏正常 Realtime 的余额校验、usage 结算或退款语义。不得只验证
    “上游没收到帧”而忽略计费侧效果。

### 3.2 明确不包含

- 不实现标准 HTTP Relay、Midjourney、视频、通用 Task Plugin 或 Responses Bridge 的门禁。
- 不实现管理 API、配置存储、事件表、清理任务或前端页面。
- 不审核业务上游生成的响应，不审核音频二进制内容；客户端提供的 transcript 仍属于文本，必须审计。
- 不新增 async/只记录后放行模式、队列、Worker、Redis 原文载荷或后台补偿审计。
- 不复制 Guard 核心、配置 Manager、AES 加密、EventStore 或 HTTP Relay Gate。
- 不做与 Realtime 审计无关的 WebSocket 全面重构、依赖升级或格式化。
- 不修改任何受保护的 new-api / QuantumNous 项目身份、品牌、元数据或归属信息。
- 父设计要求 `relaykit/` 保持不变。优先在根模块使用 Realtime 专用提取结构安全解析原始事件；不得
  为方便而把审计依赖下沉到 `relaykit/`。若实际协议契约确实无法在不修改 `relaykit/` 的情况下
  表达，先退回规划说明原因、最小变更和下游影响，并重新请求审批。任何已批准的 `relaykit/` 改动
  都必须保持模块独立，并执行 `cd relaykit && GOWORK=off go build ./...`。

## 四、必须保持的状态机与行为不变量

按已批准设计实现或等价表达以下状态：

```text
client_connected
  ├─ 审计关闭/分组未命中 ──> upstream_connected ──> streaming
  └─ 审计命中 ──> awaiting_first_text
                     ├─ NoPrompt ──> 有界缓存，继续等待
                     ├─ Block ──> 返回 error，丢帧，继续等待
                     ├─ Pass/Flag ──> upstream_connected ──> 按序刷新缓存 ──> streaming
                     └─ Error/Unsupported/缓冲超限 ──> 返回 error ──> closing

streaming
  ├─ NoPrompt ──> 转发
  ├─ Pass/Flag ──> 必要事件写入成功后转发
  ├─ Block ──> 返回 error，仅丢弃该帧，继续 streaming
  └─ Error/Unsupported ──> 返回 error ──> closing
```

以下不变量必须由测试直接观察，而不是只检查内部 Decision：

1. 审计通过前，上游拨号为 0。
2. 每个被转发文本帧都已有对应通过判定；Blocked 帧在 fake upstream 中出现次数为 0。
3. 首文本前缓存帧和通过帧的上游到达顺序与客户端原顺序一致；Blocked 帧不会混入刷新队列。
4. 一个 Realtime 连接最多建立一次当前业务上游连接；正常业务重试不会导致同一帧重复审计或重复发。
5. 客户端同一连接收到的上游数据、审计 error 和 close 控制消息均由单一 writer 串行写出。
6. Block 后的下一条安全文本仍能成功处理；基础设施错误后的任何后续帧都不得转发。
7. 纯 `input_audio_buffer.append` 的 Base64 不进入 Guard；任何明确 transcript/text/instructions
   不得借控制帧或缓存绕过。
8. 审计关闭、分组未命中和已知 NoPrompt 路径不新增事件、不改变原协议输出。
9. 错误 event 和日志只包含稳定错误码、安全文案及 request_id，不包含 FullPrompt、ScanText、
   Guard 请求/响应、Token、Endpoint URL、密文或内部错误字符串。

## 五、实现约束与建议文件

具体符号和文件名以实际代码及冻结接口为准，不要机械照搬计划；预计至少涉及：

- 新增 `service/promptaudit/extract_realtime.go` 及其测试；
- 在 `service/promptaudit` 的稳定领域边界增加 Realtime Gate/适用性判断，避免 handler 复制 HTTP
  Gate 的 Manager、分组、Evaluator 和 EventStore 流程；
- 修改 `relay/websocket.go`，把“准备 adaptor”和“真正拨号上游”拆成可延迟、至多一次的生命周期；
- 修改 `relay/channel/openai/relay_realtime.go`，落实解析、审计、单写入器、缓冲、取消与错误行为；
- 添加相邻 Realtime handler/WebSocket 集成测试；若计费前置顺序确有问题，再以最小范围修改
  `controller/relay.go` 并添加回归测试。

实现时遵守：

- Router → Controller → Service → Model 分层；提取和审计领域逻辑放在 `service/promptaudit`，
  WebSocket 生命周期和协议转发留在 relay 层，Controller 只做接线和错误适配。
- 所有 JSON 编解码调用 `common.Marshal`、`common.Unmarshal`、`common.UnmarshalJsonStr` 或
  `common.DecodeJson`；业务代码禁止直接调用 `encoding/json` 的 Marshal/Unmarshal。允许使用
  `json.RawMessage` 等类型表达原始字段，但实际编解码仍走 `common.*`。
- 不把原始客户端帧或提示词拼进 `%v` 日志、error wrapping、panic 恢复消息、测试失败文本或指标标签。
- 保持 optional scalar 的显式 `0`、`0.0`、`false` 和原始 JSON 语义；审计解析不得重写转发帧。
- 代码直接、可读、低嵌套，优先早返回和清晰状态；不要新增只有一个调用点、没有稳定业务含义的
  包级 helper。必要注释使用中文，只解释协议差异、失败关闭、并发 ownership 和安全边界。
- 不使用不受控 goroutine、无界 channel、无界缓存、真实 sleep 或依赖时间竞争的同步方式。

## 六、测试与质量门禁

新增或大幅重写的 Go 测试使用 `github.com/stretchr/testify/require` 处理前置和致命断言，使用
`github.com/stretchr/testify/assert` 处理非致命值比较。采用确定性表驱动测试、本地
`httptest`/WebSocket fake 和显式同步信号，禁止随机输入、大循环、真实 sleep、时间性能比较、
日志式断言或只为覆盖率存在的测试。

至少覆盖：

1. `session.update`、`conversation.item.create`、`response.create` 的每类批准文本字段，验证
   提取顺序、角色、`FullPrompt`、`ScanText`、Hash、预览、protocol、stage 和 message count。
2. 工具描述/参数 schema、transcript、工具结果、多 content block、空文本和 Unicode；验证 URL、
   Base64 音频、模型名、event id、call id、控制值和无关 metadata 不被误扫。
3. 已知 NoPrompt 与未知/畸形 Unsupported 的明确差异，包括字段类型错误和可能藏有文本的未知事件。
4. 审计关闭、分组未命中、Manager degraded、Evaluator/EventStore 缺失或失败。
5. 首个 Block：fake upstream 拨号 0 次、收到帧 0 条、预扣费 0 次，客户端收到
   `prompt_guard_blocked`，连接仍可继续；随后安全文本只拨号一次并成功转发。
6. 首个 Pass/Flag：先审计和必要写事件，再拨号；缓存帧与当前帧按原序到达上游。
7. 上游已连接后的 Block：危险帧未到达，安全后续帧到达；Block 帧不计入输入 token/usage。
8. Unavailable、Invalid、ConfigDegraded、RecordFailed、Unsupported 和缓冲超限：上游未收到
   对应帧，客户端收到稳定 error，连接关闭且协程退出。
9. `store_pass_events=true/false` 的逐帧事件写入行为，以及必要 Block/Error 事件写入失败后不放行。
10. 纯音频/控制帧缓存边界、原序刷新、上游拨号失败、客户端先断开、上游先断开、审计中取消、
    close 竞争和 channel 满载，证明无 panic、重复 close、并发写或 goroutine 残留。
11. 正常上游响应透传、session format 更新、usage 累计、余额校验、预扣/结算/退款语义不回归。
12. 日志及 Realtime error event 的敏感信息防泄漏；使用 canary 值断言，而不是打印真实提示词。

每条后台测试命令自身设置不超过 60 秒的超时，按受影响包拆分。根据实际包和测试名调整，至少
执行并记录与以下等价的检查：

```bash
gofmt -w <本子任务实际修改的 Go 文件>
go test -count=1 -timeout 55s ./service/promptaudit
go test -count=1 -timeout 55s ./relay/channel/openai
go test -count=1 -timeout 55s ./relay
go test -count=1 -timeout 55s ./controller
go test -race -count=1 -timeout 55s ./relay/channel/openai
go vet ./service/promptaudit ./relay ./relay/channel/openai ./controller
```

若某包没有独立测试、命令超过时限或 race 检查需要进一步拆分，应使用 `-run` 按相关测试集拆分，
不能移除超时或用一个可能长时间运行的全仓命令代替。检查新增业务代码是否绕过 `common.*` JSON
包装，并检索日志/错误路径确认敏感字段不泄漏。

本子任务正常不修改数据库模式，因此不自行声称完成三数据库验证；如果实际改动影响数据库模型、
迁移、查询、事务或驱动行为，必须按根 `AGENTS.md` 完成 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+
真实实例矩阵，未完成时列为阻塞。正常不修改 `relaykit/`；若经重新审批后实际触及，额外执行：

```bash
cd relaykit && GOWORK=off go build ./...
```

## 七、检查、文档和交付

1. 实现完成后派发 `trellis-check` 对本子任务全范围检查并直接修复发现的问题，不能只看最后一批
   diff。检查必须覆盖子任务全部验收项、父任务 Realtime/失败关闭/计费/安全约束、核心和存储契约、
   client→audit→event store→upstream 数据流、upstream→client 写入流以及所有资源释放路径。
2. 检查后运行 `trellis-update-spec`，把值得长期保留的 WebSocket 单写入器、延迟拨号、Realtime
   事件提取或失败关闭经验写入合适规范；没有可沉淀内容时也应明确记录判断，不能伪造更新。
3. 更新 Realtime 子任务文档，记录最终状态机、实际提取矩阵、缓冲边界、稳定错误映射、修改文件、
   验证命令和结果。若实际实现发现父/子设计冲突或改变公共契约，先回到规划同步相关父子任务文档，
   重新提交规划摘要并等待批准。
4. 按根 `AGENTS.md` 将关键约束、最佳实践和经验写入 Serena memory；Serena 不可用时明确说明
   降级，不能声称已经写入。
5. 检查 `git diff` 和 `git status`，确认没有敏感数据、无关改动、占位符、TODO、旧兼容分支或
   被覆盖的并行工作。
6. 不自动执行 `git commit` 或 `git push`。按 Trellis Phase 3.4 先向用户展示分批提交计划、每批
   文件和所有未识别脏文件，等待明确确认后才能提交；禁止 push。提交完成后再按
   `trellis-finish-work` 完成收尾提醒。

只有以下条件全部满足，才可声明 Realtime 子任务完成：

- 子任务 PRD 每项验收标准都有实现和直接测试证据；
- 首文本前零上游拨号、后续逐帧门禁、Block 可继续、基础设施错误关闭均已由 fake WebSocket 验证；
- 计费、事件写入、错误响应和敏感信息不变量均已验证；
- 定向测试、race 检查、静态检查和格式化全部通过；
- 没有越过子任务范围，也没有覆盖其他任务改动；
- Trellis 全范围质量检查已通过。

最终交付用中文简洁报告：完成的提取矩阵与状态机、关键安全/并发/计费决策、可点击的绝对文件路径
及关键行号、执行过的精确命令和结果、未执行/失败验证及其影响、是否触及 `relaykit` 或数据库、
以及是否已满足父任务后续 integration-check 的启动条件。任何必要验证未执行、依赖未冻结或验收项
缺失时，必须明确列为阻塞，不能宣称“全部完成”。
