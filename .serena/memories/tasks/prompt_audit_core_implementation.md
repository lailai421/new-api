# prompt-audit-core 实施完成总结（2026-09-03）

## 冻结公共契约与边界
- 领域包位置：service/promptaudit/
- 冻结类型：
  - ActiveConfig, PublicConfig, Endpoint, ActiveEndpoint, PublicEndpoint
  - PromptSnapshot, NormalizedResult, Decision, GuardError
  - ScannerCatalog, ScannerDefinition, AllScannerIDs (9类标准分类)
  - 稳定错误码：ErrorCodeBlocked, ErrorCodeUnavailable, ErrorCodeInvalidResponse, ErrorCodeConfigDegraded, ErrorCodeRecordFailed, ErrorCodeUnsupportedProtocol, ErrorCodeConfigConflict, ErrorCodeEncryptionKeyRequired
- 冻结抽象接口：
  - ConfigStore (Load, Save CAS)
  - EventStore (Record)
  - Encryptor (EncryptToken, DecryptToken, EncryptPrompt, DecryptPrompt)
  - Evaluator (Evaluate)
  - PromptScanner (Scan)
  - Manager (Active, RuntimeState, IsDegraded, Reload)
- 安全与工程规则：
  - 两态制：仅 off 与 blocking，无 async/worker。
  - AES-256-GCM envelope：版本化前缀 v1:，随机 nonce，HMAC-SHA256 用途隔离派生（guard_token 与 full_prompt 严格分离）。
  - 必须配置稳定 CRYPTO_SECRET 或 SESSION_SECRET 才能启用审计。
  - 运行时 degraded：配置损坏或启用后无可用节点时进入 degraded，请求返回 503，不退化为关闭。
  - 节点故障切换：429、5xx、网络超时按序 failover；4xx 与非法格式不盲目重试。
  - Qwen3Guard 严格解析：必须且仅能有一个 Safety: 和一个 Categories: 主字段；单分片 Block 立即短路终止。
  - 安全外发 HTTP Client：TLS >= 1.2，禁用系统代理，禁止跨主机重定向，响应体限制 256 KiB。
  - 严格结构化元数据日志白名单：绝无 Token、ScanText、FullPrompt 或请求响应体泄漏。
  - 所有 JSON 编解码必须调用 common.Marshal / common.Unmarshal。
