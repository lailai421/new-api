# 提示词审计排除 AGENTS.md — 技术设计

## 1. 设计目标

1. 在 user-scan 之后再剥掉 Codex `AGENTS.md` 信封，使 ScanText / FullPrompt 只剩真实用户提示词。
2. LLM 分类器对 `deepseek/deepseek-v4-flash` 这类带厂商前缀的 DeepSeek V4 模型关闭思考链。

不改门禁接线、失败关闭、加密算法、事件表 schema。

## 2. 边界

| 保持不变 | 本任务改变 |
|---|---|
| 同步阻断、分组、失败关闭 | 真实 user 文本不含 AGENTS.md 信封 |
| 事件表、AES-GCM | `FullPrompt` / 预览 / hash / 长度 / 条数按过滤后文本 |
| `latest_turn_only` 语义 | 分类器思考链匹配含 `provider/model` |
| 无 user → Allow 且不写事件 | 过滤后无真实 user 走同一空路径 |
| Qwen3Guard / LLM 请求形态 | 前端说明点名排除 AGENTS.md 信封 |
| 历史已入库密文 | |

不新增 Option 字段。不迁移旧行。

## 3. 数据流

```
ExtractRelay/Realtime/Task
        │
        ▼
  []PromptSegment（仍含 AGENTS.md 信封，role=user）
        │
        ▼
  StripAgentsMdEnvelopes
        │  丢掉纯信封段；混合段去掉信封块
        ▼
  []PromptSegment（仅真实 user + 原非 user 段）
        │
        ├─ JoinUserSegments → FullPrompt / hash / length / count / 密文
        └─ SelectUserScanSegments → ScanText / preview
                    │
                    ▼
              GuardEvaluator（其后逻辑不变）
                    └─ LLMClassifierScanner
                         └─ shouldDisableThinking(model, baseURL)
```

提取层仍可以看到信封，便于单测断言「提取到了但不送审」。热路径在 `BuildPromptSnapshot` 入口做剥离，Realtime / Task 走同一 snapshot 函数则自动生效。

## 4. AGENTS.md 信封契约

与 Codex `UserInstructions` 对齐（`codex-rs/core/src/context/user_instructions.rs`）：

| 字段 | 值 |
|---|---|
| role | `user` |
| kind（可选） | `agents_md.instructions` |
| 起始标记 | `# AGENTS.md instructions` |
| 内层开闭 | `<INSTRUCTIONS>` … `</INSTRUCTIONS>` |

判定 `IsAgentsMdEnvelope(text)`：

1. `strings.TrimSpace(text)` 后，以 `# AGENTS.md instructions` 开头（大小写敏感，与 Codex 源码一致），且同时包含 `<INSTRUCTIONS>` 与 `</INSTRUCTIONS>`；或
2. 整段是替换/移除通知变体：以同一标题开头，正文含 `These AGENTS.md instructions replace` 或 `The previously provided AGENTS.md instructions no longer apply.`

`StripAgentsMdEnvelopes(segments)`：

- 非 user 段原样保留（人设仍不当 user）。
- user 段若整段是信封：丢弃。
- user 段若中间夹着信封：删除从 `# AGENTS.md instructions` 到对应 `</INSTRUCTIONS>` 的闭区间（含标记），再 Trim；剩余为空则丢弃。
- 不根据仓库 AGENTS.md 正文（如「工程师工作规范」）做内容指纹。

不把环境上下文、skills 等其它 Codex fragment 纳入本任务过滤器。

## 5. 与 user-scan 的关系

`JoinUserSegments` / `SelectUserScanSegments` 继续只看 `IsUser()`。本任务保证进入这两个函数的 user 段已经没有信封。

因此：

- `audit_scope` 仍是 `user` / `latest_turn`
- 空真实 user 时 ScanText 与 FullPrompt 为空，Evaluate 不 HTTP，Allow 不写事件
- `latest_turn_only` 仍只缩小送审，落库仍是该请求全部**真实** user

## 6. 思考链禁用

把 `llm_classifier.go` 的前缀判断换成「规范化模型名」：

1. `TrimSpace` + `ToLower`
2. 取最后一个 `/` 之后的段（`deepseek/deepseek-v4-flash` → `deepseek-v4-flash`）
3. 若该段以 `deepseek-v4-` 开头，或 BaseURL 含 `deepseek.com`，则注入 `thinking: {type: disabled}`
4. 若原模型名（未截 provider 前）以 `-none` 结尾，发出时去掉 `-none`

测试必须包含 `deepseek/deepseek-v4-flash`。不要用 `Contains("deepseek")`，以免误伤 V3 或其它模型。

## 7. 前端

`latest_turn_only` 说明补一句：排除 Codex `AGENTS.md` 信封。英文源键更新后跑 `bun run i18n:sync`，并补 zh / zh-TW / fr / ru / ja / vi。

## 8. 兼容与回滚

- 聊天补全、Claude、Gemini 的普通 user 不受影响，除非有人故意发送同一信封。
- 用户讨论 AGENTS.md 但未带信封标记的文本仍送审。
- 回滚：还原剥离函数与思考链判断即可；已按新规则写入的事件无需回迁。
- 本任务上线前含信封的旧密文不改，管理员用现有删除/保留期处理。
