# 新窗口执行提示词：09-04-prompt-audit-exclude-agents-md

把本文件全文作为新上下文窗口的第一条用户消息粘贴即可。规划已审核通过，不要重新调研需求，不要再问“要不要建任务 / 要不要写方案 / 环境上下文要不要排除 / 标题生成要不要送审”，直接按下列步骤实现。

---

你是本仓库的实现代理。仓库：`/Users/laiyanfei/code/python/ai-project/github/new-api`。全程用中文回复。

## 任务

实现 Trellis 任务 **`09-04-prompt-audit-exclude-agents-md`**（父任务 `09-03-prompt-audit`）：

1. 提示词审计在 user-scan 之后，再剥掉 Codex 注入的 **AGENTS.md 信封**。送审和落库都只保留用户真实提示词。
2. 修复 LLM 分类器思考链禁用判断，使 **`deepseek/deepseek-v4-flash`** 真正注入 `thinking: {"type": "disabled"}`。

权威文档（必须按此实现，不要另起方案）：

- `.trellis/tasks/09-04-prompt-audit-exclude-agents-md/prd.md`
- `.trellis/tasks/09-04-prompt-audit-exclude-agents-md/design.md`
- `.trellis/tasks/09-04-prompt-audit-exclude-agents-md/implement.md`
- `.trellis/tasks/09-04-prompt-audit-exclude-agents-md/research/codex-agents-md-envelope.md`

状态：`planning`，产物齐全（prd / design / implement / research / jsonl）。**用户已于 2026-09-04 审核通过本方案。** `prd.md` / `implement.md` 里若仍写「本轮只写方案 / 等待审核 / 不要 start」，以本交接提示词为准：本窗口的工作是 Phase 1.3 核对 → 1.4 start → 2.1 实现 → 2.2 检查，不是 Phase 1.1。

## 启动顺序（先做这些，再写代码）

1. 用 Read 读仓库根目录 `AGENTS.md` 全文，并遵守其中全部规则。涉及 `web/` 时再读 `web/AGENTS.md`。
2. 读 `.agents/skills/trellis-start/SKILL.md`，执行：
   ```bash
   python3 ./.trellis/scripts/get_context.py
   python3 ./.trellis/scripts/get_context.py --mode phase
   python3 ./.trellis/scripts/get_context.py --mode packages
   ```
3. 读本任务 `prd.md` / `design.md` / `implement.md` / `research/codex-agents-md-envelope.md` 全文，以及 `.agents/skills/trellis-before-dev/SKILL.md`，按 skill 加载相关 spec。
4. **Phase 1.3**：核对 jsonl，不要推倒重写。当前 `implement.jsonl` / `check.jsonl` 已含真实 spec/research 条目（无 `_example`、无业务代码路径）。执行：
   ```bash
   python3 ./.trellis/scripts/task.py validate 09-04-prompt-audit-exclude-agents-md
   ```
   必须通过。若有人把 `service/promptaudit/*.go` 或 `web/src/...` 写进 jsonl，删掉再 validate。
5. **Phase 1.4**：方案已批准，直接启动，不要再向用户确认：
   ```bash
   python3 ./.trellis/scripts/task.py start 09-04-prompt-audit-exclude-agents-md
   python3 ./.trellis/scripts/task.py current --source
   ```
   必须确认 current 指向 `.trellis/tasks/09-04-prompt-audit-exclude-agents-md`，status 变为 `in_progress`。
6. 读 `.agents/skills/trellis-before-dev/SKILL.md` 并执行完，再改代码。
7. 按当前平台的 Trellis 2.1 实现：可 inline 自己写，或 `spawn_subagent`（`trellis-implement`）。dispatch 时 prompt **必须以** `Active task: .trellis/tasks/09-04-prompt-audit-exclude-agents-md` 开头，并声明自己已经是 implement 代理，禁止再套一层 implement/check。
8. 实现完成后读 `.agents/skills/trellis-check/SKILL.md` 做质量检查；前端文案读 `.agents/skills/i18n-translate/SKILL.md`。
9. 不要 `git commit` / `git push`，除非用户在本窗口明确要求。不要 `task.py archive`。不要 SSH 改生产配置、不要重新开启生产审计。

## 已冻结的产品决定（禁止改口径）

