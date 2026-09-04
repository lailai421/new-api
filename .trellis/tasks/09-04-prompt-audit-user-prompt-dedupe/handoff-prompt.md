# 新窗口执行提示词：09-04-prompt-audit-user-prompt-dedupe

把本文件全文作为新上下文窗口的第一条用户消息粘贴即可。规划已审核通过，不要重新调研需求，不要再问“要不要建任务 / 要不要写方案 / 环境上下文要不要排除 / 标题生成要不要整段送审 / 要不要拦截 Codex 上游多轮调用”，直接按下列步骤实现。

---

你是本仓库的实现代理。仓库：`/Users/laiyanfei/code/python/ai-project/github/new-api`。全程用中文回复。

## 任务

实现 Trellis 任务 **`09-04-prompt-audit-user-prompt-dedupe`**（父任务 `09-03-prompt-audit`）：

1. 提示词审计在 user-scan 与 AGENTS.md 剥离之后，再去掉 Codex `<environment_context>` 以及其它带成对标记的自动 user 片段；标题生成只保留 `User prompt:` 之后的真实用户正文。送审和落库都只留用户亲手输入。
2. 同一句过滤后的真实用户提示词：并发只打 **一次** 远程 Guard；10 分钟 TTL 内后续请求走判定缓存；Allow/Flag 缓存命中 **不再写事件**。Block 每次仍拦截、仍写事件。

权威文档（必须按此实现，不要另起方案）：

- `.trellis/tasks/09-04-prompt-audit-user-prompt-dedupe/prd.md`
- `.trellis/tasks/09-04-prompt-audit-user-prompt-dedupe/design.md`
- `.trellis/tasks/09-04-prompt-audit-user-prompt-dedupe/implement.md`
- `.trellis/tasks/09-04-prompt-audit-user-prompt-dedupe/research/codex-context-and-duplicate-scan.md`

状态：`planning`，产物齐全（prd / design / implement / research / jsonl / 本交接文件）。**用户已于 2026-09-04 审核通过本方案。** `prd.md` / `implement.md` / `task.json` 里若仍写「本轮只写方案 / 等待审核 / 不要 start」，以本交接提示词为准：本窗口的工作是 Phase 1.3 核对 → 1.4 start → 2.1 实现 → 2.2 检查，不是 Phase 1.1。

前置已落地、不要重做：`09-04-prompt-audit-user-scan`、`09-04-prompt-audit-exclude-agents-md`。本任务修正 exclude-agents-md 的 **AC7**：标题生成不再整段送审。

## 启动顺序（先做这些，再写代码）

1. 用 Read 读仓库根目录 `AGENTS.md` 全文，并遵守其中全部规则。涉及 `web/` 时再读 `web/AGENTS.md`。
2. 读 `.agents/skills/trellis-start/SKILL.md`，执行：
   ```bash
   python3 ./.trellis/scripts/get_context.py
   python3 ./.trellis/scripts/get_context.py --mode phase
   python3 ./.trellis/scripts/get_context.py --mode packages
   ```
3. 读本任务 `prd.md` / `design.md` / `implement.md` / `research/codex-context-and-duplicate-scan.md` 全文，以及 `.agents/skills/trellis-before-dev/SKILL.md`，按 skill 加载相关 spec。
4. **Phase 1.3**：核对 jsonl，不要推倒重写。当前 `implement.jsonl` / `check.jsonl` 已含真实 spec/research 条目（无 `_example`、无业务代码路径）。执行：
   ```bash
   python3 ./.trellis/scripts/task.py validate 09-04-prompt-audit-user-prompt-dedupe
   ```
   必须通过。若有人把 `service/promptaudit/*.go` 或 `web/src/...` 写进 jsonl，删掉再 validate。
5. **Phase 1.4**：方案已批准，直接启动，不要再向用户确认：
   ```bash
   python3 ./.trellis/scripts/task.py start 09-04-prompt-audit-user-prompt-dedupe
   python3 ./.trellis/scripts/task.py current --source
   ```
   必须确认 current 指向 `.trellis/tasks/09-04-prompt-audit-user-prompt-dedupe`，status 变为 `in_progress`。
6. 读 `.agents/skills/trellis-before-dev/SKILL.md` 并执行完，再改代码。
7. 按当前平台的 Trellis 2.1 实现：可 inline 自己写，或 `spawn_subagent`（`trellis-implement`）。dispatch 时 prompt **必须以** `Active task: .trellis/tasks/09-04-prompt-audit-user-prompt-dedupe` 开头，并声明自己已经是 implement 代理，禁止再套一层 implement/check。
8. 实现完成后读 `.agents/skills/trellis-check/SKILL.md` 做质量检查；前端文案读 `.agents/skills/i18n-translate/SKILL.md`。
9. 不要 `git commit` / `git push`，除非用户在本窗口明确要求。不要 `task.py archive`。不要 SSH 改生产配置、不要重新开启生产审计。

