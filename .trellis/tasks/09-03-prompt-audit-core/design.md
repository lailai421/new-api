# 核心能力设计

## 包边界

新增 `service/promptaudit/` 领域包，内部按 `types`、`config`、`manager`、`crypto`、`guard`、`qwen3guard`、`http_client` 拆分。该包可以依赖 `common`，不能依赖 Controller、Relay 或前端。

核心对外暴露：

- `ActiveConfig` 与脱敏 `PublicConfig`。
- `Manager.Active()`、`Manager.RuntimeState()`、`Manager.Reload()`。
- `Evaluator.Evaluate(ctx, config, snapshot)`。
- `Decision`：Allow、Warn、Block、Unavailable、Invalid。
- `ConfigStore`：加载和 CAS 保存抽象。
- `EventStore`：保存判定结果抽象。
- `Encryptor`：Token 与 Prompt 分域加解密。

协议子任务构造统一 `PromptSnapshot`，核心只消费 Snapshot，不认识 Gin 或具体请求 DTO。

## 关键约束

- 模式只有 off/blocking，不保留 async 字段或 Worker。
- 分组采用精确字符串匹配；配置错误进入 degraded。
- 节点调用固定 OpenAI Chat Completions 协议，响应最大 256 KiB。
- 不继承系统代理，不跟随跨主机重定向，TLS 最低 1.2，允许 Root 配置的私网 Guard。
- JSON 操作全部走 `common.*`。
- 每个请求只生成一次最终 Decision；业务入口不自行解释 Guard 文本。

## 测试边界

使用 httptest Guard 节点和内存 Store，验证配置、密钥、解析、并发隔离、故障切换、取消、超时与敏感日志防泄漏。

