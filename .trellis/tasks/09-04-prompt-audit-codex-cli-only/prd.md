# 提示词审计仅允许 Codex CLI

## Goal

当全局提示词审计开启时，只允许标准 Codex CLI 客户端进入受审计保护的生成与任务提交链路。其他客户端在送审、计费和业务上游调用之前直接收到 HTTP 503，避免非目标客户端占用 Guard 资源或继续访问模型上游。

## Background

- 本任务是父任务 `09-03-prompt-audit` 的新增子任务；规划完成后必须等待用户审核，不在本轮修改业务代码。
- HTTP Relay、Midjourney、Task、Task Plugin 与 Realtime 分别从 `service/promptaudit/gate.go:18`、`:111`、`:254`、`:368` 及 `service/promptaudit/gate_realtime.go:25`/`:44` 进入提示词审计。
- 普通 HTTP Relay 在 `controller/relay.go:132` 调用审计。Realtime 在 `controller/relay.go:85` 先升级 WebSocket，而 `CheckRelayRequest` 会跳过 Realtime（`service/promptaudit/gate.go:19`），所以 Realtime 客户端身份检查必须前移到 WebSocket 升级之前。
- 项目尚无统一的 Codex 客户端判定函数。现有 Codex 请求头契约包含 `Originator`、`User-Agent` 和会话/线程头（`setting/operation_setting/channel_affinity_setting.go:39`）；Codex 渠道适配器默认写入 `originator: codex_cli_rs`（`relay/channel/codex/adaptor.go:185`）。
- 用户已确认采用兼容性识别：`Originator` 是硬判定依据，`User-Agent` 等仅作辅助诊断，不新增签名或设备证明。请求头可以伪造，这是本任务明确接受的风险。

## Requirements

- **R1 — 开关语义**：审计关闭或 Manager 未初始化时，不执行 Codex CLI 客户端限制，所有现有请求行为保持不变。
- **R2 — 判定契约**：审计开启时，读取入站 HTTP `Originator`，Trim 后按 ASCII 大小写不敏感做**完整值**匹配。显式允许 `codex_cli_rs`、`codex-cli`、`codex-tui` 与 `codex_exec`；空值、未知值、仅包含允许值的前后缀字符串均不允许。VS Code / Desktop / Monitor 等非 CLI 第一方客户端不允许。
- **R3 — 辅助头不授权**：`User-Agent`、`Session_id`、`Thread_id`、`X-Codex-*` 等头不得单独授予访问权限；它们不参与首版硬判定。
- **R4 — 全局优先级**：客户端限制按全局审计开关生效，执行顺序早于审计分组匹配。审计开启后，非 Codex CLI 即使位于未纳入扫描范围的分组，也必须返回 503。
- **R5 — 入口覆盖**：同一判定契约覆盖普通 HTTP Relay、Realtime WebSocket 握手、Midjourney 提交、原生 Task 提交及 Task Plugin Responses Bridge 提交。只查询任务或资源、不承载提示词提交的接口不受影响。
- **R6 — 短路保证**：客户端不符合时，不解析/构造提示词快照，不调用 `Evaluator.Evaluate`，不调用 `EventStore.Record`，不预扣费，不选择或调用业务上游渠道。
- **R7 — Realtime 握手**：Realtime 非 Codex CLI 请求必须在 WebSocket Upgrade 前返回普通 HTTP 503；不得建立客户端 WebSocket，也不得建立上游 WebSocket。
- **R8 — 错误契约**：新增稳定错误码 `prompt_audit_codex_cli_required`，HTTP 状态固定为 503，安全消息固定为 `Prompt audit is enabled; only Codex CLI requests are accepted.`。OpenAI、Claude、Midjourney、Task 和 Task Plugin 沿用各自既有错误外壳，不回显收到的请求头。
- **R9 — 失败关闭保持**：已通过客户端身份检查的 Codex CLI 请求继续执行现有分组、提示词提取、Guard 判定、事件存储、失败关闭、计费与上游转发逻辑；配置 degraded 的既有 503 语义不得弱化。
- **R10 — 可维护性**：允许值、规范化规则与错误码必须由 `service/promptaudit` 单点维护，各入口不得复制字符串判断。

## Acceptance Criteria

- [ ] **AC1（R1）**：审计关闭时，无 `Originator`、未知 `Originator` 和合法 Codex `Originator` 均不因本功能被拒绝。
- [ ] **AC2（R2、R3）**：审计开启时，`codex_cli_rs`、`CODEX_CLI_RS`、`codex-cli`、`codex-tui`、`codex_exec` 及带首尾空白的等价值通过；空值、`curl`、`my-codex_cli_rs-wrapper`、`codex_vscode`、`Codex Desktop` 以及只有 Codex `User-Agent`/会话头的请求均返回 503。
- [ ] **AC3（R4）**：审计开启且请求分组不匹配扫描范围时，非 Codex CLI 仍返回 503；合法 Codex CLI 继续按既有分组规则放行且不送审。
- [ ] **AC4（R5、R8）**：HTTP Relay、Midjourney、Task 与 Task Plugin 非 Codex CLI 提交均返回状态 503、错误码 `prompt_audit_codex_cli_required` 和固定安全消息。
- [ ] **AC5（R6）**：所有非 Codex CLI 拒绝用例均断言 Evaluator 调用数、EventStore 写入数、预扣费次数和业务上游调用数为 0。
- [ ] **AC6（R7）**：Realtime 非 Codex CLI 请求在 WebSocket Upgrade 前得到 HTTP 503；客户端/上游 WebSocket 连接数均为 0。
- [ ] **AC7（R8）**：错误响应和日志不包含提示词、Token、完整 `Originator`、`User-Agent` 或其他入站头内容。
- [ ] **AC8（R9）**：合法 Codex CLI 且命中审计分组时，Allow、Block、Guard 不可用、配置 degraded、事件写入失败等既有行为及状态码保持不变。
- [ ] **AC9（R9）**：现有审计测试夹具补充标准 Codex `Originator` 后，继续验证原有真实行为，不通过关闭新门禁或放宽断言来规避回归。
- [ ] **AC10（R10）**：代码检索确认所有入口复用同一客户端识别函数和同一错误常量，不存在第二套 `Originator` 值判断。

## Out of Scope

- 不实现不可伪造的客户端认证、签名、设备证明或私有密钥握手。
- 不新增管理配置项、Originator 自定义白名单或前端开关。
- 不限制管理 API、模型列表、任务查询/回调、文件下载、健康检查等不承载提示词提交的接口。
- 不改造提示词提取、风险分类器、Guard 节点、分组配置、缓存、事件表或计费规则。
- 不修改数据库 schema、历史审计事件、`relaykit/` 公共 API 或生产环境配置。

## Deferred Items

- 若官方 Codex CLI 后续更改标准 `Originator`，通过独立兼容性变更显式扩展允许值并补充契约测试；首版不接受通配符、子串或管理员自定义值。
- 若未来需要防止伪造，另建任务设计带密钥或签名的客户端认证，不能把本任务的请求头识别宣传为安全证明。

