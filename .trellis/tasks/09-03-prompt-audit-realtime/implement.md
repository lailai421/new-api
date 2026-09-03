# Realtime 接入实施步骤

## 前置输入

核心与存储子任务完成后开始，先冻结 Decision 和 EventStore。

## 步骤

1. 实现 RealtimeEvent 的显式文本提取器和 NoPrompt/Unsupported 判定。
2. 为首个文本前的控制帧定义数量和字节上限。
3. 重构 `relay.WssHelper` 和 OpenAI Realtime handler 的上游连接时序。
4. 建立单一上游写入器，确保每个文本帧先审计后入队。
5. 实现 Block 保持连接、基础设施错误关闭连接的错误事件。
6. 正确传播 context 取消并回收读写协程、连接与通道。
7. 用本地 WebSocket fake 验证连接次数、帧顺序、阻断和取消。
8. 执行 race-sensitive 单元测试、gofmt 和受影响包检查，每条命令不超过 60 秒。

## 交付证明

测试必须观察 fake upstream 实际收到的帧，证明危险帧为零，而不是只检查本地 Decision。

