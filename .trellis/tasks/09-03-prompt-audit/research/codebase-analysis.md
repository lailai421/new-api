# 提示词审计代码调研

## 调研基线

- 参考仓库：`/Users/laiyanfei/code/python/ai-project/github/sub2api/`
- 参考分支与提交：`main` / `5097b31457e6dc9f49e5f5c9c72b925ce79543b3`
- 目标仓库：`/Users/laiyanfei/code/python/ai-project/github/new-api/`
- 目标分支与提交：`main` / `0ed497f066a68613375124303ef54f220267b334`
- 调研目标：识别可复用的产品语义、不能直接复制的实现，以及 `new-api` 中所有提示词入口的同步门禁接入点。

## sub2api 参考能力

### 配置与运行模式

`sub2api/backend/internal/securityaudit/prompt_config.go` 提供三态运行模式：关闭、异步审计、同步阻断。配置包含有序节点池、OpenAI 兼容协议、模型、超时、单次输入上限、风险分类、分组范围、配置版本和 CAS 更新语义。

本任务只保留关闭与同步阻断两态。异步队列、Redis 原文载荷、Worker、租约、重试任务不迁移，避免出现“功能已开启但请求未经审计即放行”的状态。

### 审计执行

- `prompt_guard.go`：全局与单节点并发隔离、有序故障切换、按字符分片、失败关闭。
- `prompt_qwen3guard.go`：调用 `{base_url}/v1/chat/completions`，默认模型为 `sileader/qwen3guard:0.6b`，严格解析 `Safety` 与 `Categories`。
- 九类风险：暴力、非暴力违法、性内容、个人敏感信息、自杀与自残、不道德行为、政治敏感、版权侵权、越狱攻击。
- `Safety: Safe` 放行；`Controversial` 通常标记告警，其中越狱、个人敏感信息、自杀与自残升级为阻断；`Unsafe` 命中启用分类或未知分类时阻断。
- 节点不可用返回 `prompt_guard_unavailable`，响应格式非法返回 `prompt_guard_invalid_response`，命中阻断返回 `prompt_guard_blocked`。

### 提示词提取与留存

`prompt_snapshot.go` 对 OpenAI Chat、Anthropic Messages、Gemini、OpenAI Responses 和媒体请求分别提取文本，并优先审计最新用户输入。当前实现还支持 `blocking_latest_turn_only`。

参考仓库的旧 OpenSpec 声称数据库不保存完整原文，但实际迁移 `182_prompt_audit_full_prompt.sql` 已在 `prompt_audit_events` 增加 `full_prompt`，代码最多保存 65536 个 Unicode 字符。方案以实际代码和本任务已确认的产品决定为准：保存完整未脱敏提示词，但禁止原文进入普通日志和非专用接口。

### 管理控制台

参考前端 `frontend/src/features/prompt-audit/` 包含运行状态、节点池、策略与分类、分组范围、事件筛选、事件详情和删除。该信息架构可复用，Vue 代码和组件不可直接复制到 React 前端。

## new-api 请求链路

### 常规 HTTP Relay

入口位于 `router/relay-router.go`，统一经过 TokenAuth、限流和 `middleware.Distribute()`，再进入 `controller.Relay()`。

`controller/relay.go` 的当前顺序是：

1. `helper.GetAndValidateRequest` 解析并校验协议请求；
2. `relaycommon.GenRelayInfo` 构造 Relay 上下文；
3. 敏感词检查；
4. token 估算与模型价格计算；
5. `service.PreConsumeBilling` 预扣费；
6. 重试循环内选择或复用渠道并调用业务上游。

最小且安全的同步门禁接入点是第 2 步之后、第 3 步之前。此时请求已完成协议级解析，业务上游尚未调用，也未发生预扣费。`middleware.Distribute` 已选定渠道，但渠道选择仅为本地数据库/缓存操作，不等于调用业务上游。审计只执行一次，不随业务渠道重试重复执行。

需覆盖的 Relay 格式包括 OpenAI Chat/Completions、Responses/Compaction、Claude Messages、Gemini、Images、Embeddings、Rerank、Alpha Search、Audio Speech，以及任何确实包含客户端文本的其他格式。Moderations 和纯查询接口没有可送审提示词时不创建事件。

### OpenAI Realtime WebSocket

`controller.Relay` 当前先升级客户端连接，再由 `relay.WssHelper` 建立业务上游 WebSocket。`relay/channel/openai/relay_realtime.go` 的客户端读取协程在解析 `dto.RealtimeEvent` 后立即写入上游。

要满足全入口同步门禁，必须调整为：首个携带可发送文本的客户端事件先审计，再建立或放行业务上游连接；后续 `session.update`、`conversation.item.create`、`response.create` 等携带 instructions、text、transcript、工具描述或工具参数的事件逐帧提取、逐帧审计，通过后才能写入 `targetConn`。纯音频 Base64 和控制帧不作为文本提示词扫描，但可提取的 transcript 必须审计。

### Midjourney

`router/relay-router.go:registerMjRouterGroup` 注册提交与查询入口，`controller.RelayMidjourney` 在生成 RelayInfo 后直接进入对应业务处理。仅提交、变化、缩短、视频等包含文本的动作进入门禁；任务查询、通知、图片获取等无新提示词的操作不创建事件。

提示词字段来自 `dto.MidjourneyRequest` 的 `prompt` 和会被上游使用的 `content`。Base64 图片、Mask 和回调地址不能被误当作文本提示词。

### 视频与通用 Task Plugin

