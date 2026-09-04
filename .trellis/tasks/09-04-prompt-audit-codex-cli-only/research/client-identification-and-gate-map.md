# 调研：Codex CLI 客户端识别与提示词审计入口

调研日期：2026-09-04。范围：当前工作区代码、本机 `codex-cli 0.153.2` 标识、现有提示词审计任务资料。未修改业务代码，未读取或发送真实 Token。

## 1. 当前审计入口

- 普通 HTTP Relay：`controller/relay.go:115-135` 完成请求解析和 `RelayInfo` 构造后调用 `promptaudit.CheckRelayRequest`。
- Midjourney：`controller/relay.go:465-489` 在业务提交前调用 `CheckMidjourneyRequest`。
- 原生 Task：`controller/relay.go:631-649` 在来源任务解析和提交前调用 `CheckTaskRequest`。
- Task Plugin Responses Bridge：`controller/plugin_protocol.go:245-265` 在任务来源解析和提交前调用 `CheckTaskPluginProtocolRequest`。
- Realtime：`controller/relay.go:85-92` 先完成客户端 WebSocket Upgrade；之后 `relay/websocket.go:33-58` 与 `relay/channel/openai/relay_realtime.go:72-77` 才根据 `ShouldAuditRealtime` 延迟业务上游拨号，实际提示词在 `CheckRealtimeEvent` 中逐帧审计。

结论：普通入口可在各自 gate 中短路；Realtime 若要返回普通 HTTP 503，必须在 `upgrader.Upgrade` 前复用同一客户端门禁。

## 2. 现有执行顺序

`service/promptaudit/gate.go` 的四个入口均采用以下结构：

1. 获取 Manager；
2. 配置 degraded 时失败关闭；
3. 读取 ActiveConfig，关闭时放行；
4. 判断分组；
5. 提取 PromptSegment、构造 PromptSnapshot；
6. 调用 Evaluator；
7. 必要时写 EventStore；
8. 返回 Allow / 403 / 503。

客户端限制必须插入“确认审计开启”之后、“分组判断”之前，才能满足全局开关语义，并天然保证非 Codex 请求不创建快照、不送审、不落事件。

## 3. 可用客户端信号

- `setting/operation_setting/channel_affinity_setting.go:39-66` 已把 `Originator`、`User-Agent`、Session/Thread 及 `X-Codex-*` 列为 Codex CLI 透传头。
- `relay/channel/codex/adaptor.go:185-187` 和 `service/codex_wham_usage.go:158-160` 在缺失时写入 `originator: codex_cli_rs`，这是仓库内最明确的机器可读 Codex 标识。
- 本机 CLI 版本输出为 `codex-cli 0.153.2`，发行标识使用 `codex-cli`。为兼容当前发行标识与仓库既有 `codex_cli_rs` 标识，方案采用两个**完整值**的显式允许集合。
- `relay/channel/api_request_test.go:161-205` 中的 `Originator: Codex CLI` 是请求头透传测试数据，不是生产端写入契约；首版不把展示文案值加入允许集合。

判定规则：Go `http.Header.Get` 已处理头名大小写；对值做 `TrimSpace` 和 ASCII 小写后，只接受 `codex_cli_rs`、`codex-cli`。不做子串、正则或 User-Agent 回退。

## 4. 信任边界

所有普通 HTTP 客户端都能自行设置 `Originator`，所以此规则只能区分“声明自己是标准 Codex CLI 的请求”，不能证明进程真实来源。增加 User-Agent、Session 或 X-Codex 头的组合也不能形成不可伪造认证，只会增加版本兼容风险。

用户已接受这一限制。若要防伪，需要密钥/签名/设备证明及 Codex 侧配置，必须另行设计。

## 5. 错误与零副作用约束

- 稳定错误码：`prompt_audit_codex_cli_required`。
- HTTP 状态：503，与现有 Guard 不可用/配置降级的失败关闭状态一致。
- 安全消息：`Prompt audit is enabled; only Codex CLI requests are accepted.`。
- 不回显实际 `Originator`、User-Agent、请求体、Prompt 或 Token。
- `types.NewAPIError` 必须带 `ErrOptionWithSkipRetry()`；Task 错误保持 `LocalError: true`，避免渠道重试。
- 拒绝发生在 Snapshot、Evaluator、EventStore、预扣费和业务上游之前。

## 6. 测试影响

现有 gate/controller 审计测试多数没有构造 `Originator`。实现后，凡是要继续测试 Block、Allow、失败关闭、事件存储等下游行为的夹具，都必须显式加标准 Codex Originator；非 Codex 新用例则故意留空或使用未知值。这样测试继续保护真实审计行为，而不是被新门禁提前截断后产生假阳性。

