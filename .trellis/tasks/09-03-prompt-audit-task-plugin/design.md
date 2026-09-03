# Task Plugin 提示词审计接入技术设计

## 1. 架构目标与定位

为通用 Task Plugin、视频生成、声明式路由（submit / dynamic）以及被插件接管的共享协议端点（OpenAI Video / OpenAI Responses Bridge）建立严格、同步、失败关闭的提示词审计门禁。

```text
客户端请求 (POST /v1/tasks/:key | native submit | dynamic | /v1/videos | /v1/responses)
   │
   ├─ Gin Middleware / Distribute / TokenAuth / Channel 选取
   │
   ├─ Task Plugin Decode Hook ──> 规范化 task_request 写入 Context (pinned LoadedPlugin.Meta)
   │
   ├─ 门禁点 1: controller.RelayTask (native / legacy / video)
   │     ├─ GenRelayInfo
   │     ├─ [Prompt Audit Gate: CheckTaskRequest]
   │     │    ├─ 读取 pinned Meta.AuditTextPaths
   │     │    ├─ 受限 JSON Pointer 从 task_request 提取 PromptSegment
   │     │    ├─ Evaluator.Evaluate + 必要 EventStore.Record
   │     │    └─ 失败 -> respondTaskSubmissionError (LocalError=true, 0 upstream, 0 billing)
   │     ├─ ResolveOriginTask / ApplyOriginTaskAffinity
   │     └─ executeTaskSubmission ──> 渠道 retry / 预扣 / 上游 submit / 任务持久化
   │
   └─ 门禁点 2: controller.serveTaskPluginProtocol (OpenAI Responses Bridge)
         ├─ GenRelayInfo
         ├─ [Prompt Audit Gate: CheckTaskPluginProtocolRequest]
         │    ├─ 源 A: ProtocolRequestContext 原始输入 -> 标准 Responses 提取
         │    ├─ 源 B: pinned Meta.AuditTextPaths -> 规范化 task_request 提取
         │    ├─ 确定顺序合并 + 角色/内容稳定去重
         │    ├─ Evaluator.Evaluate + 必要 EventStore.Record
         │    └─ 失败 -> respondPluginProtocolSubmissionError (LocalError=true, 0 upstream, 0 billing)
         ├─ ResolveOriginTask / ApplyOriginTaskAffinity
         └─ deps.submit ──> executeTaskSubmission (单逻辑请求仅审计一次，普通路径跳过)
```

## 2. 插件契约与受限 JSON Pointer 规范

### 2.1 Meta 扩展
在 `pkg/jsplugin.Meta` 及 `v1.schema.json`、`v1.d.ts` 增加可选字段：
```json
"auditTextPaths": ["/prompt", "/negative_prompt"]
```
- Go 类型：`AuditTextPaths []string`，包含严格解码、cloneMeta、不可变 generation 复制。
- 插件注册校验（`normalizeV1Meta` / `ValidateV1Meta`）：
  - 路径数量限制：最多 16 条。
  - 单路径长度：最大 256 字符。
  - 单路径深度：最大 10 级。
  - 必须以 `/` 开头，符合受限 RFC 6901 语法。
  - 明确禁止通配符 `*`、递归下降 `//`、相对路径或 JSONPath 表达式。
  - 重复路径在校验时规范化去重。

### 2.2 受限 JSON Pointer 提取规则
在 `service/promptaudit/extract_task.go` 中解析并提取规范化 `task_request`：
- **转义支持**：`~1` 还原为 `/`，`~0` 还原为 `~`。
- **数组索引**：仅支持无前导零的十进制非负整数（`0..1000`）；越界视为字段缺失。
- **目标值类型白名单**：
  - `string`：非空字符串提取为文本段。
  - `[]string` 或 `[]any`（元素均为非空字符串）：按顺序逐项提取。
  - 文本内容块（`map[string]any`）：形如 `{"type": "text", "text": "..."}`、`{"type": "input_text", "text": "..."}` 或仅含 `{"text": "..."}` 且不含 `image_url`、`url`、`data`、`fileRef` 等二进制/URL 容器。
- **目标值非法类型**：
  - 数字、布尔、普通无 text 结构的对象、含二进制/URL/文件容器的对象、非字符串数组均视为契约类型错误，失败关闭，返回 503 `prompt_audit_unsupported_protocol`。
- **缺失字段与空值**：
  - 路径未命中、空字符串或空数组视为无内容。若请求的所有声明路径均未提取到有效文本，且当前操作不含其它文本输入，返回 `ErrNoPrompt`（跳过审计，不写事件，不阻断请求）。

## 3. 十大内置插件与全入口字段覆盖清单 (Field Coverage Inventory)

