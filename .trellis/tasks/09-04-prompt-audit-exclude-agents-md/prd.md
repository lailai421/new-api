# 提示词审计排除 AGENTS.md 并修复 DeepSeek 思考链禁用

## Goal

提示词审计只把用户真实输入送给 Guard、只把用户真实输入加密落库。Codex CLI 自动注入的 `AGENTS.md` 信封不得送审、不得入库。同时让 LLM 分类器在模型名为 `deepseek/deepseek-v4-flash` 时真正关闭思考链，避免空正文导致 `prompt_guard_invalid_response`。

用户价值：用 Codex 发「你好」时，审计预览和详情就是「你好」，而不是仓库规范；分类器也不再因为没关思考链而 503。

## Background

- 父任务：`09-03-prompt-audit`。前置子任务 `09-04-prompt-audit-user-scan` 已把 system / developer / assistant / 工具结果 / Responses `instructions` 排除。
- 本地证据（2026-09-04 17:32–17:33）：用户只输入「你好」，事件 #33 仍是 5299 字符、3 段 user，预览为 `你好` + `# AGENTS.md instructions` + `<INSTRUCTIONS>`。详见 `research/codex-agents-md-envelope.md`。
- 根因 1：Codex 把 `AGENTS.md` 标成 `role=user`，user-scan 按角色全收。
- 根因 2：分类器禁用思考链只匹配 `HasPrefix(model, "deepseek-v4-")`，不匹配 `deepseek/deepseek-v4-flash`。
- 用户确认：不论用什么手段，永远不要把 AGENTS.md 内容送审；只获取用户真实提示词送审。本轮只写方案，批准前不改业务代码。

## Requirements

- R1. 送审文本不得包含 Codex 注入的 AGENTS.md 信封。识别以 Codex 源码契约为准：文本以 `# AGENTS.md instructions` 起头（允许前面空白），并含 `<INSTRUCTIONS>` 与 `</INSTRUCTIONS>`。若 content item 带 `kind=agents_md.instructions`，同样排除。不得用人设指纹 `You are Codex`。
- R2. 若一整段 user 都是该信封，丢弃整段。若一段里信封和真实提示词拼在一起，去掉信封块，保留剩余真实文本。替换通知、移除通知（`These AGENTS.md instructions replace...` / `The previously provided AGENTS.md instructions no longer apply.`）视为同一信封。
- R3. 落库 `FullPrompt`、`prompt_hash`、`prompt_length`、`message_count`、详情解密正文、列表 `redacted_preview` 与送审使用同一套「真实用户提示词」。预览不得以 `# AGENTS.md instructions` 开头，除非用户自己输入了这句话且没有配套信封。
- R4. Codex 自动发出的其它真实 user 文本仍要送审，例如 `Generate a concise, single-line task title...`。历史真实 user 仍按现有 `latest_turn_only` 规则处理。
- R5. 过滤后没有任何真实用户提示词时：不打远程 Guard，按 Allow 转发，且不写事件（与无 user 路径一致）。
- R6. 不新增管理开关。不把「重新纳入 AGENTS.md」做成可选项。不回写历史上已含 AGENTS.md 的密文。
- R7. LLM 分类器在模型名（含 `provider/model`）指向 DeepSeek V4 时必须注入 `thinking: {"type": "disabled"}`。至少覆盖 `deepseek/deepseek-v4-flash`、`deepseek-v4-flash`、大小写变体。`*-none` 后缀仍要剥掉后再发给上游。`deepseek.com` BaseURL 继续禁用思考链。非 V4 模型不得误开。
- R8. 前端说明改为：保存的是用户真实提示词，排除客户端人设、AGENTS.md 信封、system、工具结果。文案覆盖 en / zh / zh-TW / fr / ru / ja / vi。
- R9. 本任务修正 `09-04-prompt-audit-user-scan`：`role=user` 仍不够，必须再去掉 AGENTS.md 信封。失败关闭、分组、加密、缓存、远程截断、空 user 不写事件保持不变。

## Acceptance Criteria

- [ ] AC1. 构造 Responses 请求：`instructions` 人设 + user 段 `# AGENTS.md instructions\n\n<INSTRUCTIONS>\n仓库规范\n</INSTRUCTIONS>` + user 段 `你好`。Guard 收到的文本含 `你好`，不含 `仓库规范`，也不含 `# AGENTS.md instructions`。
- [ ] AC2. 同一请求的事件预览以 `你好` 开头（96 rune 内看不到 AGENTS.md 信封标题）。详情解密正文只有真实 user，不含信封。
- [ ] AC3. `prompt_length` / `message_count` / `prompt_hash` 按过滤后的真实 user 计算。上例 `message_count=1`，长度接近「你好」，而不是信封全文。
- [ ] AC4. 用户自己输入「请根据 AGENTS.md 写个 README」、正文不含信封标记时，整段仍送审、仍落库。
- [ ] AC5. 过滤后只剩信封、没有真实 user 时：不发起 Guard HTTP，返回 Allow，不插入 `prompt_audit_events`。
- [ ] AC6. `latest_turn_only=true` 时仍只送审最新一轮真实 user；落库仍是该请求全部真实 user，不含信封。
- [ ] AC7. 标题生成请求（user 文本以 `Generate a concise, single-line task title` 开头、无信封）仍送审。
- [ ] AC8. `LLMClassifierScanner.Scan` 对 `deepseek/deepseek-v4-flash` 的请求体含 `thinking.type=disabled`；对 `deepseek-v4-flash`、`deepseek/deepseek-v4-flash-none`（发出的 model 为去掉 `-none` 后的名字）同样注入。`gpt-4o` 不注入。
- [ ] AC9. 管理页 `latest_turn_only` 说明与全部前端语言写明：排除 AGENTS.md 信封。无新配置项、无 schema 变更。
- [ ] AC10. 现有失败关闭、分组、加密、Qwen3Guard / LLM 协议、Probe、user-scan 的非信封回归不被改坏。

## Out of Scope

- 不维护「You are Codex」人设指纹名单。
- 不在本任务排除 Codex 环境上下文、skills、hook、shell 回显等其它 contextual fragment（可后续加；本任务只保证 AGENTS.md 信封）。
- 不改 `timeout_ms`、`input_limit`、失败关闭、判定缓存 TTL。
- 不把扫描缓存做到 Redis。
- 不回写、不批量清理已入库的含 AGENTS.md 密文。
- 不在本任务切换生产配置。
- 不修改受保护的项目身份、品牌或归属信息。

## Confirmed Decisions

- 2026-09-04：用户要求无论用什么手段，永远不要把 AGENTS.md 送审，只送真实提示词。
- 2026-09-04：识别用 Codex 信封标记，不用项目 AGENTS.md 正文指纹（例如「工程师工作规范」）。
- 2026-09-04：送审和落库同一过滤结果；预览和详情都不得再出现该信封。
- 2026-09-04：标题生成请求仍视为真实 user。
- 2026-09-04：思考链禁用必须覆盖当前生产/本地使用的 `deepseek/deepseek-v4-flash`。
- 2026-09-04：本轮只写方案，批准前不写业务代码。

## Notes

- 本任务为复杂任务，实施前以本 `prd.md`、`design.md`、`implement.md` 为共同基线。
- 本任务修正子任务 `09-04-prompt-audit-user-scan` 的「全部 role=user 都是用户提示词」假设。
- Codex 信封与思考链缺口的源码锚点见 `research/codex-agents-md-envelope.md`。
