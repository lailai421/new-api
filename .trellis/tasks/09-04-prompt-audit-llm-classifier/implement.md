# LLM 分类器实施计划

## 实施边界

实现 `prd.md` 与 `design.md` 的完整范围。不得删除或改坏 Qwen3Guard 路径，不得把审核请求接入业务 Relay，不得把分类提示词做成可编辑配置，不得引入 Moderations / 阿里云护栏 / 渠道选择器。

本轮规划完成后必须等待用户审核；未获批准前不执行 `task.py start`，不修改业务代码。

## 依赖

父任务 `09-03-prompt-audit` 的领域核心、存储 API、门禁接线和管理页已经存在。本任务在其之上增量修改，不重做门禁。

实施顺序不可并行拆成多个子任务：协议常量、Scanner、Probe、前端是同一可验证交付。

## 阶段 1：共享判决与协议常量

### 目标

把 Qwen3Guard 的判决规则抽成可复用函数，并允许配置里出现 `llm_classifier`。

### 主要改动

- `service/promptaudit/types.go`：新增 `ProtocolLLMClassifier = "llm_classifier"`，以及 LLM 默认超时 / `max_tokens` 常量。
- `service/promptaudit/qwen3guard.go`：抽出 `ApplySafetyDecision`；`ParseQwen3Guard` 改为解析后调用它。
- `service/promptaudit/config.go`：protocol 白名单包含两种协议；`llm_classifier` 且 model 为空时失败，不回填 Qwen3Guard 默认模型。
- `dto/prompt_audit.go`：Probe 请求增加 `protocol`。

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s
```

现有 `qwen3guard_test.go`、`config_test.go` 必须继续通过。补：空协议归一、非法协议拒绝、`llm_classifier` 空模型拒绝。

## 阶段 2：LLM Classifier Scanner

### 目标

独立实现分类请求、JSON 解析、文本降级和协议分发。

### 主要改动

- 新增 `service/promptaudit/llm_classifier.go`（prompt 常量、Scan、JSON 提取）。
- 新增 `service/promptaudit/scanner_dispatch.go`。
- `service/promptaudit/init.go` / `NewGuardEvaluator`：默认注入 `DispatchScanner`。
- `OpenAICompatibleScanner` 请求体保持 `user-only` + `max_tokens=64`。

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s
```

必须覆盖：

- 请求含 system prompt 与 `<<<BEGIN_UNTRUSTED_CONTENT>>>`
- JSON 判决与 `ParseQwen3Guard` 同输入语义一致
- markdown 围栏 JSON
- JSON 失败后的文本降级
- 双失败 → `prompt_guard_invalid_response` 且 Retryable=false
- 注入样本（忽略指令、强制输出 Safe）不得在测试夹具里被解析为无 jailbreak 的 Safe；对 Scanner 用假上游返回可控 JSON，prompt 构造测试断言 user 消息被分隔。抗注入的真实模型效果不在单测里用随机闲聊冒充。
- 分发器按 protocol 选择请求形态

## 阶段 3：Probe 与管理 API

### 目标

探测走真实协议。

### 主要改动

- `controller/prompt_audit.go` 的 `ProbePromptAuditEndpoint`：尊重节点或请求中的 protocol，走 `DispatchScanner`。
- `controller/prompt_audit_test.go`：LLM 协议探测成功/失败；确认强制 Qwen3Guard 的旧行为已消失。

### 验证

```bash
go test ./controller/ -count=1 -timeout 60s -run PromptAudit
```

## 阶段 4：前端协议选择与 i18n

### 目标

管理员能创建、探测、保存 `llm_classifier` 节点。

### 主要改动

- `web/src/features/prompt-audit/constants.ts`：第二协议与默认超时/占位。
- `web/src/features/prompt-audit/lib/schema.ts`：protocol 枚举；`createDefaultEndpoint` 保持 Qwen3Guard 默认，另提供 LLM 默认工厂或按协议生成。
- `web/src/features/prompt-audit/components/endpoints-tab.tsx`：协议 Select、说明、Probe 带 protocol。
- 七种语言 locale。
- 现有 schema / page 测试更新。

### 验证

```bash
cd web && bun run test -- src/features/prompt-audit
cd web && bun run i18n:sync
cd web && bun run build
```

按仓库当前前端脚本为准；若测试命令不同，以 `web/package.json` 为准。

## 阶段 5：回归与说明

### 目标

确认门禁接线无需改动即可消费新 Scanner；文档只在管理界面完成，不另开用户手册任务。

### 验证

```bash
go test ./service/promptaudit/ ./controller/ -count=1 -timeout 60s
```

抽查 `controller/relay.go`、Realtime、Task Plugin 仍只调用 `Evaluator`，无协议硬编码。

无数据库 schema 变更，不做 SQLite / MySQL / PostgreSQL 迁移矩阵。若实施中误改 GORM 模型，必须补三库验证并在 PR 记录版本与命令。

## 风险文件

- `service/promptaudit/guard.go`：不要把 Qwen3Guard 请求改成带 prompt。
- `service/promptaudit/qwen3guard.go`：抽判决时保持测试黄金向量。
- `service/promptaudit/config.go`：错误拒绝旧配置会导致开启审计后 degraded。
- `controller/prompt_audit.go`：Probe 不得记录完整模型响应。
- `web/src/features/prompt-audit/components/endpoints-tab.tsx`：协议切换勿覆盖用户已编辑的 URL/模型。

## 回滚

1. 先在管理页删除或停用 `llm_classifier` 节点，或关闭审计。
2. 再回滚代码。否则旧版本会把未知 protocol 判为配置非法。
3. Option JSON 无需迁移脚本；节点 protocol 字段是普通字符串。

## 开始前检查

- [x] 用户已审核并批准本方案终稿（含拟决议与固定提示词）。
- [x] 未将 Moderations、阿里云护栏、渠道选择器塞进范围。
- [x] 批准后才允许 `python3 ./.trellis/scripts/task.py start 09-04-prompt-audit-llm-classifier`。
- [x] 启动后先读 `trellis-before-dev`，再改代码。
