# 提示词审计只扫描用户输入 — 技术设计

## 1. 设计目标

在不改动门禁接线、失败关闭和加密算法的前提下：

1. 远程 Guard 只收到本请求的用户提示词。
2. `full_prompt_ciphertext` 只保存用户提示词，不再保存 Codex 人设、system、assistant、工具结果。
3. 修复预览用人设开头、`latest_turn_only` 退回全文、失败事件 `latency_ms=0`。

## 2. 边界

| 保持不变 | 本任务改变 |
|---|---|
| 同步阻断、分组、失败关闭 | `ScanText` 默认只含 user 分段 |
| 事件表 schema、AES-GCM 加密 | `FullPrompt` / 密文 / hash / 长度 / 条数只含 user |
| `latest_turn_only` 配置项与默认 false | `redacted_preview` 来自 ScanText |
| Guard 协议、节点、Probe | 无 user 的 Allow 不落库、不退回全文 |
| Qwen3Guard / LLM 分类器请求形态 | 进程内送审文本判定缓存 |
| 本地 cyber_abuse 启发式入口 | 远程送审 rune 上限与截断 |
| 历史已入库密文 | 失败 Decision 写入真实耗时 |
| | Responses item type 角色映射 |

不新增 Option 字段。不改 `PromptAuditConfigSecret` JSON 形状。不迁移旧行。

## 3. 数据流

```
ExtractRelay/Realtime/Task
        │
        ▼
  []PromptSegment
        │
        ├─ JoinUserSegments（全部 user，原顺序）
        │     → FullPrompt / PromptHash / PromptLength / MessageCount
        │     → 加密落库（仅此正文）
        │
        └─ SelectUserScanSegments(latestTurnOnly)
              → ScanText / audit_scope / RedactedPreview(ScanText)
                    │
                    ▼
              GuardEvaluator.Evaluate
                    ├─ 本地启发式（未截断 ScanText）
                    ├─ ScanText 为空 → Allow，不打 HTTP，不写事件
                    ├─ 截断到 MaxRemoteScanRunes
                    ├─ 缓存查找（config_version + scanners + sha256(remoteScan)）
                    └─ 未命中 → 现有分片 + Scanner HTTP → 写入缓存（仅 Allow/Flag/Block）
```

`PromptHash` 对 **落库的用户提示词**（全部 user 拼接）计算，不再对人设整包计算。缓存键仍对 **实际送远程的文本** 计算（可能因 `latest_turn_only` 或截断而更短）。

## 4. 送审与落库契约

### 4.1 用户分段

`PromptSegment.IsUser()`：`User==true` 或 `role=="user"`。

ScanText 与 FullPrompt **都只**使用这些分段。

非 user（不得进入 ScanText，也不得进入 FullPrompt / 密文）：

- `system` / `developer` / `assistant` / `tool` / `model`
- Responses 顶层 `instructions`
- Responses `type=function_call` / `function_call_output` / `reasoning`

内存里可以为了角色判断而看到非 user 项，但 `JoinUserSegments` 之后不得把它们拼进 `snapshot.FullPrompt`。`GormEventStore.Record` 只加密 `snapshot.FullPrompt`，因此人设不会入库。

### 4.2 范围

| `latest_turn_only` | `audit_scope` | ScanText | 落库 FullPrompt |
|---|---|---|---|
| false（默认） | `user` | 全部 user；最后一段 user 作为 `BuildScanText` 优先段 | 全部 user，**原顺序**拼接 |
| true | `latest_turn` | 仅最新一轮连续 user，不附带 assistant | 仍为该请求**全部 user**（原顺序） |

找不到任何 user 时：`ScanText=""`，`FullPrompt=""`，`RedactedPreview=""`，不退回人设全文，不写事件。

落库始终保存该请求全部 user，是为了详情能看到多轮用户原话；user 文本相对人设很小。`latest_turn_only` 只减少 DeepSeek 输入，不靠丢掉历史 user 来省存储。

现有 `SelectLatestTurnSegments`（`snapshot.go:89`）在无 user 时退回全部分段。热路径改走 `SelectUserScanSegments` / `JoinUserSegments`。

### 4.3 预览与长度

- `RedactedPreview = BuildPromptPreview(scanText)`
- `PromptLength = utf8.RuneCountInString(FullPrompt)`（用户提示词）
- `MessageCount = 用户分段数`
- 详情接口解密后不得出现 `You are Codex` 人设（除非用户自己输入了这句话）

### 4.4 远程截断

`MaxRemoteScanRunes = 8000`。

- 启发式：未截断 ScanText
- 远程：截断后的 remoteScan
- 落库：未截断的全部 user FullPrompt
- 截断从 ScanText 开头切

