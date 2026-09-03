# 新上下文交接提示词：提示词审计 HTTP Relay 接入

你现在位于仓库：

`/Users/laiyanfei/code/python/ai-project/github/new-api`

请继续 Trellis 子任务：

`09-03-prompt-audit-http-relay`

目标是完整实施并验证“标准 HTTP Relay 与 Midjourney 的提示词提取和同步审计门禁”。不要创建重复任务，也不要启动父任务 `09-03-prompt-audit`。

## 1. 必须遵守的工作方式

1. 全程使用中文回答、分析、注释和文档。
2. 先读取仓库根目录 `AGENTS.md`，并遵守其中全部约束；若涉及更深目录，再读取对应目录的 `AGENTS.md`。
3. 按 `.trellis/workflow.md` 和当前 `<workflow-state>` 推进。先运行：

   ```bash
   python3 ./.trellis/scripts/task.py current --source
   ```

   新上下文没有活动任务、活动任务仍指向父任务，均不代表需要新建任务。始终以现有子任务目录为工作目标。
4. 当前交接文件生成时，子任务状态仍为 `planning`。先核对规划材料并向用户提交最终规划摘要，摘要必须包含 Goal、In Scope、Out of Scope、Acceptance Criteria、Key Decisions、Risks/Deferred Items 和 artifact status。若没有证据表明用户已在该摘要之后明确批准实施，则停止并等待批准；不得自行运行 `task.py start`。
5. 用户随后明确批准最新摘要后，运行：

   ```bash
   python3 ./.trellis/scripts/task.py start 09-03-prompt-audit-http-relay
   ```

   只启动这个子任务，不启动父任务。
6. 进入 `in_progress` 后，遵循 Trellis 的主会话流程：`trellis-implement` → `trellis-check` → `trellis-update-spec` → 提交计划确认 → `trellis-finish-work` 提醒。派发实现/检查代理时，提示词第一行必须是：

   ```text
   Active task: .trellis/tasks/09-03-prompt-audit-http-relay
   ```

   明确告诉代理它不是代码库中唯一的工作者，不得回退或覆盖其他人的改动；它已经是对应 Trellis 代理，不得再次递归派发 `trellis-implement` 或 `trellis-check`。
7. 不发送可省略的过程性 commentary，只在需要汇报关键进度、风险、阻塞或用户决策时发消息。
8. 开始修改前检查 `git status --short`，保护所有既有或并行改动。不得使用 `git reset --hard`、`git checkout --` 等破坏性命令，不得擅自删除、覆盖或提交与本子任务无关的文件。
9. 不修改、删除、替换任何与受保护项目身份 **nеw-аρi** 或组织/作者身份 **QuаntumΝоuѕ** 相关的品牌、元数据、归属和引用。
10. 本任务不授权 `git commit`、`git push`、数据库破坏性操作或依赖升级。提交前必须按 Trellis 工作流展示分批提交计划并等待用户明确确认；禁止 push。

## 2. 必读上下文与读取顺序

不要依赖本提示词复述来替代原始材料。按以下顺序完整读取：

1. `.trellis/tasks/09-03-prompt-audit-http-relay/prd.md`
2. `.trellis/tasks/09-03-prompt-audit-http-relay/design.md`
3. `.trellis/tasks/09-03-prompt-audit-http-relay/implement.md`
4. `.trellis/tasks/09-03-prompt-audit-http-relay/implement.jsonl`
5. `.trellis/tasks/09-03-prompt-audit-http-relay/check.jsonl`
6. `.trellis/tasks/09-03-prompt-audit/prd.md`
7. `.trellis/tasks/09-03-prompt-audit/design.md`
8. `.trellis/tasks/09-03-prompt-audit/implement.md`
9. `.trellis/tasks/09-03-prompt-audit/research/codebase-analysis.md`
10. `.trellis/tasks/09-03-prompt-audit-core/design.md`
11. `.trellis/tasks/09-03-prompt-audit-storage-api/design.md`
12. `implement.jsonl` / `check.jsonl` 引用的全部规范与研究文件。

本提示词生成时，依赖状态为：

- `09-03-prompt-audit-storage-api`：`completed`。
- `09-03-prompt-audit-core`：`in_progress`。

状态可能已发生变化，必须重新读取两个依赖任务的 `task.json` 并检查实际工作树。只有核心任务和存储任务均已完成、所需公共契约已落地且验证通过后，才可启动本子任务。若核心仍未完成、接口仍在变化或实现与设计存在冲突，报告具体缺口并停止，不得在本子任务内复制一套临时 Snapshot、Evaluator、Decision、Manager 或 EventStore 实现来绕过依赖。

实现前使用代码检索确认实际符号、调用关系和当前请求流。文档描述与已落地代码不一致时，以已批准的产品需求为准，回到规划阶段记录差异并请求决策；不要猜测接口或强行兼容过时设计。

