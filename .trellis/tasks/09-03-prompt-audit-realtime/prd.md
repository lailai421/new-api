# 提示词审计 Realtime 接入

## 目标

为 OpenAI Realtime WebSocket 建立首个文本发送前和连接存续期逐帧同步审计，确保任何危险文本帧都不会写入业务上游。

## 依赖

- 依赖父任务 `09-03-prompt-audit`。
- 强依赖 `09-03-prompt-audit-core` 的 Snapshot、Evaluator 和错误语义。
- 强依赖 `09-03-prompt-audit-storage-api` 的 EventStore。
- 不依赖 HTTP 和 Task Plugin 子任务；共享错误码和事件契约，不自行定义替代协议。

## 范围

- session.instructions、工具描述/参数、conversation item text/transcript、response.create 输入提取。
- 首个含文本事件通过前延迟业务上游 WebSocket 连接。
- 上游连接后的每个新增文本事件逐帧 Gate。
- Block 仅丢弃对应帧并返回 Realtime error event；基础设施错误返回 error event 后关闭连接。
- 单写协程、取消、超时和缓冲上限。

## 不包含

- 纯音频 Base64 内容审核。
- 业务上游输出内容审核。
- 非 Realtime HTTP 协议。

## 验收标准

- [ ] 首个危险文本事件被阻断时，业务上游连接次数为零。
- [ ] 已建立连接后的危险帧不被写入上游，安全后续帧仍可处理。
- [ ] transcript 和文本工具定义被审计，纯音频 Base64 不被误扫。
- [ ] Block 返回 `prompt_guard_blocked` 且连接可继续。
- [ ] Unavailable、Invalid、ConfigDegraded、RecordFailed 和 Unsupported 返回错误后关闭连接。
- [ ] 缓存控制帧不能携带未提取文本绕过首帧门禁。
- [ ] WebSocket 读写满足 gorilla/websocket 单写入器约束，无 goroutine 泄漏。
- [ ] 所有错误事件和日志均不包含完整提示词。

