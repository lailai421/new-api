# 核心能力实施步骤

## 前置输入

先读取父任务 `prd.md`、`design.md` 和 `research/codebase-analysis.md`。本子任务无其他子任务依赖。

## 步骤

1. 定义配置、Snapshot、Decision、GuardError、ScannerCatalog 和 Store 接口。
2. 实现默认值、规范化、分组匹配、节点与边界校验。
3. 实现版本化 AES-256-GCM envelope、随机 nonce、用途隔离和稳定密钥检查。
4. 实现 Manager 的不可变快照、版本状态与 degraded 语义，具体持久化通过 ConfigStore 注入。
5. 实现 rune 分片、优先片段、结果聚合和并发隔离。
6. 实现安全 HTTP Client、OpenAI 兼容请求和 Qwen3Guard 严格解析。
7. 补齐确定性单元测试和日志泄漏回归测试。
8. 执行 gofmt、受影响包测试和静态检查。

## 交付接口冻结点

已完成并在交付前冻结以下核心公共契约：

- **ActiveConfig** 与 **PublicConfig**：不可变运行时快照、脱敏公开展现、`MatchesGroup(group string) bool`、`EnabledEndpoints() []ActiveEndpoint`。
- **PromptSnapshot**：统一协议提取输入，包含 RequestID、UserID、TokenID、Group、Protocol、Model、Stage、FullPrompt（敏感原文）、ScanText（送审文本）、PromptHash、RedactedPreview。
- **Decision** 与 **NormalizedResult**：最终判定（Allow, Flag, Block, Unavailable, Invalid）、HTTPStatus (200, 403, 503)、AllowNextStage、分类与分值聚合。
- **GuardError** 与稳定错误码：`prompt_guard_blocked`, `prompt_guard_unavailable`, `prompt_guard_invalid_response`, `prompt_audit_config_degraded`, `prompt_audit_record_failed`, `prompt_audit_unsupported_protocol`, `prompt_audit_config_conflict`, `prompt_audit_encryption_key_required`。
- **抽象存储与执行契约**：`ConfigStore`, `EventStore`, `Encryptor`, `Evaluator`, `PromptScanner`。
- **Manager**：`Active()`, `RuntimeState()`, `IsDegraded()`, `Reload(ctx)`。

## 验证与验收记录

- 执行命令：
  ```bash
  gofmt -w service/promptaudit/*.go
  go test -count=1 -v -timeout 55s ./service/promptaudit
  go vet ./service/promptaudit
  ```
- 验证结果：
  - `gofmt` 格式化无差异。
  - `go test` 全部单元测试通过（耗时 0.254s），覆盖配置规范化、密钥隔离、Manager 状态机、Qwen3Guard 严格解析、安全 HTTP Client、并发与 failover、零敏感日志泄漏。
  - `go vet` 静态检查 0 警告 0 错误。
  - `encoding/json` 校验：无直接调用，全部走 `common.Marshal` / `common.Unmarshal`。
  - 依赖与治理：未修改 `relaykit/`，未修改受保护品牌与元数据。