| 插件 Key | 渠道类型 | 提交入口 | decode 前文本字段 | 规范化 `task_request` 字段 | 声明 `auditTextPaths` | 明确排除字段 (不送审) |
|---|---|---|---|---|---|---|
| `alibaba` | 17 | native route (`/ali/...`), responses, video, legacy | `prompt` | `prompt` | `["/prompt"]` | `img_url`, `media`, `first_frame_url`, `last_frame_url`, `audio_url`, `size`, `duration`, `parameters.*` |
| `doubao` | 39 | native route (`/api/v3/...`), responses, video, legacy | `content[].text` (type=text), `prompt` | `prompt` | `["/prompt"]` | `content[].image_url`, `draftTask`, `duration`, `model` |
| `google` | 24 | responses, video, legacy | `prompt`, `input` | `prompt` | `["/prompt"]` | `image`, `images`, `durationSeconds`, `resolution`, `aspectRatio`, `metadata` |
| `hailuo` | 35 | responses, video, legacy | `prompt`, `input` | `prompt` | `["/prompt"]` | `first_frame_image`, `last_frame_image`, `subject_reference`, `callback_url`, `duration`, `resolution` |
| `jimeng` | 49 | native route (`POST /?Action=CVSync2AsyncSubmitTask`), responses, video, legacy | `prompt`, `input` | `prompt` | `["/prompt"]` | `binary_data_base64`, `image_urls`, `images`, `aspect_ratio`, `frames` |
| `kling` | 50 | native routes (`/kling/v1/videos/*`), responses, video, legacy | `prompt`, `negative_prompt`, `input` | `prompt`, `negative_prompt` | `["/prompt", "/negative_prompt"]` | `image`, `image_tail`, `images`, `input_reference`, `__fileRef`, `duration`, `aspect_ratio`, `cfg_scale` |
| `sora` | 48 | responses, video, legacy (含 remix) | `prompt`, `input` | `prompt` | `["/prompt"]` | `files`, `fileRef`, `image`, `images`, `duration`, `size` |
| `sunoapi` | 38 | native route (`POST /suno/submit/:action`), responses, legacy | `prompt` (歌词/流派), `gpt_description_prompt` (描述词), `tags`, `title` | `prompt`, `gpt_description_prompt`, `tags`, `title` | `["/prompt", "/gpt_description_prompt", "/tags", "/title"]` | `make_instrumental`, `mv`, `continue_at`, `continue_clip_id` |
| `vertex-ai` | 41 | responses, video, legacy | `prompt`, `input` | `prompt` | `["/prompt"]` | `image`, `bytesBase64Encoded`, `fileRef`, `durationSeconds`, `resolution`, `generate_audio` |
| `vidu` | 52 | responses, video, legacy | `prompt`, `input` | `prompt` | `["/prompt"]` | `images`, `__fileRef`, `duration`, `resolution`, `movement_amplitude`, `bgm` |

## 4. Responses Bridge 双源合并与稳定去重

在 `serveTaskPluginProtocol` 中独立执行桥接门禁：
1. **源 A（原始标准协议输入）**：从 `ProtocolRequestContext.RequestBody` 提取 `dto.OpenAIResponsesRequest`，复用 `extractResponsesRequest`（提取 `instructions` 与 `input` 中的多轮消息/文本内容）。
2. **源 B（插件 decode 规范化输入）**：从 `c.GetString("task_request")` 按 `pinned.Plugin.Meta.AuditTextPaths` 提取补充文本。
3. **合并与去重**：
   - 保留确定顺序：源 A 片段优先追加，源 B 补充片段随后追加。
   - 稳定去重：对于内容完全相同的文本片段（按角色与文本严格比对），仅保留首个出现的片段，避免插件 decode 将原始输入字段重写后导致重复送审。
   - 保留 Role（user / system）、User 标志及 Stage。
4. **单次审计保证**：在 `serveTaskPluginProtocol` 成功完成门禁后，在 Gin 上下文标记已审计标识；后续无论 `deps.submit` 还是内部 retry，不再重复调用 Gate。

## 5. 失败关闭与业务安全隔离

当审计开启且用户命中生效分组时：
1. **第三方未覆盖插件**：若具备提交能力（含 submit route / dynamic route / protocol claims / task channelTypes）且 `len(meta.AuditTextPaths) == 0`，拒绝请求，返回 HTTP 503 `prompt_audit_unsupported_protocol`。
2. **路径类型不合法**：目标字段出现对象或非字符串，返回 HTTP 503 `prompt_audit_unsupported_protocol`。
3. **Guard 阻断**：返回 HTTP 403 `prompt_guard_blocked`。
4. **基础设施失败**：Unavailable、Invalid、ConfigDegraded、RecordFailed 均稳定映射为 HTTP 503。
5. **零副作用保证**：所有上述错误均设置 `LocalError = true`，确保：
   - 业务渠道调用次数为 0；
   - 任务持久化与创建次数为 0；
   - 预扣费与计费扣除次数为 0；
   - 不进入渠道重试循环，不触发渠道自动禁用。

## 6. Runtime 未覆盖插件报告契约

在 `pkg/jsplugin/routing.go` 提供 `UncoveredSubmitPlugins(g *RoutingGeneration) []string`：
- 读取当前发布 generation 中的全部有效插件；
- 过滤出具备提交能力但 `len(meta.AuditTextPaths) == 0` 的插件；
- 排除已禁用插件、被拒绝加载插件及纯查询插件；
- 对 Key 进行稳定排序与去重；
- 在 `controller.GetPromptAuditRuntime` 中作为 `uncovered_plugins` 数组返回给 Root 管理员。