- `/v1/tasks/:key`：`PrepareTaskPluginSubmit` 已解析 JSON 并把规范化请求保存到 Gin 上下文的 `task_request`。
- 声明式插件路由：`PrepareTaskPluginRoute` 执行 decode hook 后把规范化 `RequestBody` 写入 `task_request`。
- OpenAI Responses/Video 共享端点：`PrepareTaskPluginEndpoint` 执行协议 decode 后写入同一上下文。
- `controller.RelayTask` 在 `executeTaskSubmission` 前仍未调用业务上游和结算，是统一门禁位置。

Task Plugin 请求体由插件定义，不能依赖遍历所有 JSON 字符串，否则会误扫模型名、回调 URL、凭据或 Base64。插件元数据当前没有提示词路径声明。为保证新旧及第三方插件不绕过审计，需要扩展插件 v1 清单，增加规范化 `requestBody` 上的 `auditTextPaths` 契约；内置插件补齐声明。审计开启时，包含提交能力但没有有效审计路径的插件请求必须失败关闭，并在运行状态中暴露未覆盖项。审计关闭时保持现有插件行为。

## 身份、分组和权限

`middleware.TokenAuth` 已把用户 ID、用户名、邮箱、Token ID、Token 名称、用户分组和请求使用分组写入 Gin 上下文。审计快照直接复制这些字段，避免事件列表依赖后续可能删除或改名的用户、Token、分组记录。

`new-api` 的分组是稳定字符串，不是 `sub2api` 的数据库分组 ID，因此范围配置采用 `all_groups + groups []string`，匹配最终生效的 using group。

系统设置 `/api/option` 使用 `middleware.RootAuth()`。提示词审计配置、节点探测、运行状态和事件管理同样应使用 RootAuth，完整提示词不进入管理员通用日志接口。

## 配置、密钥和运行时

现有 `Option` 表与 `common.OptionMap` 可承载配置 JSON，但通用 `GetOptions` 会返回大部分 Option。审计配置必须使用专用键并在 `controller.GetOptions` 中显式排除，通过专用 API 返回脱敏 PublicConfig；节点 Token 只能返回 `has_token` 和状态。

`common.CryptoSecret` 在未设置 `CRYPTO_SECRET` 或 `SESSION_SECRET` 时是进程随机值。节点 Token 和完整提示词采用应用层加密时，开启功能前必须验证存在稳定密钥；否则重启或多实例会导致密文无法解密。密钥派生需做用途隔离，节点 Token 与提示词正文不能共用 nonce 或固定密钥上下文。

运行时配置使用不可变快照和原子替换。不存在配置时默认为关闭；已存在但无法解析、解密、启用节点为空或刷新失败时进入 degraded 状态，命中审计范围的请求返回 503，不得按关闭状态放行。配置保存使用 config_version 做 CAS，避免两个管理员页面相互覆盖。

## 数据库适配

`new-api` 通过 `model/main.go:migrateDB()` 的 GORM `AutoMigrate` 管理主库模型，要求 SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 同时兼容。

不能直接复制 `sub2api` 的 PostgreSQL 专用 SQL：`BIGSERIAL`、`JSONB`、`TIMESTAMPTZ`、JSONB CHECK、`SKIP LOCKED` 均不适合作为三库共同实现。本任务没有异步 Job，因此只需要 `prompt_audit_events`。数组、映射和证据采用 TEXT 中的 JSON，并通过 `common.Marshal`/`common.Unmarshal` 读写；时间采用项目常见的 Unix 秒或毫秒整型；查询通过 GORM 构造。

事件表需要围绕 `created_at + id`、request_id、user_id、token_id、group、decision、risk_level、guard_endpoint_id 和 prompt_hash 建立组合或单列索引。自动清理按主键小批量删除，避免长事务和全表锁；手动批量删除设置数量上限。

## 前端接入

“安全与限制”菜单由以下文件驱动：

- `web/src/components/layout/config/system-settings.config.ts`
- `web/src/features/system-settings/security/section-registry.tsx`
- `/system-settings/security/$section` 动态路由

新增 section ID `prompt-audit` 即可形成 `/system-settings/security/prompt-audit`。由于页面包含状态、配置、节点池和事件表，不应塞入通用 Option 表单，而应建立独立 feature 目录、专用 React Query API 和类型。

项目 `web/components.json` 使用 `base-nova`、Base UI、Tailwind CSS 变量、Hugeicons 和 `@/components/ui` 别名。页面复用现有 Card、Tabs、Table、Dialog/Sheet、Form、Switch、Select、Badge、Alert、Pagination、Skeleton 和 Sonner；表单使用 React Hook Form + Zod；完整提示词只在按需加载的事件详情中展示，不进入事件列表查询缓存。

所有新增用户可见文案使用 `t('English key')`，并同步到 en、zh、zh-TW、fr、ru、ja、vi 七个 locale。

## 核心差异与结论

1. 产品模式从 sub2api 三态收敛为关闭/同步阻断两态，不迁移异步基础设施。
2. new-api 的核心 HTTP 请求可以复用已解析 DTO，在计费与业务上游之前统一审计。
3. Realtime 必须重构首帧与逐帧转发边界，单纯在 Controller 入口审计不够。
4. Task Plugin 必须新增明确的 `auditTextPaths` 插件契约；缺少契约时在审计开启状态下失败关闭。
5. 审计事件保存完整未脱敏提示词，但应加密落库、专用详情解密，并禁止进入日志与列表响应。
6. 配置与事件写入失败都不能静默放行；同步门禁以安全优先，返回稳定 503 错误。
7. 数据库实现只能采用 GORM 和三库兼容字段/查询，并完成真实三数据库新建、升级、重复迁移验证。

