# 研究：Codex 环境上下文、标题生成与重复送审

采集时间：2026-09-04。来源：本地 `prompt_audit_events` #40–#45、`logs/oneapi-20260904180907.log`、openai/codex 源码（`gh api` 当前 main）、现有 `service/promptaudit` 实现。

## 1. 本地证据（用户只输入「你是什么模型？」）

窗口：2026-09-04 18:09:49–18:10:27 CST。Codex CLI 对同一句用户输入发出 **6 次** `POST /v1/responses`，审计写入 #40–#45。

| 事件 | 时间 | prompt_length | Guard 耗时 | 实际 Scan/落库 |
|---|---|---|---|---|
| #40 | 18:09:56 | 545 | 6501ms（远程） | 标题生成模板 + `User prompt: 你是什么模型？` |
| #41 | 18:09:56 | 905 | 7017ms（远程） | `<environment_context>…</environment_context>` + `你是什么模型？` |
| #42 | 18:10:02 | 905 | 0ms（缓存） | 与 #41 同 `prompt_hash` |
| #43–#45 | 18:10:06–18:10:22 | 905 | 0ms（缓存） | 同上 |

结论：

- AGENTS.md 信封已被上一任务剥掉，#41 密文不含 `# AGENTS.md instructions`。
- 远程 Guard **实际打了 2 次**，不是 6 次。后 4 次命中现有 10 分钟判定缓存，但仍写了 Allow 事件。
- #40 与 #41 在 18:09:49 / 18:09:50 **并发启动**，都在 18:09:56 才结束，现有缓存是「完成后写入」，挡不住同飞重复。
- #40 与 #41 的送审文本不同（标题模板 vs 环境上下文），即使加 singleflight，若不先剥成同一句用户提示词，仍会打 2 次分类器。
- 6 次上游 `gpt-5.6-sol` 转发是 Codex agent 循环，不是审计把一条 HTTP 拆开。审计只能控制 Guard 调用和事件写入，不能也不应拦截 Codex 后续轮次的上游调用。

#41 解密正文结构（节选）：

```
<environment_context>
  <cwd>…</cwd>
  <shell>zsh</shell>
  <current_date>2026-09-04</current_date>
  <timezone>Asia/Shanghai</timezone>
  <filesystem>…</filesystem>
</environment_context>

你是什么模型？
```

#40 解密正文结构（节选）：

```
<environment_context>…日期/时区…</environment_context>

Generate a concise, single-line task title of at most 36 characters …
Do not answer the request.

User prompt:
你是什么模型？
```

## 2. Codex 契约（识别必须用标记，不用正文指纹）

### 2.1 `environment_context`

源码：`codex-rs/protocol/src/protocol.rs`

- `ENVIRONMENT_CONTEXT_OPEN_TAG = "<environment_context>"`
- `ENVIRONMENT_CONTEXT_CLOSE_TAG = "</environment_context>"`

源码：`codex-rs/core/src/context/world_state/environment.rs`

- `role = "user"`
- `content_kind = environments.environment_context`
- `matches_text`：trim 后以开标签起头、以闭标签结尾（`matches_marked_text`，ASCII 大小写不敏感）

网关侧通常看不到 kind，必须以文本标记为准。

### 2.2 其它 role=user 的 contextual fragment

源码：`codex-rs/core/src/context/contextual_user_message.rs` 的 `CONTEXTUAL_USER_FRAGMENT_MATCHERS`。

本任务用同样的成对标记剥离（整段丢弃或从混合段切除），不做人设指纹：

| 标记 | 含义 |
|---|---|
| `# AGENTS.md instructions` + `<INSTRUCTIONS>` | 上一任务已剥 |
| `<environment_context>…</environment_context>` | 本任务主缺口 |
| `<skills_instructions>…</skills_instructions>` | skills |
| `<user_shell_command>…</user_shell_command>` | 用户 shell 回显 |
| `<turn_aborted>…</turn_aborted>` | 中止通知 |
| `<plugins_instructions>…</plugins_instructions>` | 插件说明 |
| `<external_…>` 自定义 additional context | Codex additional user fragment |

`environments.instructions` 角色是 **developer**，user-scan 已排除。

### 2.3 标题生成（不是 contextual fragment）

源码：`codex-rs/tui/src/app/thread_title.rs`

- `thread_title_instructions()` 以 `Generate a concise, single-line task title of at most ` 起头，并含 `Do not answer the request.`
- `thread_title_prompt()` 拼成：`{instructions}\n\nUser prompt:\n{user_message}`
- 这是隐藏线程的独立 `/v1/responses`，role 仍是 user

上一任务 AC7 要求「标题生成整段仍送审」。本任务修正：只保留 `User prompt:` 之后的真实用户文本。剥完后应与主对话变成同一句，从而合并送审。

识别契约：

1. trim 后以 `Generate a concise, single-line task title of at most ` 起头
2. 含独立行 `User prompt:`
3. 只保留该标签之后的正文；若为空则整段丢弃
4. 用户自己讨论「帮我起个标题」但没有这套模板，整段保留

## 3. 现有代码缺口

| 位置 | 现状 | 缺口 |
|---|---|---|
| `snapshot.go` `StripAgentsMdEnvelopes` | 只剥 AGENTS.md | 不剥 `<environment_context>`，不拆标题模板 |
| `guard.go` `DecisionCache` | 完成后缓存 Allow/Flag/Block，TTL 10 分钟，键 = config_version + scanners + remoteScan | 同飞重复各打一次 Guard；标题模板与主对话文本不同，缓存键不同 |
| `gate.go` `StorePassEvents` | 每次 Allow 都 `Record` | 缓存命中仍写事件，UI 出现 6 条 |
| `event_store.go` | 空 FullPrompt 的 Allow 不写事件 | 有用户文本的重复 Allow 仍写 |

`golang.org/x/sync v0.20.0` 已在 `go.mod`，可直接用 `singleflight.Group`，不必新依赖。

## 4. 对方案的约束

- 送审和落库继续用同一套「真实用户提示词」。
- 识别用 Codex 标记 / 标题模板结构，不用「工程师工作规范」或 `You are Codex`。
- 合并的是 **Guard HTTP**，不是拦截 Codex 后续上游调用。
- 不新增管理开关，不改事件表 schema，不回写旧密文。
