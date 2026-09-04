# 提示词审计只送审真实用户输入并合并重复送审 — 实施计划

## 实施边界

实现 `prd.md` 与 `design.md` 的完整范围。不得写「You are Codex」人设指纹，不得按仓库正文匹配，不得拦截上游转发，不得改失败关闭，不得新增配置项，不得改事件表，不得回写历史密文，不得在本任务改生产机配置。

本轮规划完成后必须等待用户审核；未获批准前不执行 `task.py start`，不修改业务代码。

## 依赖

user-scan 与 AGENTS.md 剥离已在热路径。本任务只在 snapshot 增加上下文/标题过滤，并在 Evaluator / gate 增加并发合并与 Allow 缓存命中跳过写事件。

## 阶段 1：剥离环境上下文与其它标记块，展开标题模板

### 目标

R1–R6、AC1–AC5、AC10、AC12 中与过滤相关的部分。

### 主要改动

- `service/promptaudit/`：在 `BuildPromptSnapshot` 里，`StripAgentsMdEnvelopes` 之后对 user 段做成对标记剥离，再展开标题生成模板。
- 成对标记与标题前缀写在 snapshot 同包的直接函数里，避免单调用无意义的包级机械拆分。
- 回归：提取测试仍可断言分段列表里存在 `<environment_context>`；snapshot / Evaluate 断言 ScanText 与 FullPrompt 只有真实 user。

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s
```

必须覆盖：

- 环境上下文段 + `你是什么模型？`：ScanText/FullPrompt 只有后者；预览不含 `<environment_context>`
- 混合段：标记块后跟真实文本，只保留真实文本
- 只有环境上下文：不 HTTP、不写事件
- 标题生成模板抽出 `User prompt:` 正文
- 用户提到环境/标题但无标记：整段保留
- AGENTS.md 剥离仍然生效
- `latest_turn_only` 只作用于真实 user

## 阶段 2：并发合并与 Allow 缓存命中不写事件

### 目标

R7–R9、AC6–AC9、AC8。

### 主要改动

- `GuardEvaluator`：缓存未命中时用 `singleflight.Group` 包远程扫描；`Decision.FromCache` 标记命中与跟随者。
- `gate.go`（HTTP / Midjourney / Realtime / Task）：Allow/Flag 且 `FromCache` 时不 `Record`；Block 每次 `Record`。
- 现有 TTL、不缓存失败、不存原文保持不变。

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s -run 'TestUserScan_DecisionCache|Test.*Singleflight|Test.*FromCache|Test.*Title|Test.*Environment'
```

必须覆盖：

- 连续 6 次相同 ScanText：Guard HTTP = 1，Allow 事件 = 1
- 两个并发 Evaluate：Guard HTTP = 1
- 改 `config_version` 后重新 HTTP
- Block 缓存命中仍 403 且第二次仍写 Block 事件
- 超时/503 不得当成功缓存

## 阶段 3：前端说明与 i18n

### 目标

R11、AC11。

### 主要改动

- `web/src/features/prompt-audit/components/policy-tab.tsx` 与对应测试：说明排除环境上下文等自动片段，并写明短时间重复送审合并。
- 更新 `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`。

### 验证

```bash
cd web && bun run i18n:sync
```

按 `web/AGENTS.md` 跑相关前端测试。

## 阶段 4：全量回归

```bash
go test ./service/promptaudit/ ./controller/ ./model/ -count=1 -timeout 60s
```

手工对照验收（批准实施后）：用 Codex 再发「你是什么模型？」——事件预览应只有这句；同一窗口远程 Guard 应为 1 次 Allow 事件；上游仍可多轮转发。

## 风险文件

- `service/promptaudit/snapshot.go` — 过滤热路径
- `service/promptaudit/guard.go` — 缓存与 singleflight
- `service/promptaudit/gate.go` / `gate_realtime.go` — 写事件
- `service/promptaudit/types.go` — `FromCache` 内存字段
- `web/src/features/prompt-audit/components/policy-tab.tsx` — 文案

## 回滚点

还原上述文件即可。已按新规则写入的短事件无需回迁。不要在失败关闭路径上「为了省钱而放行」。
