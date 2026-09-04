# 提示词审计仅允许 Codex CLI：规划约束（2026-09-04）

- Trellis 任务：`.trellis/tasks/09-04-prompt-audit-codex-cli-only`，父任务 `09-03-prompt-audit`，状态保持 `planning`。
- 用户已确认采用兼容性识别：入站 `Originator` 是硬判定依据，User-Agent/Session/X-Codex 等辅助头不得单独授权；明确接受请求头可伪造的风险，不做签名/设备证明。
- 首版 Originator 完整值 allowlist：规范化后仅 `codex_cli_rs`、`codex-cli`；Trim + ASCII 大小写不敏感，不做子串、正则或通配符。
- 全局审计开启后，客户端门禁早于审计分组；非 Codex 即使 group mismatch 也返回 503。
- 稳定错误码 `prompt_audit_codex_cli_required`，固定安全消息 `Prompt audit is enabled; only Codex CLI requests are accepted.`，不得回显头、Prompt 或 Token。
- 覆盖 HTTP Relay、Midjourney、Task、Task Plugin、Realtime；Realtime 必须在 WebSocket Upgrade 前返回普通 HTTP 503。
- 非 Codex 拒绝必须保证 PromptSnapshot/Evaluator/EventStore/预扣费/业务上游均为 0；Relay 保持 SkipRetry，Task 保持 LocalError。
- 合法 Codex 请求继续既有 degraded、分组、审计、事件、计费与上游逻辑；不改数据库、前端配置、relaykit API。
- 规划文档 `prd.md`、`design.md`、`implement.md`、research 及 implement/check JSONL 已齐；必须等用户明确审核批准后才能 `task.py start` 和实施。