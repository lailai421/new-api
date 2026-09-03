# Task Plugin 接入设计

## 插件契约

在 Meta 增加 `auditTextPaths: []string`，路径指向 decode 后规范化 `requestBody`。采用受限 JSON Pointer，允许读取字符串、字符串数组和已知 text 内容块，禁止递归通配对象。

内置插件显式声明路径。第三方插件字段为可选，以保持审计关闭时兼容；审计开启且请求属于 submit/dynamic 时，没有可验证文本覆盖就失败关闭。Runtime 返回未覆盖的启用插件列表。

## 接线位置

`PrepareTaskPluginSubmit`、`PrepareTaskPluginRoute` 和 `PrepareTaskPluginEndpoint` 已把规范化请求写入 `task_request`。在 `controller.RelayTask` 生成 RelayInfo 后、解析源任务和 executeTaskSubmission 前构造 Snapshot 并执行 Gate。

被插件接管的 OpenAI Responses 先运行标准 Responses 提取，再合并 auditTextPaths 结果，按角色与文本去重。

