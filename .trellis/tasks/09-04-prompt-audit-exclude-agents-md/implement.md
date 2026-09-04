# 提示词审计排除 AGENTS.md — 实施计划

## 实施边界

实现 `prd.md` 与 `design.md` 的完整范围。不得写「You are Codex」人设指纹，不得按仓库 AGENTS.md 正文匹配，不得改失败关闭，不得新增配置项，不得改事件表，不得回写历史密文，不得在本任务改生产机配置。

本轮规划完成后必须等待用户审核；未获批准前不执行 `task.py start`，不修改业务代码。

## 依赖

user-scan 已把非 user 排除。本任务只在 snapshot 热路径增加信封剥离，并修正分类器思考链匹配。

## 阶段 1：剥离 AGENTS.md 信封

### 目标

R1–R6、AC1–AC7、AC10。

### 主要改动

- `service/promptaudit/` 增加信封判定与剥离（可放在 `snapshot.go` 或与 snapshot 同包的直接函数，避免单调用无意义的包级机械拆分）。
- `BuildPromptSnapshot` 在 `JoinUserSegments` / `SelectUserScanSegments` 之前调用剥离。
- 回归：Responses 提取测试仍可断言分段列表里存在信封；snapshot / Evaluate 断言 ScanText 与 FullPrompt 不含信封。

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s
```

必须覆盖：

- 人设 + AGENTS.md 信封 + `你好`：ScanText/FullPrompt 只有 `你好`
- 预览不以 `# AGENTS.md instructions` 开头
- 混合段：信封后跟真实文本，只保留真实文本
- 只有信封：不 HTTP、不写事件
- 用户提到 AGENTS.md 但无信封：整段保留
- 标题生成请求保留
- `latest_turn_only` 只作用于真实 user
- 替换/移除通知信封被丢掉

## 阶段 2：修复思考链禁用

### 目标

R7、AC8。

### 主要改动

- `llm_classifier.go`：按去掉 provider 前缀后的模型名判断 DeepSeek V4；保留 `-none` 剥离与 `deepseek.com` BaseURL。
- `llm_classifier_test.go`：增加 `deepseek/deepseek-v4-flash`、`deepseek/deepseek-v4-flash-none`；保留原无前缀用例；断言 `gpt-4o` 无 `thinking`。

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s -run 'TestLLMClassifier|Thinking|deepseek'
```

## 阶段 3：前端说明与 i18n

### 目标

R8、AC9。

### 主要改动

- `web/src/features/prompt-audit/components/policy-tab.tsx` 与对应测试：`latest_turn_only` 说明写明排除 AGENTS.md 信封。
- 更新 `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`。

### 验证

```bash
cd web && bun run i18n:sync
# 按 web/AGENTS.md 跑相关前端测试
```

## 阶段 4：全量回归

```bash
go test ./service/promptaudit/ ./controller/ ./model/ -count=1 -timeout 60s
```

不改数据库 schema，不跑三库迁移矩阵。若未改 model 层，三库验证不适用。

## 风险文件

- `service/promptaudit/snapshot.go`：热路径，剥离顺序错误会再次把信封送进 Guard。
- `service/promptaudit/llm_classifier.go`：匹配过宽会误伤非 V4 模型。
- 前端 i18n 源键：改英文 key 时必须同步全部语言文件。

## 回滚

还原上述文件即可。已按新规则写入的短正文事件可保留。

## `task.py start` 前检查

- [ ] 用户已审核并明确批准本方案
- [ ] `prd.md` / `design.md` / `implement.md` 与 jsonl 已齐
- [ ] 不在批准前改业务代码
