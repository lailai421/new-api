# 研究：Codex 如何把 AGENTS.md 送进 /v1/responses

采集时间：2026-09-04。来源：本地审计事件 #33–#39、openai/codex 源码（commit `8e6a44b`）、现有分类器代码。

## 本地证据（user-scan 上线后）

窗口：2026-09-04 17:32:35–17:33:18 CST。用户只输入「你好」，Codex CLI 走 `POST /v1/responses`。

| 事件 | 结果 | prompt_length | message_count | 预览开头 |
|---|---|---|---|---|
| #33 | allow 7420ms | 5299 | 3 | `你好` + `# AGENTS.md instructions` + `<INSTRUCTIONS>` + 工程师工作规范 |
| #34–#38 | unavailable / invalid_response | 4939 | 3 | `Generate a concise, single-line task title...` |
| #39 | allow 6680ms | 4939 | 3 | 同上 |

`audit_scope=user`：人设 `You are Codex` 已排除。AGENTS.md 仍在 user 分段里，所以被送审、被落库。

分类器节点当时：`protocol=llm_classifier`，`model=deepseek/deepseek-v4-flash`，`timeout_ms=8000`，`input_limit=4000`。失败码混有 `prompt_guard_unavailable`（8001ms）和 `prompt_guard_invalid_response`（4.7–7.4s）。

## Codex 信封契约

源码：`codex-rs/core/src/context/user_instructions.rs`

- `ContentItemKind`：`agents_md.instructions`
- `role`：`user`（不是 system / developer）
- 文本标记：`("# AGENTS.md instructions", "</INSTRUCTIONS>")`
- body：`{directory}\n\n<INSTRUCTIONS>\n{text}\n`

源码：`codex-rs/core/src/context/world_state/agents_md.rs`

- Codex 自己用 `UserInstructions::matches_text` 识别这段信封，而不是靠「You are Codex」人设指纹。
- 替换通知：`These AGENTS.md instructions replace all previously provided AGENTS.md instructions.`
- 移除通知：`The previously provided AGENTS.md instructions no longer apply.`

源码：`codex-rs/core/src/context/contextual_user_message.rs`

- Codex 把 AGENTS.md 信封归类为 contextual user fragment（不是用户打出来的那一句）。
- 另有 `environments.instructions`，角色是 **developer**，user-scan 已经排除。

网关侧通常只看到标准 Responses item：`type=message, role=user, content[].text`。`content_item_kinds` 不一定会出现在发给 new-api 的 JSON 里。因此识别必须以 **文本标记** 为主，kind 为辅。

## 思考链禁用缺口

`service/promptaudit/llm_classifier.go:137`：

```
strings.HasPrefix(modelName, "deepseek-v4-") || strings.Contains(endpoint.BaseURL, "deepseek.com")
```

本地配置模型是 `deepseek/deepseek-v4-flash`，BaseURL 是 `https://api.commandcode.ai/provider`。两条都不命中，不会注入 `thinking: {type: disabled}`。

现有测试只覆盖 `deepseek-v4-flash` / `deepseek-v4-flash-none`，没有厂商前缀。

`extractOpenAIContent` 在只有 `reasoning_content`、正文为空时返回 `prompt_guard_invalid_response`。这与 #35/#36/#38 吻合。

## 对方案的约束

- 不要用人设字符串 `You are Codex` 当过滤器；用 Codex 自己的 AGENTS.md 信封标记。
- 过滤必须发生在 JoinUser / SelectUserScan 之前或之内，送审和落库用同一套「真实用户提示词」。
- 不要把「Generate a concise, single-line task title」当 AGENTS.md 丢掉；那是另一条真实 user 文本。
- 思考链判断必须识别 `provider/model` 形式，至少覆盖 `deepseek/deepseek-v4-flash`。
