# HTTP Relay 接入实施步骤

## 前置输入

核心与存储 API 子任务完成后开始。合并前确认 `controller/relay.go` 中 Realtime/Task Plugin 的并行改动。

## 步骤

1. 实现 segment、完整/最新轮选择、Hash 和脱敏预览。
2. 为每个标准 DTO 编写显式提取器和确定性表驱动测试。
3. 实现 Midjourney 提交/查询动作分类与字段提取。
4. 在标准 Relay 的计费前位置接入统一 Gate。
5. 在 Midjourney 业务提交前接入统一 Gate。
6. 添加 fake Guard、fake EventStore、fake billing 和 fake upstream 测试。
7. 验证请求体复用及 optional scalar 的 0/false 值不受影响。
8. 执行 gofmt、受影响包测试和静态检查。

## 交付证明

测试必须逐类断言危险输入不触达业务上游且不预扣费，不能只断言返回状态码。

## 实施与验证结果

- **已实现能力**：
  1. `service/promptaudit/snapshot.go`：支持 PromptSegment 规范化分段、最新轮选择（最新 user + 上轮 assistant，以 `\x1e` 隔离）、SHA-256 哈希计算、最多 96-rune 脱敏预览（密码、API Key、Bearer Token、邮箱、手机号替换为 `***`）。
  2. `service/promptaudit/extract_relay.go`：覆盖 OpenAI Chat/Completions/FIM/Edits、Claude (含 Thinking 块与工具结果)、Gemini (含代码执行与函数结果)、Responses/Compaction、AlphaSearch、Image、Embedding、Rerank、Audio (Speech input 与表单 prompt)；严格排除独立 URL、Data URL 和 Base64 媒体载荷。
  3. `service/promptaudit/extract_midjourney.go`：明确区分提交类与查询/通知/获取类动作，仅对提交动作安全读取请求体并提取 Prompt 和 Content。
  4. `service/promptaudit/gate.go` 与 `controller/relay.go`：在 `controller.Relay` 与 `controller.RelayMidjourney` 的敏感词、token 估算、计费预扣费及业务渠道重试循环之前完成同步阻断与持久化门禁；拦截均设置 `SkipRetry`，确保上游调用 0 次、预扣费 0 次。
  5. 修复了媒体 URL 误杀漏洞，确保如 `https://example.com 总结文档` 或 Midjourney 图文提示词正常送审；增加 Realtime 握手旁路规则，避免阻断 WebSocket 首帧建立。

- **验证记录**：
  - `gofmt` 格式化全部通过。
  - `go test -count=1 -v -timeout 55s ./service/promptaudit`：全部 36+ 组测试 PASS (0.435s)。
  - `go test -count=1 -v -timeout 55s ./controller -run "Prompt|Realtime"`：全部 12 项端到端控制器测试 PASS (1.265s)。
  - `go vet ./service/promptaudit ./controller`：静态检查无报警 (exit 0)。
  - `cd relaykit && GOWORK=off go build ./...`：独立构建 PASS (exit 0)。
  - `go build -v ./...`：全仓编译通过 (exit 0)。

