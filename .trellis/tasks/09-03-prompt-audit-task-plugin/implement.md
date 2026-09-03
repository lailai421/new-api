# Task Plugin 接入实施步骤

## 前置输入

核心与存储子任务完成后开始。修改 `controller/relay.go` 前先同步 HTTP 子任务的最新内容。

## 步骤

1. 扩展 Meta、注册校验、JSON Schema 和插件 API 文档。
2. 实现受限 JSON Pointer 解析、类型校验、去重和输入量边界。
3. 为十个内置插件补齐 auditTextPaths，并添加 Schema/注册测试。
4. 实现 task_request、原始 Responses 与规范化插件文本合并提取。
5. 在 RelayTask 的业务提交前接入统一 Gate。
6. 实现缺少契约时的失败关闭和 runtime 未覆盖插件报告。
7. 对 legacy task、声明式 route、OpenAI Video、Responses bridge 逐入口测试。
8. 执行 Go 测试、插件 lint/format 检查和相关协议回归。

## 交付证明

每个内置插件至少提供一条 Pass 和一条 Block 回归路径；第三方无契约插件必须有“关闭可用、开启阻断”测试。

