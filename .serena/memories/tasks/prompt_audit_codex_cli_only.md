# 提示词审计仅允许 Codex CLI：实现约束（2026-09-04）

- Trellis 任务：`.trellis/tasks/09-04-prompt-audit-codex-cli-only`，状态 `in_progress`；未 commit、未 archive。
- 身份是兼容性识别，不是鉴权。唯一硬判定：入站 HTTP `Originator`。Header 名走 Go 大小写不敏感；值先 TrimSpace，再 ASCII 小写后完整匹配。
- 允许值单点在 `service/promptaudit/client.go`：`codex_cli_rs`、`codex-cli`、`codex-tui`、`codex_exec`。禁止子串、正则、User-Agent/Session/X-Codex 回退。交互 TUI 实际发送 `codex-tui`，不是发行标识 `codex-cli`。
- 统一入口：`CheckAuditClientAccess`。调用方：`CheckRelayRequest`、`CheckMidjourneyRequest`、`CheckTaskRequest`、`CheckTaskPluginProtocolRequest`、Realtime Upgrade 前的 `controller/relay.go`。
- 顺序：Manager 缺失放行 → degraded 用 `prompt_audit_config_degraded` → 审计关闭放行 → 非允许 Originator 用 `prompt_audit_codex_cli_required` 503 → 合法 Codex 再走原分组/送审。身份检查早于分组。
- 错误码/安全消息单点在 `types.go`：`ErrorCodeCodexCLIRequired`、`CodexCLIRequiredMessage`。禁止回显 Originator、UA、Prompt、Token。
- Relay 用 `NewRelayAuditError` + SkipRetry；Task/Plugin 保持 LocalError；Realtime 未 Upgrade 时走普通 OpenAI HTTP JSON 503，不 Hijack。
- Claude 外壳在 `controller/relay.go` 显式补 `error.code`，因为 `ToClaudeError()` 只有 type/message。不要改 relaykit 公共 API。
- 非 Codex 拒绝不得提取 Prompt、不得 Evaluate、不得 Record、不得预扣费、不得拨号上游。
- 帧级 Realtime 测试不必加 Originator；只有真正经过客户端前置门禁的夹具才补 `Originator: codex_cli_rs`。