## 已冻结的产品决定（禁止改口径）

- 只送审、只落库用户真实提示词。Codex 自动注入内容一律不送审。
- 识别用 Codex **成对标记** / 标题模板结构，**禁止**匹配 `You are Codex`，**禁止**匹配仓库正文（如「工程师工作规范」）。
- `<environment_context>…</environment_context>` 必须整块去掉（含 cwd、shell、date、timezone、filesystem）。识别：trim 后开闭标签，ASCII 大小写不敏感；可选 kind=`environments.environment_context`。
- 其它必须排除的成对标记（整段丢弃或从混合段切除）：
  - `<skills_instructions>` / `</skills_instructions>`
  - `<plugins_instructions>` / `</plugins_instructions>`
  - `<user_shell_command>` / `</user_shell_command>`
  - `<turn_aborted>` / `</turn_aborted>`
- AGENTS.md 信封继续走现有 `StripAgentsMdEnvelopes`，不要重写那套契约。
- 剥离顺序（在 `BuildPromptSnapshot` 内、Join/Select **之前**）：
  1. `StripAgentsMdEnvelopes`
  2. 成对标记剥离（环境上下文等）
  3. 展开标题生成模板
- 标题生成：trim 后以 `Generate a concise, single-line task title of at most ` 起头，且含独立行 `User prompt:` → 只保留该标签之后的正文；没有正文则整段丢弃。用户讨论起标题但没有这套模板 → 整段仍送审。这修正上一任务 AC7。
- ScanText、FullPrompt、预览、`prompt_hash`、`prompt_length`、`message_count`、详情解密用**同一套**过滤结果。
- 对「你是什么模型？」：预览就是这句；`message_count=1`；长度接近 7 个 rune；不得再出现 `<environment_context>` 或标题模板。
- 过滤后无真实 user：不打 Guard HTTP，Allow 转发，**不写事件**。
- 同一 `config_version` + scanners + 过滤后送审文本：并发 `singleflight` 只打一次远程 Guard；完成后沿用现有 10 分钟进程内缓存。不缓存失败。缓存不存原文。用 `golang.org/x/sync/singleflight`（`go.mod` 已有），不要新依赖。
- `Decision.FromCache` 仅为内存字段，**不落库**。缓存命中与 singleflight 跟随者：`FromCache=true`。Allow/Flag 且 FromCache → gate **不 Record**。Block 忽略 FromCache，每次拦截、每次写事件。
- **不得**拦截 Codex 后续轮次的上游 `/v1/responses`。合并的是 Guard 送审，不是模型转发。验收只数 Guard HTTP 次数和审计事件行，不要去限流上游。
- 不新增管理开关。不改事件表 schema。不回写含环境上下文的旧密文。
- 失败关闭、分组、加密、远程截断、空 user 不写事件、Qwen3Guard / LLM 请求形态、Probe 保持不变。
- `latest_turn_only` 只缩小送审范围为最新一轮**真实** user；落库仍是该请求全部真实 user（不含自动片段）。

## 明确不做

- 「You are Codex」人设指纹或仓库正文指纹
- 拦截 / 限流 Codex 对上游模型的多次 `/v1/responses`（工具循环、思考续写仍会计上游费用）
- 把标题生成里 `User prompt:` 之后的用户原文丢掉
- 改 `timeout_ms`、`input_limit`、失败关闭、判定缓存 TTL
- Redis 扫描缓存；缓存里存提示词原文
- 新增「重新纳入环境上下文」管理开关
- 回写或批量清理已入库含环境上下文的旧密文
- SSH 生产机改审计配置或重新开启审计
- 改事件表 schema / AutoMigrate / 三库迁移矩阵
- 重做 HTTP Relay / Realtime / Task Plugin 门禁接线
- 重写 AGENTS.md 剥离或思考链禁用逻辑
- 修改受保护的项目身份 / 品牌信息
- 直接改 `web/src/i18n/locales/*.json`（必须走 i18n-translate skill）
- 改 `one-api.db*` 等本地运行时数据库文件
- 顺手重构 Qwen3Guard 协议

## 实现顺序（与 implement.md 一致）

### 阶段 1：剥离环境上下文与其它标记块，展开标题模板

- 在 `service/promptaudit/` 实现成对标记剥离与标题展开（可放 `snapshot.go`；这是稳定领域概念，允许单独成函数）。
- `BuildPromptSnapshot`：`StripAgentsMdEnvelopes` → 成对标记剥离 → 标题展开 → 再 `JoinUserSegments` / `SelectUserScanSegments`。
- 提取测试仍可断言分段列表里存在 `<environment_context>`；snapshot / Evaluate 断言 ScanText 与 FullPrompt 只有真实 user。