## 3. 本子任务的准确范围

### 需要实现

- 标准 HTTP Relay 的确定性文本提取：
  - OpenAI Chat / Completions；
  - OpenAI Responses / Compaction；
  - Claude Messages；
  - Gemini；
  - Images；
  - Embeddings；
  - Rerank；
  - Alpha Search；
  - Audio 中明确由客户端提供并会发送给业务上游的文本字段。
- Midjourney 提交类动作的 `prompt`、确实作为提示指令使用的 `content` 等明确字段。
- 基于实际已落地核心契约构造 `PromptSnapshot`，包括稳定顺序的完整原文、实际送审文本、完整原文 Hash、脱敏预览、协议/阶段信息以及从 Gin/TokenAuth 上下文复制的身份和渠道快照。
- 默认审计即将进入业务上游的完整可提取文本；`latest_turn_only` 只改变 `ScanText`，不得改变完整 `FullPrompt` 的留存内容。
- 在标准 Relay 和 Midjourney 的业务上游调用、敏感词检查、token 估算、价格计算、预扣费及业务渠道 retry 之前执行一次同步 Gate。
- 将 Block、Unavailable、Invalid、ConfigDegraded、RecordFailed、Unsupported 等结果映射为父设计规定的稳定、安全、协议兼容错误；错误必须是本地错误并设置 `SkipRetry`，不得进入业务渠道重试、渠道禁用或计费流程。
- 添加有业务意义的确定性测试，证明提取结果、门禁位置、错误契约和旁路行为正确。

### 明确不包含

- OpenAI Realtime WebSocket 的握手、延迟建连和逐帧门禁。
- 视频任务、通用 Task Plugin、Responses Bridge 的插件补充提取以及 `auditTextPaths`。
- 前端管理页、管理 API、事件表、清理任务或 Guard 核心的重新实现。
- 图片、音频、视频二进制内容的视觉/语音审核。
- 业务上游生成响应的内容审核。
- 与提示词审计无关的重构、依赖升级或品牌修改。

`controller/relay.go` 是 HTTP Relay 与 Task Plugin 子任务的共享热点。修改前后都要检查现有差异，保留 Realtime、Task Plugin 和其他并行工作，不得用旧版本文件整体覆盖。

## 4. 必须保持的行为与安全不变量

1. 审计模式只有关闭和同步阻断；不得增加异步只记录后放行的路径。
2. 审计关闭、请求分组未命中审计范围、或已明确证明为无文本的查询/控制请求时，保持原有行为。
3. 使用明确的 DTO/协议字段分派，不得递归扫描任意 JSON 字符串。
4. `system`、`developer`、`user`、`assistant`、`tool`、工具结果及协议特有、确实会上送的文本按已批准协议矩阵覆盖。
5. 模型名、URL、Data URL、Base64、回调地址、凭据、业务元数据和二进制载荷不得误入 Guard。
6. 必须明确区分：
   - `NoPrompt`：协议和动作已知，且本次请求确实没有需要送审的新文本；
   - `Unsupported`：未知格式、解析失败或无法证明没有上送文本。审计开启且命中范围时必须失败关闭。
7. Gate 对每个业务请求只执行一次，不随业务渠道 retry 重复执行。
8. Block 或任何失败关闭错误发生时：
   - 业务上游调用次数为 0；
   - 预扣费次数为 0；
   - 不进入敏感词、token 估算、价格计算和业务 retry 的后续链路。
9. Guard 通过后，如当前配置要求保存该事件，必须先完成必要事件写入再放行业务上游；写入失败返回 `prompt_audit_record_failed` 并失败关闭。
10. 请求体读取必须使用项目现有可复用 body 机制，不能消费、重排或改写原始请求语义。审计提取与重新序列化不得丢失 optional scalar 的显式 `0`、`0.0` 或 `false`。
11. 完整原文不截断；超长内容按核心契约以 Unicode 字符分片全部扫描，不得只扫描前 N 个字符。
12. 完整提示词、`ScanText`、Guard 请求体、Guard 原始响应、节点 Token/地址和内部异常不得进入日志、错误响应、指标标签或测试失败输出。客户端错误只暴露稳定错误码、安全文案和 `request_id`。
13. 新增业务代码中的 JSON 编解码必须使用 `common.Marshal`、`common.Unmarshal`、`common.UnmarshalJsonStr` 或 `common.DecodeJson`；不得直接调用 `encoding/json` 的 Marshal/Unmarshal。允许引用 `json.RawMessage` 等类型。
14. 不破坏 `relaykit/` 的独立构建边界。根模块代码不得通过修改 `relaykit/` 来下沉提示词审计依赖。
15. 保持 Router → Controller → Service → Model 分层；入口只做接线和协议响应适配，审计领域逻辑放在 `service/promptaudit` 的稳定边界内。

