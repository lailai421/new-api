# Task Plugin 接入实施步骤

## 前置输入与依赖状态

- `service/promptaudit` 核心领域契约（Snapshot、Decision、Evaluator、Manager、EventStore、加密）已通过定向测试验证并保持冻结。
- `controller/relay.go` 现有 HTTP Relay、Realtime 改动已通过回归测试，修改时采用局部增量编辑，禁止整体覆盖。
- 工作树干净，HEAD 为 `0f6c3591`。

## 实施步骤

### 步骤 1：插件 Meta、注册校验、Schema 与文档扩展
1. 修改 `pkg/jsplugin/registry.go`：
   - 在 `Meta` 结构体中添加 `AuditTextPaths []string`（`json:"auditTextPaths,omitempty"`）。
   - 在 `decodeMeta` 中将 `auditTextPaths` 加入合法字段集合，并调用受限路径解码器。
   - 在 `cloneMeta` 中深度复制 `AuditTextPaths`。
   - 在 `normalizeV1Meta` 中进行严格受限 JSON Pointer 校验（最多 16 条、单条 <=256 字符、深度 <=10、必须以 `/` 开头、无通配符、去重规范化）。
2. 同步更新文档与 Schema：
   - `docs/plugin-api/v1.schema.json`：新增 `auditTextPaths` 数组定义。
   - `docs/plugin-api/v1.d.ts`：在 `Meta` 接口声明 `auditTextPaths?: readonly string[]`。
   - `docs/plugin-api/v1.md` 与相关 README。
3. 单元测试覆盖：`pkg/jsplugin/registry_test.go` 验证合规路径、转义、越界/畸形路径拦截及 generation 复制。

### 步骤 2：受限 JSON Pointer 文本提取器实现
1. 在 `service/promptaudit/extract_task.go` 实现：
   - `EvaluateJSONPointer(target any, pointer string) (any, bool, error)`：解析并遍历 pointer，严格处理 `~1` 和 `~0` 转义，数组索引限制 0..1000。
   - `ExtractTaskRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo) ([]PromptSegment, string, string, error)`：从 `task_request` 按照请求绑定的 pinned `Meta.AuditTextPaths` 提取文本。
   - 目标值类型检查：仅允许非空字符串、非空字符串数组、安全 text 内容块；对象/数字/布尔/Base64/URL 容器报错返回 `ErrUnsupportedProtocol`。
   - 若插件缺少 `auditTextPaths` 且具备提交能力，返回 `ErrUnsupportedProtocol`；若全部路径提取为空，返回 `ErrNoPrompt`。
2. 单元测试覆盖：`service/promptaudit/extract_task_test.go`，覆盖合法值、缺失值、数组越界、非法类型、空输入及转义处理。

### 步骤 3：十个内置插件补齐 auditTextPaths 契约
修改以下 10 个内置插件的 `meta` 导出：
1. `plugins/tasks/alibaba/plugin.js` -> `auditTextPaths: ["/prompt"]`
2. `plugins/tasks/doubao/plugin.js` -> `auditTextPaths: ["/prompt"]`
3. `plugins/tasks/google/plugin.js` -> `auditTextPaths: ["/prompt"]`
4. `plugins/tasks/hailuo/plugin.js` -> `auditTextPaths: ["/prompt"]`
5. `plugins/tasks/jimeng/plugin.js` -> `auditTextPaths: ["/prompt"]`
6. `plugins/tasks/kling/plugin.js` -> `auditTextPaths: ["/prompt", "/negative_prompt"]`
7. `plugins/tasks/sora/plugin.js` -> `auditTextPaths: ["/prompt"]`
8. `plugins/tasks/sunoapi/plugin.js` -> `auditTextPaths: ["/prompt", "/gpt_description_prompt", "/tags", "/title"]`
9. `plugins/tasks/vertex-ai/plugin.js` -> `auditTextPaths: ["/prompt"]`
10. `plugins/tasks/vidu/plugin.js` -> `auditTextPaths: ["/prompt"]`

每个内置插件在测试中提供一条 Pass 和一条 Block 回归路径。

### 步骤 4：Task Plugin 统一门禁接线（RelayTask 路径）
1. 在 `service/promptaudit/gate.go` 新增 `CheckTaskRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo) *dto.TaskError`：
   - 检查 Manager degraded 状态 -> 503 `prompt_audit_config_degraded`；
   - 检查 ActiveConfig.Enabled 及分组匹配；
   - 调用 `ExtractTaskRequest` 提取文本片段；
   - 构建 `BuildPromptSnapshot` 并调用 `Evaluator.Evaluate`；
   - Block 时落库并返回 403 `prompt_guard_blocked`；
   - 基础设施异常或写库失败返回 503；
   - 所有失败返回均设置 `LocalError: true`。
2. 在 `controller/relay.go` 的 `RelayTask` 中：
   - 在 `GenRelayInfo` 成功、读取 `task_action` 之后，在 `ResolveOriginTask` 与 `executeTaskSubmission` 之前调用 `CheckTaskRequest`；
   - 若返回错误，调用 `respondTaskSubmissionError(c, taskErr)` 并立即返回。

