# 提示词审计只送审真实用户输入并合并重复送审 — 技术设计

## 1. 设计目标

1. 在 user-scan 与 AGENTS.md 剥离之后，再去掉 Codex 环境上下文和其它带标记的自动 user 片段，并从标题生成模板中抽出真实用户提示词。
2. 同一过滤后送审文本：并发合并为一次远程 Guard，完成后走现有 10 分钟判定缓存；Allow/Flag 缓存命中不再写事件。

不改门禁接线、失败关闭、加密算法、事件表 schema。不拦截上游模型转发。

## 2. 边界

| 保持不变 | 本任务改变 |
|---|---|
| 同步阻断、分组、失败关闭 | 真实 user 不含 `<environment_context>` 等标记块 |
| 事件表、AES-GCM | 标题生成只保留 `User prompt:` 后正文 |
| `latest_turn_only` 语义 | 并发 singleflight + Allow/Flag 缓存命中不写事件 |
| 无真实 user → Allow 且不写事件 | ScanText / FullPrompt / 预览 / hash / 长度 / 条数按过滤后文本 |
| 判定缓存 TTL 10 分钟、不缓存失败 | |
| Qwen3Guard / LLM 请求形态 | 前端说明点名环境上下文与重复送审合并 |
| 历史已入库密文 | |

不新增 Option 字段。不迁移旧行。

## 3. 数据流

```
ExtractRelay/Realtime/Task
        │
        ▼
  []PromptSegment
        │
        ▼
  StripAgentsMdEnvelopes          （已有）
        │
        ▼
  StripCodexContextualUser        （本任务：成对标记块）
        │
        ▼
  UnwrapCodexThreadTitle          （本任务：标题模板 → User prompt 正文）
        │
        ├─ JoinUserSegments → FullPrompt / hash / length / count / 密文
        └─ SelectUserScanSegments → ScanText / preview
                    │
                    ▼
              GuardEvaluator.Evaluate
                    ├─ ScanText 空 → Allow，不 HTTP
                    ├─ 启发式（未截断 ScanText）
                    ├─ 截断 remoteScan
                    ├─ DecisionCache.Get
                    │     hit → FromCache=true，不 HTTP
                    └─ miss → singleflight.Do(cacheKey)
                          ├─ 双检缓存
                          ├─ 远程分片扫描
                          └─ Put 缓存（仅 Allow/Flag/Block）
                    │
                    ▼
              gate Record
                    ├─ Allow/Flag && FromCache → 不写事件
                    ├─ Allow/Flag && !FromCache && StorePassEvents → 写一条
                    └─ Block / 失败 → 每次都写
```

提取层仍可看到环境上下文，便于单测断言「提取到了但不送审」。热路径仍在 `BuildPromptSnapshot`，Realtime / Task 自动生效。

## 4. 过滤契约

### 4.1 成对标记

对每个 **user** 段，按 Codex `matches_marked_text` 语义处理：

- trim 后整段以开标签起头、以对应闭标签结尾 → 丢弃整段
- 否则删除文本中每一段完整 `open…close` 闭区间（含标签），再 Trim；剩余为空则丢弃

开闭标签（字面量，ASCII 大小写不敏感）：

- `<environment_context>` / `</environment_context>`
- `<skills_instructions>` / `</skills_instructions>`
- `<plugins_instructions>` / `</plugins_instructions>`
- `<user_shell_command>` / `</user_shell_command>`
- `<turn_aborted>` / `</turn_aborted>`

AGENTS.md 继续走现有函数，不要把「工程师工作规范」当指纹。

用户正文里出现这些英文词但没有成对标签时，整段保留。

### 4.2 标题生成

在标记剥离之后处理。满足：

1. `strings.TrimSpace(text)` 以 `Generate a concise, single-line task title of at most ` 为前缀
2. 存在独立行 `User prompt:`（允许 `\nUser prompt:\n`）

则 Scan/落库只取该标签之后的 Trim 文本。否则保持原样。

剥完后，标题请求与主对话的 `ScanText` 应相同，从而共享缓存键。

### 4.3 空结果

过滤后无 user：`ScanText=""`、`FullPrompt=""`，Evaluate 不 HTTP，Allow 不写事件。

## 5. 重复送审合并

### 5.1 缓存键

沿用 `computeDecisionCacheKey(configVersion, scanners, remoteScan)`。`remoteScan` 是过滤、截断后的送审文本。缓存仍只存 Decision，不存原文。

### 5.2 singleflight

`GuardEvaluator` 增加 `golang.org/x/sync/singleflight.Group`（`go.mod` 已有 `golang.org/x/sync v0.20.0`）。

缓存未命中时用 `cacheKey` 做 `Do`：

- 函数体内再 Get 一次，避免 leader 写入后跟随者重入远程
- 只把 Allow / Flag / Block 放入 DecisionCache
- 失败不入缓存；同飞跟随者共享这次错误（只付一次 Guard 费用），后续新请求再试

返回的 Decision 必须 `cloneDecision`，避免共享可变指针。

### 5.3 FromCache

`Decision` 增加内存字段 `FromCache bool`（不落库）：

- `cache.Get` 命中：`true`
- singleflight 跟随者：`true`
- 远程或启发式首次得出：`false`

`CheckRelayRequest` / Midjourney / Realtime / Task 的 Allow/Flag 落库：`FromCache == true` 时跳过 `Record`。Block 忽略该字段，每次都写。

空 ScanText 的 Allow 仍走现有「不写事件」，与 FromCache 无关。

## 6. 与前置任务的关系

- user-scan：继续只看 `IsUser()`。本任务保证进入 Join/Select 的 user 段已无自动片段。
- exclude-agents-md：AGENTS.md 逻辑不动。AC7「标题生成整段送审」由本任务覆盖为「只送 User prompt 正文」。
- cyber_abuse 启发式仍用未截断 ScanText；剥掉环境上下文后只对真实用户句匹配，这是期望行为。

## 7. 前端

更新 `latest_turn_only` 说明：排除环境上下文等 Codex 自动片段；同一真实提示词短时间内只远程送审一次。英文源键更新后跑 `bun run i18n:sync`，补 zh / zh-TW / fr / ru / ja / vi。

## 8. 兼容与回滚

- 普通聊天补全、Claude、Gemini 的纯 user 不受影响。
- 用户讨论「环境上下文」但未带标签的文本仍送审。
- 回滚：还原剥离、标题展开、singleflight 与 FromCache 跳过写入即可；已按新规则写入的事件无需回迁。
- 旧密文含环境上下文的行不改，管理员用现有删除/保留期处理。

## 9. 风险

| 风险 | 处理 |
|---|---|
| Codex 以后改标题模板前缀 | 识别失败时整段仍当 user 送审，可能再多一次 Guard；不静默丢用户句 |
| 用户故意发送完整 `<environment_context>` 块 | 按自动片段丢弃；若同段还有其它文字则保留其它文字 |
| 误把上游 6 次模型调用当成审计 | 方案明确不拦截转发；验收只数 Guard HTTP 与事件行 |
| singleflight 共享 503 | 可接受：同飞只付一次失败费用，下一请求重试 |