## 5. 实施要求

按实际代码和冻结后的核心接口调整文件名，但交付内容至少覆盖：

1. 实现或补齐 segment、稳定拼接、全文/最新轮选择、SHA-256 Hash、96-rune 脱敏预览和身份快照。
2. 为每一种标准 Relay DTO 编写显式提取逻辑；对 union、字符串/数组内容块、工具调用与工具结果等边界做确定处理。
3. 为 Midjourney 建立提交/查询/通知/资源动作分类，只审计会向业务上游提交新文本的动作。
4. 在 `controller.Relay` 的 `GenRelayInfo` 成功之后、现有敏感词检查之前接入统一 Gate。
5. 在 `controller.RelayMidjourney` 识别提交动作并安全读取 DTO 之后、调用业务 submit 之前接入统一 Gate。
6. 复用已落地的全局 Manager/Evaluator/EventStore 与错误契约，不在 controller 中自行解释 Guard 文本或重写判定规则。
7. 使用清晰命名、早返回和直接分支。不要为了缩短调用者而创建只有一个调用点、没有稳定业务含义的包级 helper。
8. 代码注释只用于关键流程、协议差异和安全边界，使用中文，不写重复代码表意的注释。

## 6. 测试与质量门禁

新增或大幅重写的 Go 测试使用：

- `github.com/stretchr/testify/require`：前置条件和致命断言；
- `github.com/stretchr/testify/assert`：非致命值断言。

测试必须是确定性的表驱动或精确场景测试，不得用随机输入、大循环、sleep、时间对比或只记录日志的伪压力/冒烟测试。至少覆盖：

1. 每种协议的完整输入和最新轮输入精确等于预期，包含角色、内容块、工具结果和协议特有字段。
2. URL、Data URL、Base64、模型名、回调地址和元数据被明确排除。
3. NoPrompt 与 Unsupported 的差异。
4. 审计关闭、分组未命中和 NoPrompt 时不改变原链路。
5. Pass/Warn 时只执行一次审计并进入原业务链路，业务 retry 不重复审计。
6. Block、Unavailable、Invalid、ConfigDegraded、RecordFailed、Unsupported 时断言：上游调用 0 次、预扣费 0 次；不要只断言 HTTP 状态码。
7. Midjourney Pass/Block，以及查询/通知类 NoPrompt；Block 时 submit 调用 0 次。
8. 可复用请求体在审计后仍可被原处理链读取，显式 `0`、`0.0`、`false` 不丢失。
9. 错误响应与日志不包含提示词原文、Token、Guard 请求/响应和内部节点信息。

所有测试命令自身设置不超过 60 秒的超时，并按受影响包拆分执行，避免用一个可能长时间运行的全仓命令阻塞。例如根据实际包调整并执行：

```bash
go test -timeout 60s ./service/promptaudit
go test -timeout 60s ./controller
```

随后执行与变更范围相称的构建、静态检查和格式检查。先对实际修改的 Go 文件执行 `gofmt`。若实施中意外触及 `relaykit/`，除根模块检查外必须执行：

```bash
cd relaykit && GOWORK=off go build ./...
```

本子任务正常不涉及数据库模式变更；若实际改动影响数据库行为，则必须按根 `AGENTS.md` 完成 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 的真实实例验证矩阵。未完成时明确报告阻塞，不得声称三库兼容或任务完成。

## 7. 完成与交付

实现代理完成后，必须派发 `trellis-check` 做全范围检查并直接修复发现的问题，不能只检查最后一批改动。检查范围应同时覆盖：

- 子任务 PRD、设计、实施计划和验收项；
- 父任务的协议矩阵、失败关闭和安全约束；
- 依赖核心/存储契约；
- 所有实际受影响包的 Trellis Quality Check；
- 请求流、错误流、事件写入流、计费流和上游调用流；
- 敏感数据泄漏、重复审计、协议漏扫与误扫。

检查通过后使用 `trellis-update-spec` 判断并记录值得沉淀的新约束或陷阱。不要因为本提示词要求“完成”就跳过 Trellis 的提交确认：先展示提交批次、文件和未识别脏文件，等待用户明确确认后才可提交；不得 push。

最终汇报必须包含：

1. 已实现的协议与门禁位置；
2. 关键安全/兼容决策；
3. 修改文件及对应说明，使用可点击的绝对路径和关键行号；
4. 执行过的精确验证命令及结果；
5. 未执行、失败或受环境限制的验证及其影响；
6. 是否触及 `relaykit/` 或数据库行为；
7. 是否仍有依赖、并行改动或集成验收需要后续子任务处理。

不要在依赖未就绪、验收项缺失、测试未通过或必要验证未执行时宣称任务完成。