### 步骤 5：OpenAI Responses Bridge 独立门禁接线与双源合并
1. 在 `service/promptaudit/extract_task.go` 实现 `ExtractTaskPluginResponsesRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo, pinned pluginruntime.PinnedEndpoint) ([]PromptSegment, string, string, error)`：
   - 提取源 A（原始 Responses RequestBody）：通过 `dto.OpenAIResponsesRequest` 提取多轮文本段；
   - 提取源 B（规范化 task_request）：按 `pinned.Plugin.Meta.AuditTextPaths` 提取补充文本；
   - 确定性合并与去重：源 A 优先，源 B 随后，内容完全相同的片段稳定去重。
2. 在 `service/promptaudit/gate.go` 新增 `CheckTaskPluginProtocolRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo, pinned pluginruntime.PinnedEndpoint) *dto.TaskError`。
3. 在 `controller/plugin_protocol.go` 的 `serveTaskPluginProtocol` 中：
   - 在 `ResolveOriginTask` 与 `deps.submit` 之前调用 `CheckTaskPluginProtocolRequest`；
   - 失败时调用 `respondPluginProtocolSubmissionError(c, taskErr)` 并中止；
   - 在上下文设置审计完成标记，确保后续 `executeTaskSubmission` 不重复审计。
4. 在 `respondPluginProtocolSubmissionError` 中适配提示词审计错误码（保持 `prompt_guard_blocked` 等错误码稳定暴露）。

### 步骤 6：Runtime 未覆盖插件报告
1. 在 `pkg/jsplugin/routing.go` 实现 `UncoveredSubmitPlugins(g *RoutingGeneration) []string`：
   - 扫描当前 generation 中的全部有效插件；
   - 筛选具备提交能力但 `len(meta.AuditTextPaths) == 0` 的插件；
   - 排除已禁用插件、被拒绝加载插件及纯查询插件；
   - 稳定排序并去重。
2. 在 `controller/prompt_audit.go` 的 `GetPromptAuditRuntime` 响应中，注入 `uncovered_plugins` 字段。

### 步骤 7：入口全覆盖回归测试
编写与扩充测试：
1. `service/promptaudit/extract_task_test.go`：JSON Pointer 边界、受限类型、双源合并去重。
2. `controller/relay_task_test.go` 或专用测试：
   - legacy submit `/v1/tasks/:key`
   - native submit（如 alibaba, doubao, sunoapi, kling）
   - dynamic route
   - OpenAI Video（JSON 与 multipart）
   - Responses Bridge（stream / sync / background）
   - 验证 Block、Unavailable、RecordFailed 时 upstream 计数为 0、预扣计数为 0。
3. 第三方无契约插件测试：审计关闭正常使用，审计开启且命中分组时返回 503。
4. Runtime 接口未覆盖插件测试：发布新无契约插件即时感知，禁用插件不误报。

### 步骤 8：静态检查、规范校验与自检
执行：
- `gofmt`
- `go test -count=1 -timeout 55s ./pkg/jsplugin ./service/promptaudit ./middleware ./controller`
- `go vet`
- 检查 `common.*` JSON 编解码规范
- 敏感信息防泄漏检索
- 不修改 `relaykit/`，不修改受保护品牌

## 实施验证结果

所有步骤 1~8 均已完整实现并通过验证：

1. **步骤 1 (Meta、校验与未覆盖探测)**：
   - `pkg/jsplugin/registry.go`、`pkg/jsplugin/routing.go`、`docs/plugin-api/` 完成扩展。
   - `go test ./pkg/jsplugin` 全部通过。
2. **步骤 2 (受限 JSON Pointer 与双源合并)**：
   - `service/promptaudit/extract_task.go`、`service/promptaudit/gate.go` 完成实现与解耦契约。
   - `go test ./service/promptaudit` 全部通过。
3. **步骤 3 (十个内置插件补充契约)**：
   - `alibaba`, `doubao`, `google`, `hailuo`, `jimeng`, `kling`, `sora`, `sunoapi`, `vertex-ai`, `vidu` 均已配置 `auditTextPaths`。
4. **步骤 4 (RelayTask 门禁接入)**：
   - `controller/relay.go` 接入 `resolveTaskAuditMeta` 与 `promptaudit.CheckTaskRequest`，在 `ResolveOriginTask` 与预扣前阻断。
5. **步骤 5 (Responses Bridge 门禁接入)**：
   - `controller/plugin_protocol.go` 接入 `CheckTaskPluginProtocolRequest` 与防重标记，错误码映射完成。
6. **步骤 6 (Runtime 未覆盖插件报告)**：
   - `controller/prompt_audit.go` 在 `GetPromptAuditRuntime` 响应中透传 `uncovered_plugins`。
7. **步骤 7 (全入口回归测试)**：
   - `controller/prompt_audit_task_plugin_test.go` 覆盖十大内置插件 Pass/Block、第三方未覆盖插件失败关闭与分组放行、Responses 桥接与双源去重、运行时接口上报及 RelayTask 零副作用端到端断言，测试全部 PASS。
8. **步骤 8 (规范与静态检查)**：
   - `gofmt` 格式化通过。
   - `go vet ./pkg/jsplugin ./service/promptaudit ./controller ./dto` 零警告。
   - `cd relaykit && GOWORK=off go test ./...` 独立编译测试通过。
   - `common.Unmarshal` 全面遵守，无业务直调 `encoding/json`，未修改品牌与版权。


