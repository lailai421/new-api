# 提示词审计只送审真实用户输入并合并重复送审

## Goal

提示词审计只把用户亲手输入的那句话送给 Guard、只把这句话加密落库。Codex 自动注入的 `<environment_context>` 以及其它非用户提示词一律不送审、不入库。同一句真实用户提示词在短时间内只远程送审一次。

用户价值：用 Codex 发「你是什么模型？」时，预览和详情就是「你是什么模型？」；分类器只打一次，不再因为标题生成、环境上下文、工具循环把 DeepSeek 费用打爆。

## Background

- 父任务：`09-03-prompt-audit`。前置：`09-04-prompt-audit-user-scan`（只保留 role=user）、`09-04-prompt-audit-exclude-agents-md`（剥 AGENTS.md 信封）。
- 本地证据（2026-09-04 18:09:49–18:10:27）：用户只输入「你是什么模型？」，产生事件 #40–#45。#41 仍含完整 `<environment_context>`（905 字、2 段 user）；#40 是 Codex 标题生成模板；#42–#45 与 #41 同 hash、Guard 耗时 0ms 但仍写 Allow 事件。远程分类器实际 2 次，事件 6 条。详见 `research/codex-context-and-duplicate-scan.md`。
- 根因 1：Codex 把环境上下文标成 `role=user`，现有过滤器只剥 AGENTS.md。
- 根因 2：标题生成是另一条 `/v1/responses`，模板与主对话文本不同，判定缓存命不中。
- 根因 3：缓存在远程返回后才写入，并发的两条请求会各打一次 Guard；Allow 缓存命中仍按 `StorePassEvents` 落库。
- 用户确认：只送审用户提示词；同一提示词多次送审不可接受，成本会爆炸。本轮只写方案，批准前不改业务代码。

## Requirements

- R1. 送审文本不得包含 Codex `<environment_context>…</environment_context>` 块（含 cwd、shell、date、timezone、filesystem 等子节点）。识别以开闭标签为准，允许标签前空白，ASCII 大小写不敏感。若 content item 带 `kind=environments.environment_context`，同样排除。
- R2. 其它 Codex 自动注入、且带稳定成对标记的 user 片段同样排除，至少包括：`<skills_instructions>`、`<user_shell_command>`、`<turn_aborted>`、`<plugins_instructions>`。AGENTS.md 信封继续按现有逻辑剥离。不得用人设指纹 `You are Codex`。
- R3. 若一整段 user 都是上述片段，丢弃整段。若一段里标记块和真实提示词拼在一起，去掉标记块，保留剩余真实文本。
- R4. Codex 标题生成请求（trim 后以 `Generate a concise, single-line task title of at most ` 起头，且含独立行 `User prompt:`）不得把模板送审。只保留 `User prompt:` 之后的正文。没有正文则整段丢弃。用户自己讨论起标题但没有这套模板时，整段仍送审。本需求修正 `09-04-prompt-audit-exclude-agents-md` 的 AC7。
- R5. 落库 `FullPrompt`、`prompt_hash`、`prompt_length`、`message_count`、详情解密正文、列表 `redacted_preview` 与送审使用同一套真实用户提示词。对「你是什么模型？」这一句，长度接近 7 个字，预览就是这句，不得再出现 `<environment_context>` 或标题模板。
- R6. 过滤后没有任何真实用户提示词时：不打远程 Guard，按 Allow 转发，且不写事件（与现有无 user 路径一致）。
- R7. 同一 `config_version` + 同一启用 scanners + 同一过滤后送审文本：并发请求只发起 **一次** 远程 Guard；完成后 10 分钟 TTL 内的后续请求不得再打远程 Guard。超时 / 不可用 / 非法响应仍不得写入判定缓存。缓存不得保存提示词原文。
- R8. 判定缓存命中（含并发合并的跟随者）的 Allow / Flag：**不得再写** `prompt_audit_events`。第一次远程（或启发式）判定仍按现有 `StorePassEvents` 规则写一条。Block 每次请求仍必须拦截；Block 事件仍每次写入，便于追查被拦请求。
- R9. 不得拦截 Codex 后续轮次的上游模型调用。合并的是 Guard 送审，不是 `/v1/responses` 转发。失败关闭、分组、加密、Qwen3Guard / LLM 协议、Probe、user-scan 非信封回归保持不变。
- R10. 不新增管理开关，不改事件表 schema，不回写历史密文。
- R11. 前端说明改为：保存的是用户真实提示词；排除客户端人设、AGENTS.md 信封、环境上下文等 Codex 自动片段、system、工具结果；同一提示词短时间内只远程送审一次。文案覆盖 en / zh / zh-TW / fr / ru / ja / vi。

