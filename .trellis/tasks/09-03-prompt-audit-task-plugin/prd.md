# 提示词审计 Task Plugin 接入

## 目标

为视频、通用 Task Plugin、声明式插件路由和共享协议桥建立明确的提示词字段契约（`auditTextPaths`）及同步门禁，保证在审计开启时，未经审计的任何提示词无法到达业务上游或触发计费。

## 依赖

- 依赖父任务 `09-03-prompt-audit`。
- 强依赖 `09-03-prompt-audit-core` 的 Snapshot、Evaluator、Decision 和稳定错误码（已验证就绪）。
- 强依赖 `09-03-prompt-audit-storage-api` 的 EventStore 与 CAS 配置（已验证就绪）。
- 与 HTTP 子任务共同涉及 `controller/relay.go`，修改时严格保留已合并的 HTTP Relay、Realtime 及其他现有逻辑。

## 范围

1. 插件 v1 Meta/JSON Schema/类型/文档的 `auditTextPaths []string` 契约与严格受限 JSON Pointer 校验。
2. 十个内置插件（`alibaba`、`doubao`、`google`、`hailuo`、`jimeng`、`kling`、`sora`、`sunoapi`、`vertex-ai`、`vidu`）全量声明真实文本字段路径。
3. 受限 JSON Pointer 解析器：支持绝对路径、确定转义（`~1` -> `/`, `~0` -> `~`），禁止通配/递归，限制数量（<=16）、单路径长度（<=256）、深度（<=10）及数组索引（<=1000）。
4. 规范化 `task_request` 文本提取：接受字符串、字符串数组、合法 text 内容块；拒绝对象/数字/布尔/二进制/文件容器宽松转换。
5. 入口级 Gate 接线：
   - `/v1/tasks/:key` legacy submit
   - 声明式 `submit` 与 `dynamic->submit` route
   - OpenAI Video JSON 与 multipart create
   - 被插件接管的 OpenAI Responses（`serveTaskPluginProtocol` 独立路径，与原始标准输入合并去重）
6. 失败关闭与安全：审计开启且分组命中时，提交类第三方插件缺少契约或路径类型错误稳定返回 503 `prompt_audit_unsupported_protocol`；审计关闭或分组未命中时保持原有行为。
7. Runtime 未覆盖插件报告：`/api/prompt-audit/runtime` 返回当前 generation 启用且具备提交能力但缺少有效契约的插件列表，稳定排序去重。

## 不包含

- 插件查询（query/retrieve/content）、任务状态轮询和二进制结果代理。
- 插件上游生成结果审核。
- 标准非插件 HTTP Relay、Midjourney 及 Realtime WebSocket（已有对应子任务完成）。
- 前端管理 UI 页面实现（由 web-console 子任务负责）。
- 修改数据库模型或迁移。
- 修改 `relaykit/` 模块。

## 验收标准

- [ ] 每个内置插件的所有提示词、negative prompt、歌词和描述提示字段都有路径声明。
- [ ] 路径采用严格受限 JSON Pointer，禁止通配符、递归和宽松类型转换。
- [ ] 审计开启且命中分组时，提交类插件缺少有效契约或路径类型错误返回 503 `prompt_audit_unsupported_protocol`。
- [ ] 审计关闭或未命中分组时，旧第三方插件行为不变。
- [ ] 被插件接管的 Responses 同时覆盖原始标准输入和 decode 后的补充文本，且稳定去重，单次逻辑请求只审计一次。
- [ ] Block/Unavailable/RecordFailed/Unsupported 时 task submit 与业务上游调用次数为零，预扣费次数为零。
- [ ] Base64 文件、URL、回调地址、模型名、Token 和内部元数据不进入 Guard 或审计事件。
- [ ] 插件注册校验、Schema、TS 类型、文档和内置插件定义严格同步。
- [ ] Runtime 接口返回当前 generation 未覆盖启用提交插件列表，不误报纯查询或禁用插件。

