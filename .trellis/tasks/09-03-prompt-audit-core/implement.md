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

完成后冻结 ActiveConfig、PromptSnapshot、Decision、ConfigStore、EventStore、Evaluator 的字段与方法，再允许依赖子任务进入实现。接口变更必须同步所有子任务文档。