## Acceptance Criteria

- [ ] AC1. 构造 Responses 请求：user 段为完整 `<environment_context>…cwd/shell…</environment_context>`，另一 user 段为 `你是什么模型？`。Guard 收到的文本是 `你是什么模型？`，不含 `<environment_context>`、`<cwd>`、`<filesystem>`。
- [ ] AC2. 同一请求的事件预览等于 `你是什么模型？`（96 rune 内看不到环境上下文）。详情解密正文只有这句。`message_count=1`，`prompt_length` 为这句的 rune 数。
- [ ] AC3. 标题生成请求（环境上下文 + `Generate a concise, single-line task title of at most …` + `User prompt:\n你是什么模型？`）过滤后 ScanText 也是 `你是什么模型？`。与 AC1 主对话并发时，远程 Guard HTTP 次数 = 1。
- [ ] AC4. 用户自己输入「请根据当前工作目录写 README」、正文不含 `<environment_context>` 时，整段仍送审、仍落库。
- [ ] AC5. 过滤后只剩环境上下文 / 标题模板、没有真实 user 时：不发起 Guard HTTP，返回 Allow，不插入 `prompt_audit_events`。
- [ ] AC6. 同一过滤后文本连续 6 次 Evaluate（模拟 Codex 循环）：远程 Guard HTTP = 1；Allow 事件行数 = 1；后 5 次仍 Allow 并放行上游。修改 `config_version` 后必须重新调用 Guard。
- [ ] AC7. 两次并发 Evaluate 同一过滤后文本：远程 Guard HTTP = 1（singleflight），不得出现两次分类器调用。
- [ ] AC8. 第一次判定为 Block 后，缓存窗口内第二次相同文本仍返回 Block 且不调用 Guard；第二次仍写入 Block 事件。
- [ ] AC9. 超时 / 503 结果不得被第二次请求当成成功缓存；失败关闭行为不变。
- [ ] AC10. `latest_turn_only=true` 时仍只送审最新一轮真实 user；落库仍是该请求全部真实 user，不含环境上下文。
- [ ] AC11. 管理页说明与全部前端语言写明排除环境上下文等自动片段，以及短时间重复送审合并。无新配置项、无 schema 变更。
- [ ] AC12. AGENTS.md 剥离、无 user 不写事件、失败关闭、分组、加密、Qwen3Guard / LLM 协议、Probe 回归不被改坏。

## Out of Scope

- 不拦截、不限流 Codex 对上游模型的多次 `/v1/responses`（工具循环、思考续写仍会计上游费用）。
- 不维护「You are Codex」人设指纹名单。
- 不把判定缓存做到 Redis。
- 不回写、不批量清理已入库的含环境上下文密文。
- 不新增「重新纳入环境上下文」开关。
- 不改 `timeout_ms`、`input_limit`、失败关闭策略、事件表结构。
- 不在本任务切换生产配置。
- 不修改受保护的项目身份、品牌或归属信息。

## Confirmed Decisions

- 2026-09-04：只送审用户真实提示词；`<environment_context>` 全部去掉。
- 2026-09-04：同一提示词多次送审不可接受。合并对象是远程 Guard，不是上游模型转发。
- 2026-09-04：标题生成只保留 `User prompt:` 后的用户原文，不再把模板当作用户提示词。
- 2026-09-04：Codex 其它带成对标记的 user 自动片段一并排除。
- 2026-09-04：Allow/Flag 缓存命中不重复写事件；Block 每次仍写事件并拦截。
- 2026-09-04：不新增配置项；沿用现有 10 分钟进程内缓存。
- 2026-09-04：本轮只写方案，批准前不写业务代码。

## Notes

- 本任务为复杂任务，实施前以本 `prd.md`、`design.md`、`implement.md` 为共同基线。
- 本任务修正 `09-04-prompt-audit-exclude-agents-md`：标题生成不再整段送审。
- 标记契约与本地事件拆解见 `research/codex-context-and-duplicate-scan.md`。