### 4.5 空用户文本

Evaluate：ScanText 为空 → Allow，不 HTTP。

Record / Gate：Allow 或 Flag 且 FullPrompt 为空 → 直接返回，不 Insert。Block / Unavailable / Invalid 若发生，仍写元数据；正文为空字符串密文。正常无 user 路径不会走到 Block。

## 5. Responses 提取修正

`extractResponsesInput`（`extract_relay.go:479`）在 `role==""` 时默认 `RoleUser`。必须按 type 映射，避免工具输出被当成用户提示词送审并落库。

| item `type` | 角色 | User | 是否抽取文本 |
|---|---|---|---|
| `message` 或空，且 `role` 有值 | 按 role | role==user | 仅 user 的 content/text 进入后续 JoinUser |
| `message` 或空，且 `role` 为空 | user | true | 是（兼容纯文本 input） |
| `function_call` / `reasoning` | assistant | false | **不抽取** `arguments` / reasoning 正文 |
| `function_call_output` | tool | false | **不抽取** `output` 正文 |

不抽取工具全文，避免为了「完整性」把 cat 出来的文件打进内存和密文。只需保证这些项不能被标成 user。

Chat Completions / Claude / Gemini 已按 role 提取；snapshot 过滤后 system 不再进入 FullPrompt。回归测试从「FullPrompt 含 system」改为「FullPrompt 仅 user」。

## 6. 判定缓存

进程内，TTL 10 分钟，容量 4096。

```
key = sha256( strconv(config_version) + "\x1f" + sorted_scanners + "\x1f" + remoteScan )
value = Decision 深拷贝（Allow / Flag / Block）
```

不缓存失败。查找在启发式之后、`globalSem` 之前。不写 Redis，不缓存原文。测试禁止 Sleep。

## 7. 失败耗时落库

- `Decision.LatencyMS`、`GuardError.LatencyMS`
- Evaluate 失败路径写入 `time.Since(start)`
- `gate.go` 拷贝到失败 Decision
- `GormEventStore.Record`：优先 `Result.LatencyMS`，否则 `Decision.LatencyMS`
- 失败事件的密文仍是用户提示词，不是人设

不改表结构。

## 8. 前端

`latest_turn_only` 说明改为：

1. 默认扫描并**保存**该请求全部用户提示词，排除客户端人设 / system / 工具结果
2. 打开此开关后只把最新一轮 user 送给 Guard；详情里仍是该请求全部用户提示词

不要再写 “Full prompt text will still be stored completely”（会被理解成整包请求入库）。

事件列表若展示 `audit_scope`，识别 `user`。i18n 覆盖 en / zh / zh-TW / fr / ru / ja / vi。

## 9. 兼容与滚动

- 本任务上线前的事件密文可能仍含人设（例如现网 281 条）。不自动解密回写。管理员用现有删除或保留期清理。
- 新事件 `audit_scope` 为 `user` 或 `latest_turn`，不再写 `full`。
- 加载新代码后立即按新规则送审和落库，无需改配置。
- 回滚镜像会恢复「全文送审 + 整包入库」。无 schema 回滚。

## 10. 风险

| 风险 | 处理 |
|---|---|
| 详情看不到 Codex 人设，难以复盘「模型当时的系统提示」 | 接受。本功能审计的是用户意图，不是客户端说明书。人设重复且巨大，正是存储顶不住的原因。 |
| 恶意内容只在工具读到的文件里 | 不审、不存工具输出。用户原话仍在。 |
| 超长用户粘贴 | 远程截断到 8000 rune；落库仍保存完整用户文本（父任务已要求超 64KiB 可存）。 |
| 旧密文仍占空间 | 本任务不迁移；用删除/保留期。 |
| 工具循环仍带历史 user，会重复落库同一段用户话 | 用户文本远小于人设。可用 hash 观察重复；本任务不做事件去重写库。判定缓存已避免重复打 DeepSeek。 |

## 11. 关键代码锚点

- FullPrompt 现为 `JoinSegments` 全文：`service/promptaudit/snapshot.go:232-349`
- 无 user 退回全文：`service/promptaudit/snapshot.go:105-107`
- 加密对象：`service/promptaudit/event_store.go:59-67`
- Responses instructions：`service/promptaudit/extract_relay.go:399-437`
- 空 role 默认 user：`service/promptaudit/extract_relay.go:498-515`
- 失败不写耗时：`service/promptaudit/gate.go:64-75`、`event_store.go:87-101`
- 启发式与分片：`service/promptaudit/guard.go:385-466`
- 开关文案：`web/src/features/prompt-audit/components/policy-tab.tsx:238-265`
