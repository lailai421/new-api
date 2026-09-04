# Cyber Abuse 拦截实施计划

## 实施边界

实现 `prd.md` 与 `design.md` 的完整范围。不得改坏 Qwen3Guard 请求形态，不得把审核请求接入业务 Relay，不得把分类 prompt 做成可编辑配置，不得新增第三种 Guard 协议，不得手改 `web/src/i18n/locales/*.json`。

用户已于 2026-09-04 批准本方案。新窗口按 `handoff-prompt.md` 执行 `task.py start` 并实现，不要重新规划。

## 依赖

父任务 `09-03-prompt-audit` 的门禁与 `09-04-prompt-audit-llm-classifier` 的 LLM 分类器已经存在。本任务在其之上增量修改。

实施顺序不可再拆子任务：目录、判决、prompt、启发式、前端是同一可验证交付。

回滚：关掉审计或取消勾选 `cyber_abuse` 即恢复旧拦截面。若需回滚二进制到不认识该 id 的版本，先保存不含 `cyber_abuse` 的配置。

## 阶段 1：目录、升级规则、配置并入

### 目标

第十类成为一等 scanner，旧九类全选配置自动获得保护。

### 主要改动

- `qwen3guard.go`：`AllScannerIDs`、`ScannerCatalog`、`categoryAliases`、`isElevatedControversial`
- `types.go`：`ScannerDefinition.DescriptionZH`
- `qwen3guard.go` 的十类 `DescriptionZH` 一并补齐
- `config.go`：`legacyDefaultScannerIDs` 与 Normalize 并入逻辑
- `config.go` 注释“九类”改为“默认全部分类”

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s
```

补：默认配置含 `cyber_abuse`；旧九类全集 Normalize 后含新类；自定义子集不变；Controversial `cyber_abuse` Block；禁用后仅该类 Unsafe → Warn；`Categories: malware` 映射到 `cyber_abuse`。

## 阶段 2：LLM 固定 prompt

### 目标

分类器被明确要求标 Cyber Abuse，且不把设备越狱标成 `jailbreak`。

### 主要改动

- `llm_classifier.go` 的 `LLMClassifierSystemPrompt` 按 `design.md` 第 6 节扩展
- `llm_classifier_test.go`：prompt 锚点 + JSON 判决样例

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s -run 'LLMClassifier|ParseLLM'
```

断言 prompt 含 `cyber_abuse`、malware/C2/credential/reverse/crack，以及 device jailbreak ≠ `jailbreak`。解析 `{"safety":"Unsafe","categories":["cyber_abuse"]}` 为 Block。

现有 JSON / 围栏 / 非法响应测试继续通过。

## 阶段 3：本地启发式前置

### 目标

直球样本不依赖 Guard 模型即可 Block，且不误伤普通编程。

### 主要改动

- 新增 `cyber_abuse.go` / `cyber_abuse_test.go`
- `guard.go` 的 `Evaluate`：审计启用且 scanners 含 `cyber_abuse` 时先跑全文启发式；命中则跳过远程 Guard，仍走事件持久化

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s
```

正例：mimikatz、注册机、免杀、steal cookies、c2 beacon、license keygen。  
反例：React hook、JSON payload、reverse proxy、git revert、crack the interview。  
启发式 Block 路径：不调用 Scanner HTTP（用 mock evaluator/scanner 断言零次远程调用）。

## 阶段 4：前端与 i18n

### 目标

管理员能看见、勾选、理解限制。

### 主要改动

- `web/src/features/prompt-audit/constants.ts`
- `policy-tab.tsx`：仅 Qwen3Guard 时的警告；Cyber Abuse 能力说明
- i18n：按 `.agents/skills/i18n-translate/SKILL.md`，用 `add-missing-keys.mjs` + `bun run i18n:sync`，禁止手改 locale json

### 验证

```bash
cd web && bun run i18n:sync
cd web && bun run build
```

策略页单测若依赖默认 scanners，更新期望列表。

## 阶段 5：回归

```bash
go test ./service/promptaudit/ ./controller/ -count=1 -timeout 60s
cd web && bun run build
```

`relaykit` 本任务不应改动。若误改：`cd relaykit && GOWORK=off go build ./...`

无 schema 变更，不做三数据库矩阵。

## 风险文件

- `service/promptaudit/llm_classifier.go`：prompt 改坏会导致分类器乱输出 → 503 失败关闭
- `service/promptaudit/guard.go`：启发式短路若绕过事件写入，会丢审计记录
- `service/promptaudit/config.go`：并入逻辑若写错，会给自定义子集强加新类
- `web/src/i18n/locales/*.json`：禁止直接编辑

## `task.py start` 前检查

- [x] `prd.md` / `design.md` / `implement.md` 已写
- [x] `research/openai-cyber-abuse.md` 已写
- [x] 用户已审核并明确批准本方案（含高召回误伤合法安全研究）
- [x] `handoff-prompt.md` 已写，供新窗口启动
- [ ] 新窗口执行 `task.py start`，然后按 `trellis-before-dev` 写代码