验证：

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s
```

必须覆盖：

- 环境上下文段 + `你是什么模型？`：ScanText / FullPrompt 只有后者；预览不含 `<environment_context>`
- 混合段：标记块后跟真实文本，只保留真实文本
- 只有环境上下文或只有标题模板：Evaluate 不 HTTP、不写事件
- 标题生成抽出 `User prompt:` 正文，与主对话变成同一 ScanText
- 用户提到工作目录 / 起标题但无标记：整段保留
- AGENTS.md 剥离仍然生效
- `latest_turn_only=true` 只作用于真实 user
- `prompt_length` / `message_count` / `prompt_hash` 按过滤后文本计算

### 阶段 2：并发合并与 Allow 缓存命中不写事件

- `GuardEvaluator`：缓存未命中时用 `singleflight.Group` 包远程扫描；返回前 `cloneDecision`；`FromCache` 标记命中与跟随者。
- `gate.go` / `gate_realtime.go`（HTTP Relay、Midjourney、Realtime、Task）：Allow/Flag 且 `FromCache` 时不 `Record`；Block 每次 `Record`。
- 现有 TTL、不缓存失败、不存原文保持不变。

验证：

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s -run 'TestUserScan_DecisionCache|Test.*Singleflight|Test.*FromCache|Test.*Title|Test.*Environment|Test.*Dedupe'
```

必须覆盖：

- 连续 6 次相同过滤后文本：Guard HTTP = 1，Allow 事件 = 1，后 5 次仍 Allow 并放行
- 两个并发 Evaluate 同一文本：Guard HTTP = 1
- 改 `config_version` 后必须重新 HTTP
- Block 缓存命中仍 403，且第二次仍写 Block 事件
- 超时 / 503 不得当成功缓存
- 标题生成与主对话并发：Guard HTTP = 1

### 阶段 3：前端说明与 i18n

- `web/src/features/prompt-audit/components/policy-tab.tsx` 与对应测试：说明排除环境上下文等 Codex 自动片段，并写明短时间重复送审合并。
- i18n：en / zh / zh-TW / fr / ru / ja / vi。**禁止**直接改 locale JSON，走 `i18n-translate` skill（`add-missing-keys.mjs` + `bun run i18n:sync`）。
- 更新锁定旧说明文案的前端测试（若有）。

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
- 并发测试用 goroutine + httptest 计数 Guard HTTP，不要用 `time.Sleep` 当同步原语
- 前端包管理用 bun
- 代码注释用中文，只注释非显而易见的约束
- 不要改无关文件
- 日志不得包含 ScanText、FullPrompt、Token、完整 Guard 响应
- 不要破坏 user-scan / AGENTS.md 已有契约：非 user 角色、空 user 不写事件、失败耗时、信封剥离、思考链禁用

## 风险文件（改前先读）

- `service/promptaudit/snapshot.go`：剥离顺序错误会把环境上下文再次送进 Guard / 密文；标题展开过早会让模板逃过标记剥离
- `service/promptaudit/guard.go`：空 ScanText 短路必须继续对「过滤后为空」生效；singleflight 必须 clone Decision；失败不得写入 DecisionCache
- `service/promptaudit/types.go`：`FromCache` 只是内存字段，不要加进事件表或 JSON 落库
- `service/promptaudit/gate.go` / `gate_realtime.go`：Allow 缓存命中漏跳过会再次写出 6 条事件；Block 误跳过会丢掉拦截记录
- `service/promptaudit/event_store.go`：过滤后空 FullPrompt 的 Allow 不得 Insert
- `service/promptaudit/agents_md_test.go`：上一任务「标题生成整段送审」的用例必须改成抽出 `User prompt:` 正文，不要删回归
- `web/src/features/prompt-audit/components/policy-tab.tsx`：管理员对「保存什么 / 为何不是 6 条」的理解

## 本地对照（不要当夹具抄进生产代码）

2026-09-04 18:09:49–18:10:27，用户只打「你是什么模型？」：

- 6 次 `POST /v1/responses`（Codex 自己发的，本任务不拦截转发）
- 事件 #40：标题生成，545 字，远程 Guard 6501ms
- 事件 #41：环境上下文 + 用户句，905 字，远程 Guard 7017ms
- 事件 #42–#45：与 #41 同 hash，Guard 0ms，但仍写了 Allow 事件

本任务完成后，同类请求应变成：

- 送审 / 预览 / 密文只有「你是什么模型？」
- 远程 Guard HTTP = 1（标题生成与主对话合并）
- Allow 事件 = 1
- 上游仍可多轮 200 转发

## 完成标准

对照 `prd.md` Acceptance Criteria AC1–AC12 逐项给出证据（测试名或命令输出）。未跑的检查如实说，不要声称“已验证”。完成后停在实现+自检，等待用户决定是否 commit / archive。