- 识别 AGENTS.md 只用 Codex 信封标记，**禁止**匹配 `You are Codex`，**禁止**匹配仓库正文（如「工程师工作规范」）。
- 信封契约（与 openai/codex `UserInstructions` 对齐）：
  - 文本 trim 后以 `# AGENTS.md instructions` 开头（大小写敏感）
  - 同时含 `<INSTRUCTIONS>` 与 `</INSTRUCTIONS>`
  - 可选 content item `kind=agents_md.instructions`
  - 替换/移除通知视为同一信封：`These AGENTS.md instructions replace all previously provided AGENTS.md instructions.` / `The previously provided AGENTS.md instructions no longer apply.`
- 整段是信封 → 丢弃整段。信封和真实文本拼在一起 → 去掉从 `# AGENTS.md instructions` 到对应 `</INSTRUCTIONS>` 的闭区间，保留剩余真实文本。
- 剥离发生在 `JoinUserSegments` / `SelectUserScanSegments` **之前**。ScanText、FullPrompt、预览、`prompt_hash`、`prompt_length`、`message_count`、详情解密用**同一套**过滤结果。
- 预览不得以 `# AGENTS.md instructions` 开头，除非用户自己输入了这句话且没有配套信封。
- Codex 标题生成请求（如 `Generate a concise, single-line task title...`）是真实 user，**必须送审**。
- 用户只是提到 AGENTS.md、没有信封标记（如「请根据 AGENTS.md 写个 README」）→ 整段仍送审、仍落库。
- 过滤后没有任何真实用户提示词：不打 Guard HTTP，Allow 转发，**不写事件**（与无 user 路径一致）。
- `latest_turn_only` 语义不变：只缩小送审范围为最新一轮**真实** user；落库仍是该请求全部真实 user（不含信封）。
- **本任务不排除** Codex 环境上下文、skills、hook、shell 回显等其它 contextual fragment。
- 不新增管理开关。不回写历史上已含 AGENTS.md 的密文。
- 思考链禁用：去掉 `provider/` 前缀后再判断模型名是否以 `deepseek-v4-` 开头；覆盖 `deepseek/deepseek-v4-flash`、`deepseek-v4-flash`、大小写变体。`*-none` 后缀仍要剥掉后再发给上游。`deepseek.com` BaseURL 继续禁用。不要用 `Contains("deepseek")`，以免误伤 V3。`gpt-4o` 不得注入 `thinking`。
- 失败关闭、分组、加密、缓存、远程截断、空 user 不写事件、Qwen3Guard / LLM 请求形态保持不变。

## 明确不做

- 「You are Codex」人设指纹或仓库 AGENTS.md 正文指纹
- 排除 environment_context / skills / hook / shell 等非 AGENTS.md fragment
- 把标题生成请求当成非用户输入丢掉
- 改 `timeout_ms`、`input_limit`、失败关闭、判定缓存 TTL
- Redis 扫描缓存；缓存里存提示词原文
- 新增「重新纳入 AGENTS.md」管理开关
- 回写或批量清理已入库含信封的旧密文
- SSH 生产机改审计配置或重新开启审计
- 改事件表 schema / AutoMigrate / 三库迁移矩阵
- 重做 HTTP Relay / Realtime / Task Plugin 门禁接线
- 修改受保护的项目身份 / 品牌信息
- 直接改 `web/src/i18n/locales/*.json`（必须走 i18n-translate skill）
- 改 `one-api.db*` 等本地运行时数据库文件
- 顺手重构 Qwen3Guard 协议或扩大思考链禁用到非 V4 模型

## 实现顺序（与 implement.md 一致）

### 阶段 1：剥离 AGENTS.md 信封

- 在 `service/promptaudit/` 实现信封判定与剥离（可放 `snapshot.go`；`IsAgentsMdEnvelope` / `StripAgentsMdEnvelopes` 是稳定领域概念，允许单独成函数）。
- `BuildPromptSnapshot` 在 `JoinUserSegments` / `SelectUserScanSegments` 之前调用剥离。
- Responses 提取测试仍可断言分段列表里存在信封；snapshot / Evaluate 断言 ScanText 与 FullPrompt 不含信封。

