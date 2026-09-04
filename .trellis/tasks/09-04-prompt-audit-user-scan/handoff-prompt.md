# 新窗口执行提示词：09-04-prompt-audit-user-scan

把本文件全文作为新上下文窗口的第一条用户消息粘贴即可。规划已审核通过（含「只保存用户提示词」的补充），不要重新调研需求，不要再问“要不要建任务 / 要不要写方案 / 人设要不要入库”，直接按下列步骤实现。

---

你是本仓库的实现代理。仓库：`/Users/laiyanfei/code/python/ai-project/github/new-api`。全程用中文回复。

## 任务

实现 Trellis 任务 **`09-04-prompt-audit-user-scan`**（父任务 `09-03-prompt-audit`）：提示词审计默认只扫描、只落库**用户提示词**。Codex CLI 等人设、system、developer、assistant、工具结果既不送 Guard，也不写入 `full_prompt_ciphertext`。同时修复：预览用人设开头、`latest_turn_only` 无 user 时退回全文、失败事件 `latency_ms=0`、Codex 工具循环把 DeepSeek 打爆。

权威文档（必须按此实现，不要另起方案）：

- `.trellis/tasks/09-04-prompt-audit-user-scan/prd.md`
- `.trellis/tasks/09-04-prompt-audit-user-scan/design.md`
- `.trellis/tasks/09-04-prompt-audit-user-scan/implement.md`
- `.trellis/tasks/09-04-prompt-audit-user-scan/research/production-evidence.md`（生产证据，不要改口径）

状态：`planning`，产物齐全。**用户已于 2026-09-04 审核通过**（含后续补充：落库不得保存非用户提示词）。`implement.md` 里若仍写「等待审核 / 不要 start」，以本交接提示词为准：本窗口的工作是 Phase 1.3 → 1.4 → 2.1 → 2.2，不是 Phase 1.1。

## 启动顺序（先做这些，再写代码）

1. 用 Read 读仓库根目录 `AGENTS.md` 全文，并遵守其中全部规则。涉及 `web/` 时再读 `web/AGENTS.md`。
2. 读 `.agents/skills/trellis-start/SKILL.md`，执行：
   ```bash
   python3 ./.trellis/scripts/get_context.py
   python3 ./.trellis/scripts/get_context.py --mode phase
   python3 ./.trellis/scripts/get_context.py --mode packages
   ```
3. 读本任务 `prd.md` / `design.md` / `implement.md` 全文，以及 `.agents/skills/trellis-before-dev/SKILL.md`，按 skill 加载相关 spec。
4. **Phase 1.3**：整理 jsonl。当前 `implement.jsonl` 误含业务代码路径，必须清掉。implement.jsonl / check.jsonl 只保留 spec/research，并删除 `_example` 行。至少覆盖：
   - 本任务 `prd.md`、`design.md`、`implement.md`
   - 本任务 `research/production-evidence.md`
   - `.trellis/spec/guides/cross-layer-thinking-guide.md`
   - `.trellis/spec/guides/code-reuse-thinking-guide.md`
   - `AGENTS.md`
   - **不要**把即将修改的业务代码路径写进 jsonl（不要 `service/promptaudit/*.go`、不要 `web/src/...`）
   ```bash
   python3 ./.trellis/scripts/task.py add-context 09-04-prompt-audit-user-scan implement "<path>" "<reason>"
   python3 ./.trellis/scripts/task.py add-context 09-04-prompt-audit-user-scan check "<path>" "<reason>"
   python3 ./.trellis/scripts/task.py validate 09-04-prompt-audit-user-scan
   ```
5. **Phase 1.4**：方案已批准，直接启动，不要再向用户确认：
   ```bash
   python3 ./.trellis/scripts/task.py start 09-04-prompt-audit-user-scan
   python3 ./.trellis/scripts/task.py current --source
   ```
   必须确认 current 指向 `.trellis/tasks/09-04-prompt-audit-user-scan`，status 变为 `in_progress`。
6. 读 `.agents/skills/trellis-before-dev/SKILL.md` 并执行完，再改代码。
7. 按当前平台的 Trellis 2.1 实现：可 inline 自己写，或 `spawn_subagent`（`trellis-implement`）。dispatch 时 prompt **必须以** `Active task: .trellis/tasks/09-04-prompt-audit-user-scan` 开头，并声明自己已经是 implement 代理，禁止再套一层 implement/check。
8. 实现完成后读 `.agents/skills/trellis-check/SKILL.md` 做质量检查；前端文案读 `.agents/skills/i18n-translate/SKILL.md`。
9. 不要 `git commit` / `git push`，除非用户在本窗口明确要求。不要 `task.py archive`。不要 SSH 改生产配置、不要重新开启生产审计。

