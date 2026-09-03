# HTTP Relay 接入实施步骤

## 前置输入

核心与存储 API 子任务完成后开始。合并前确认 `controller/relay.go` 中 Realtime/Task Plugin 的并行改动。

## 步骤

1. 实现 segment、完整/最新轮选择、Hash 和脱敏预览。
2. 为每个标准 DTO 编写显式提取器和确定性表驱动测试。
3. 实现 Midjourney 提交/查询动作分类与字段提取。
4. 在标准 Relay 的计费前位置接入统一 Gate。
5. 在 Midjourney 业务提交前接入统一 Gate。
6. 添加 fake Guard、fake EventStore、fake billing 和 fake upstream 测试。
7. 验证请求体复用及 optional scalar 的 0/false 值不受影响。
8. 执行 gofmt、受影响包测试和静态检查。

## 交付证明

测试必须逐类断言危险输入不触达业务上游且不预扣费，不能只断言返回状态码。

