# 提示词审计 Task Plugin 接入

## 目标

为视频、通用 Task Plugin、声明式插件路由和共享协议桥建立明确的提示词字段契约及同步门禁。

## 依赖

- 依赖父任务 `09-03-prompt-audit`。
- 强依赖 `09-03-prompt-audit-core` 的 Snapshot、Evaluator 和 Decision。
- 强依赖 `09-03-prompt-audit-storage-api` 的 EventStore。
- 与 HTTP 子任务共同涉及 `controller/relay.go`，实现和合并时不得覆盖其他任务的 Relay/Midjourney 改动。

## 范围

- 插件 v1 Meta/JSON Schema 的 `auditTextPaths` 契约。
- 受限 JSON Pointer 校验及规范化 `task_request` 文本提取。
- 内置 alibaba、doubao、google、hailuo、jimeng、kling、sora、sunoapi、vertex-ai、vidu 插件声明。
- `/v1/tasks/:key`、声明式 submit/dynamic 路由、OpenAI Video 和被插件接管的 Responses。
- 未覆盖插件运行状态和审计开启时失败关闭。

## 不包含

- 插件查询、任务状态读取和二进制结果代理。
- 插件上游生成结果审核。
- 标准非插件 HTTP Relay。

## 验收标准

- [ ] 每个内置插件的所有提示词、negative prompt、歌词和描述提示字段都有路径声明。
- [ ] 路径只读取明确字段，不能通配任意对象所有字符串。
- [ ] 审计开启时，提交类插件缺少有效契约或路径类型错误会返回 503。
- [ ] 审计关闭时，旧第三方插件行为不变。
- [ ] 被插件接管的 Responses 同时覆盖原始标准输入和 decode 后的补充文本，且稳定去重。
- [ ] Block/Unavailable/RecordFailed 时 task submit 与业务上游调用次数为零。
- [ ] Base64 文件、URL、回调地址、模型名和凭据不进入 Guard。
- [ ] 插件注册校验、Schema、文档和内置插件保持同步。

