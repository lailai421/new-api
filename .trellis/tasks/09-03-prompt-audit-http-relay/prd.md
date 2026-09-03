# 提示词审计 HTTP Relay 接入

## 目标

为标准 HTTP Relay 与 Midjourney 的全部文本提示入口增加协议级提取和计费前同步门禁。

## 依赖

- 依赖父任务 `09-03-prompt-audit`。
- 强依赖 `09-03-prompt-audit-core` 的 Snapshot、Evaluator 和 Decision。
- 强依赖 `09-03-prompt-audit-storage-api` 的 EventStore；必要事件持久化成功后才能放行。
- 与 Realtime、Task Plugin 子任务无功能依赖，但修改 `controller/relay.go` 时必须保留其他子任务的并行改动。

## 范围

- OpenAI Chat/Completions、Responses/Compaction、Claude、Gemini。
- Images、Embeddings、Rerank、Alpha Search、Audio 中明确的文本字段。
- Midjourney 提交类动作的 prompt/content。
- 全文与最新轮提取、Hash、脱敏预览和身份快照。
- 在敏感词、token 估算、预扣费、重试和业务上游之前执行一次 Gate。

## 不包含

- Realtime WebSocket。
- 视频与 Task Plugin。
- 二进制图片、音频和视频内容审核。

## 验收标准

- [ ] 每种协议只提取会发送上游的确定文本字段，不递归扫描任意 JSON 字符串。
- [ ] system、developer、user、assistant、tool、工具结果及协议特有文本均按设计覆盖。
- [ ] URL、Data URL、Base64、回调地址和模型名不会误入 Guard。
- [ ] Block/Unavailable/Invalid/RecordFailed 时业务上游调用和预扣费均为零。
- [ ] 审计只执行一次，不随业务渠道重试重复执行。
- [ ] 审计关闭、分组未命中和确实无文本的请求保持原行为。
- [ ] 未知且可能携带文本的格式失败关闭，不静默跳过。