验证：

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s
```

必须覆盖：

- 人设 + AGENTS.md 信封 + `你好`：ScanText / FullPrompt 只有 `你好`
- 预览不以 `# AGENTS.md instructions` 开头
- 混合段：信封后跟真实文本，只保留真实文本
- 只有信封：Evaluate 不 HTTP、不写事件
- 用户提到 AGENTS.md 但无信封：整段保留
- 标题生成请求保留
- `latest_turn_only=true` 只作用于真实 user
- 替换/移除通知信封被丢掉
- `prompt_length` / `message_count` / `prompt_hash` 按过滤后文本计算

### 阶段 2：修复思考链禁用

- `service/promptaudit/llm_classifier.go`：规范化模型名（TrimSpace、ToLower、取最后一个 `/` 之后）再判断 `deepseek-v4-`；保留 `-none` 剥离与 `deepseek.com` BaseURL。
- `service/promptaudit/llm_classifier_test.go`：增加 `deepseek/deepseek-v4-flash`、`deepseek/deepseek-v4-flash-none`（发出的 model 为去掉 `-none` 后的名字）；保留原无前缀用例；断言 `gpt-4o` 无 `thinking`。

验证：

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s -run 'TestLLMClassifier|Thinking|deepseek'
```

AC8：`deepseek/deepseek-v4-flash` 请求体必须含 `thinking.type=disabled`。

### 阶段 3：前端说明与 i18n

- `web/src/features/prompt-audit/components/policy-tab.tsx` 与对应测试：`latest_turn_only` 说明写明排除 AGENTS.md 信封。
- i18n：en / zh / zh-TW / fr / ru / ja / vi。**禁止**直接改 locale JSON，走 `i18n-translate` skill（`add-missing-keys.mjs` + `bun run i18n:sync`）。
- 更新 `web/src/features/prompt-audit/__tests__/prompt-audit-page.test.tsx`（若该测试锁定了旧说明文案）。

验证：

```bash
cd web && bun run i18n:sync
cd web && bun run build
```

若可跑：`cd web && bun run test src/features/prompt-audit`（以 `web/package.json` 为准）。

### 阶段 4：回归

```bash
go test ./service/promptaudit/ ./controller/ ./model/ -count=1 -timeout 60s
cd relaykit && GOWORK=off go build ./...
```

本任务不应改 `relaykit/`。无 schema 变更，不做三数据库矩阵。不要改 `one-api.db*`。单测超时不超过 60s。

## 工程约束（本仓库硬规则）

- JSON 只走 `common.Marshal` / `Unmarshal` / `UnmarshalJsonStr` / `DecodeJson`，业务代码禁止直接 `encoding/json` marshal/unmarshal
- 不要为单调用方抽无稳定领域含义的包级 helper
- 新测试用 `testify/require` 做 setup/fatal，`assert` 做非致命比较
- 不要加随机输入、Sleep、只刷覆盖率的测试
- 前端包管理用 bun
- 代码注释用中文，只注释非显而易见的约束
- 不要改无关文件
- 日志不得包含 ScanText、FullPrompt、Token、完整 Guard 响应
- 不要破坏 user-scan 已有契约：非 user 角色、空 user 不写事件、失败耗时、判定缓存

## 风险文件（改前先读）

- `service/promptaudit/snapshot.go`：剥离顺序错误会把信封再次送进 Guard / 密文
- `service/promptaudit/guard.go`：空 ScanText 短路必须继续对「过滤后为空」生效
- `service/promptaudit/event_store.go`：过滤后空 FullPrompt 的 Allow 不得 Insert
- `service/promptaudit/llm_classifier.go`：匹配过宽会误伤非 V4；过窄则 `deepseek/deepseek-v4-flash` 仍然开思考链
- `service/promptaudit/llm_classifier_test.go`：现有用例只覆盖无厂商前缀
- `web/src/features/prompt-audit/components/policy-tab.tsx`：管理员对「保存什么」的理解

## 本地对照（不要当夹具抄进生产代码）

2026-09-04 17:32 本地事件 #33：用户只打「你好」，`prompt_length=5299`，`message_count=3`，预览为 `你好` + `# AGENTS.md instructions` + `<INSTRUCTIONS>`。本任务完成后，同类请求应变成约 2 个字符、1 段 user、预览以「你好」开头。分类器节点模型是 `deepseek/deepseek-v4-flash`。

## 完成标准

对照 `prd.md` Acceptance Criteria AC1–AC10 逐项给出证据（测试名或命令输出）。未跑的检查如实说，不要声称“已验证”。完成后停在实现+自检，等待用户决定是否 commit / archive。
