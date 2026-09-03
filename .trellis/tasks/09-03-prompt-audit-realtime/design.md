# Realtime 接入设计

## 状态机

`client_connected → awaiting_first_text → auditing → upstream_connected → streaming`。在 awaiting_first_text 阶段缓存有限数量和总字节的控制帧；发现任何可提取文本后先审计，通过才建立业务上游并按原序发送缓存。

streaming 阶段，客户端读取器解析事件并把待发送项交给单一上游写入器。文本事件必须携带已通过的 Decision 才能进入写队列，控制/音频事件携带 NoPrompt 标记。

## 错误行为

Block 发送本地 OpenAI Realtime error event 并丢弃当前帧。Guard/配置/存储/协议错误发送不含内部细节的 error event，随后关闭客户端和业务上游，取消所有协程。

缓冲达到上限时按 Unsupported/Unavailable 失败关闭，不允许无限内存增长。

