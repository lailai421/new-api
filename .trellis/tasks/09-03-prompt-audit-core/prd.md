# 提示词审计核心能力

## 目标

实现可复用的同步提示词审计领域核心，为所有请求入口提供统一配置、Guard 调用、判定与失败关闭语义。

## 依赖

- 依赖父任务 `09-03-prompt-audit` 的 PRD、技术设计和代码调研。
- 不依赖其他子任务，必须最先完成。
- 为 `prompt-audit-storage-api`、`prompt-audit-http-relay`、`prompt-audit-realtime` 和 `prompt-audit-task-plugin` 提供稳定接口。

## 范围

- 关闭/同步阻断两态配置及默认值。
- 全部分组/指定分组匹配、完整/最新轮模式、Pass 留存开关、保留期、九类风险和有序节点池。
- AES-256-GCM 版本化密文、随机 nonce、用途隔离和稳定部署密钥校验。
- 不可变运行时配置快照、配置版本、degraded 状态。
- Unicode 分片、全局/单节点并发隔离、有序故障切换和结果聚合。
- OpenAI 兼容 Qwen3Guard 请求及严格响应解析。
- 稳定 Decision、错误码、EventStore 和 ConfigStore 接口。

## 不包含

- GORM 事件表与具体 Option 持久化。
- Controller、Router、Relay 和前端接线。
- 具体协议文本提取。

## 验收标准

- [ ] 配置默认关闭，启用时至少有一个可用节点、一个风险分类及有效分组范围。
- [ ] 未配置稳定 `CRYPTO_SECRET`/`SESSION_SECRET` 时无法启用。
- [ ] 节点 Token 和提示词正文使用不同用途密钥加密，密文可完整解密。
- [ ] 配置存在但解析、解密或激活失败时进入 degraded，不回退为关闭。
- [ ] Safe、Controversial、Unsafe、未知分类和九类 scanner 判定符合父设计。
- [ ] 429、5xx、网络失败和超时按节点顺序切换；4xx 与非法响应不被错误重试。
- [ ] 超长 Unicode 输入全部分片并按最高风险聚合，任一 Block 可提前终止。
- [ ] 核心日志不包含 Token、提示词正文或 Guard 响应正文。
- [ ] 单元测试使用 testify，覆盖真实行为边界，不添加随机/计时型伪测试。