## 已冻结的产品决定（禁止改口径）

- 扫描范围 = **该请求全部 user 输入（含历史 user）**，不含人设。不是「只审最新一轮 user」，也不是 Codex 文案指纹。
- 用 `PromptSegment.IsUser()` / 协议 role 与 Responses `type` 排除非用户内容，**禁止**匹配 `You are Codex` 这类字符串来跳过。
- 落库 = **只保存用户提示词**。`full_prompt_ciphertext`、详情解密、`prompt_hash`、`prompt_length`、`message_count` 都只反映用户提示词。人设 / system / developer / assistant / 工具结果不得入库。
- `latest_turn_only` 保留，默认 false。开启后 **只缩小送审范围**（最新一轮 user，不附带 assistant）；落库仍是该请求**全部 user**。
- 找不到 user：**不得**退回扫描或落库人设全文。Evaluate 返回 Allow、不打 Guard HTTP；Allow/Flag 且 FullPrompt 为空则 **不 Insert 事件**（即使 StorePass 开启）。
- 进程内短缓存 Allow/Flag/Block：TTL 10 分钟，容量 4096，键含 `config_version` + scanners + `sha256(remoteScan)`。不缓存失败。不写 Redis。不缓存原文。
- 远程送审上限 `MaxRemoteScanRunes = 8000`，从 ScanText 开头截断。启发式用未截断 ScanText。落库保存未截断的全部 user。
- 失败关闭不变。失败事件必须写入真实 `latency_ms`（`Decision.LatencyMS` / `GuardError.LatencyMS`），不得恒为 0。失败密文仍只有用户提示词。
- 不新增「把人设重新纳入扫描或落库」开关。不靠加大超时 / `input_limit` 作为主修复。
- 不解密、不回写上线前已入库的历史密文（含现网那 281 条）。
- 新事件 `audit_scope` 为 `user` 或 `latest_turn`，不再写 `full`。
- 不改事件表 schema、加密算法、分组、保留期、Qwen3Guard / `llm_classifier` 请求形态。

## 明确不做

- Codex / Claude Code / Cursor 人设指纹或前缀黑名单
- 超时改 30s、`input_limit` 改十万当主方案
- 失败关闭改成超时放行
- Redis 扫描缓存；缓存里存提示词原文
- 新增「扫描或保存 system/人设」管理开关
- 抽取 function_call `arguments` / function_call_output `output` 进分段或密文
- 审计上游响应；审核图片 / 音频 / 视频二进制
- 改数据库 schema / AutoMigrate / 三库迁移矩阵
- 回写或批量清理历史密文
- SSH 生产机改审计配置或重新开启审计
- 重做 HTTP Relay / Realtime / Task Plugin 门禁接线
- 修改受保护的项目身份 / 品牌信息
- 直接改 `web/src/i18n/locales/*.json`（必须走 i18n-translate skill）
- 改 `one-api.db*` 等本地运行时数据库文件

## 实现顺序（与 implement.md 一致）

### 阶段 1：用户送审文本、落库正文、预览

- `service/promptaudit/snapshot.go`：
  - `JoinUserSegments`：全部 user、**原顺序** → `FullPrompt` / hash / length / message_count
  - `SelectUserScanSegments`：默认全部 user（最后一段 user 作 `BuildScanText` 优先段）；`latest_turn_only` 时仅最新一轮连续 user
  - `RedactedPreview = BuildPromptPreview(scanText)`，禁止再对含人设的全文做预览
  - 无 user 返回空，**禁止**再走 `SelectLatestTurnSegments` 那种「退回全文」热路径
- `service/promptaudit/types.go`：`AuditScope` 注释改为 `user` | `latest_turn`；常量 `MaxRemoteScanRunes = 8000`

验证：`go test ./service/promptaudit/ -count=1 -timeout 60s`

必须覆盖：人设+多轮 user 时 FullPrompt/ScanText 都只有 user；preview 不以 `You are Codex` 开头；`latest_turn_only=true` 时 ScanText 仅最新 user、FullPrompt 仍含全部 user；无 user 时两者为空。提取层测试仍可断言分段列表里存在 system（提取 ≠ 落库）。原先「FullPrompt 含 system」的 snapshot 断言改为「仅 user」。

### 阶段 2：Responses 角色映射

