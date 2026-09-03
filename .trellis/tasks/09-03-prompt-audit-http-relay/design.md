# HTTP Relay 接入设计

## 提取边界

在 `service/promptaudit/extract_relay.go` 对 relaykit 具体 DTO 做类型分派，在 `extract_midjourney.go` 处理 Midjourney。提取器返回 Snapshot、NoPrompt 或 Unsupported 三种结果。

完整原文按稳定段落顺序拼接且不截断；最新轮模式只改变 ScanText，不改变 FullPrompt。请求体通过项目可复用 BodyStorage 读取，不能消耗或改写原始请求语义。

## 接线位置

- `controller.Relay`：`GenRelayInfo` 成功后、现有敏感词检查前。
- `controller.RelayMidjourney`：识别提交类 relay mode 并解析 DTO 后、调用 relay submit 前。

错误使用现有协议响应适配，设置 SkipRetry。门禁不参与业务渠道自动禁用、retry 或计费。