- `extractResponsesInput`：按 item `type` 映射
  - `message` / 空 type：有 role 按 role；无 role 才默认 user
  - `function_call` / `reasoning` → assistant，非 user
  - `function_call_output` → tool，非 user
- **不要**抽取 `output` / `arguments` 正文
- 夹具：`instructions`（Codex 人设）+ user message + 无 role 的 `function_call_output`

验证：ScanText 与 FullPrompt 只有 user；工具输出不出现在解密正文。

### 阶段 3：Evaluate 短路、空事件跳过、截断、缓存、耗时

- `guard.go`：空 ScanText → Allow 且零次 Scanner HTTP；启发式用未截断 ScanText；远程用截断文本；缓存查找在 `globalSem` 之前
- 缓存：进程内，TTL 10m，容量 4096，可替换时钟，**禁止 Sleep 测 TTL**
- `event_store.go` / `gate.go`：Allow/Flag 且 FullPrompt 为空 → 不 Insert；失败拷贝 `LatencyMS`；只加密 FullPrompt（此时已是用户文本）
- `Decision.LatencyMS`、`GuardError.LatencyMS`

验证：`go test ./service/promptaudit/ ./controller/ -count=1 -timeout 60s`

必须覆盖：空用户 Allow 后事件表 0 行；有用户且 StorePass=true 时落库解密不含人设；超 8000 rune 只按截断分片、落库仍是未截断用户文本；缓存命中不打 HTTP；config_version 变化后重打；error 不缓存；失败 `latency_ms` 非 0（注入耗时，不要真等 8s）。

### 阶段 4：前端说明与 i18n

- `web/src/features/prompt-audit/components/policy-tab.tsx`：不要再写 “Full prompt text will still be stored completely”
  - 默认：扫描并保存该请求全部用户提示词，排除客户端人设 / system / 工具结果
  - 打开「仅审计最新轮」：只把最新一轮 user 送 Guard；详情仍是该请求全部用户提示词
- 列表若展示 `audit_scope`，必须识别 `user`
- i18n：en / zh / zh-TW / fr / ru / ja / vi。**禁止**直接改 locale JSON，走 `i18n-translate` skill（`add-missing-keys.mjs` + `bun run i18n:sync`）
- 更新 `web/src/features/prompt-audit/__tests__/prompt-audit-page.test.tsx`

验证：

```bash
cd web && bun run i18n:sync
cd web && bun run build
```

若可跑：`cd web && bun run test src/features/prompt-audit`（以 `web/package.json` 为准）。

### 阶段 5：回归

```bash
go test ./service/promptaudit/ ./controller/ -count=1 -timeout 60s
cd relaykit && GOWORK=off go build ./...
```

本任务不应改 `relaykit/`。无 schema 变更，不做三数据库矩阵。不要改 `one-api.db*`。

## 工程约束（本仓库硬规则）

- JSON 只走 `common.Marshal` / `Unmarshal` / `UnmarshalJsonStr` / `DecodeJson`，业务代码禁止直接 `encoding/json` marshal/unmarshal
- 不要为单调用方抽无稳定领域含义的包级 helper；`JoinUserSegments` / `SelectUserScanSegments` 是稳定领域概念，可以单独成函数
- 新测试用 `testify/require` 做 setup/fatal，`assert` 做非致命比较；单测超时不超过 60s
- 不要加随机输入、Sleep、只刷覆盖率的测试
- 前端包管理用 bun
- 代码注释用中文，只注释非显而易见的约束
- 不要改无关文件，不要顺手重构 Qwen3Guard / LLM 分类器请求路径
- 日志不得包含 ScanText、FullPrompt、Token、完整 Guard 响应

## 风险文件（改前先读）

- `service/promptaudit/snapshot.go`：送审与落库正文；改错会把人设再次送审或入库
- `service/promptaudit/extract_relay.go`：空 role 默认 user 会把 `function_call_output` 当用户提示词
- `service/promptaudit/guard.go`：超时、成本、隔离舱；缓存必须放在 `globalSem` 之前
- `service/promptaudit/event_store.go`：只加密 FullPrompt；空 Allow 必须跳过 Insert
- `service/promptaudit/gate.go`：失败落库必须带耗时
- `web/src/features/prompt-audit/components/policy-tab.tsx`：管理员对「保存什么」的理解

## 完成标准

对照 `prd.md` Acceptance Criteria AC1–AC12 逐项给出证据（测试名或命令输出）。未跑的检查如实说，不要声称“已验证”。完成后停在实现+自检，等待用户决定是否 commit / archive。
