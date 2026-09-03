# 09-03-prompt-audit-web-console 新上下文执行提示词

你正在仓库 `/Users/laiyanfei/code/python/ai-project/github/new-api` 中继续 Trellis
子任务 `09-03-prompt-audit-web-console`。请全程使用中文，把该子任务真正推进到可交付状态。
本任务只实现提示词审计的前端管理控制台，不修改后端审计核心、存储、管理 API 或各协议门禁。

## 一、先恢复上下文，不要直接编码

1. 完整读取仓库根目录 `AGENTS.md` 与 `web/AGENTS.md`，遵守其中全部工程、安全、测试、
   国际化、前端组件和项目治理约束。严禁修改、删除、替换任何与受保护项目身份
   `new-api`、`QuantumNous` 有关的名称、品牌、元数据、版权头或归属信息。
2. 使用 `trellis-continue` Skill 恢复工作流阶段，并明确把目标锁定为
   `09-03-prompt-audit-web-console`。先运行：

   ```bash
   python3 ./.trellis/scripts/task.py current
   ```

   编写本提示词时当前指针仍是父任务 `.trellis/tasks/09-03-prompt-audit`，不要因此误做父任务、
   storage-api 或其他协议子任务。
3. 完整读取以下权威资料，不能只依赖本提示词摘要：
   - `.trellis/tasks/09-03-prompt-audit/prd.md`
   - `.trellis/tasks/09-03-prompt-audit/design.md`
   - `.trellis/tasks/09-03-prompt-audit/implement.md`
   - `.trellis/tasks/09-03-prompt-audit/research/codebase-analysis.md`
   - `.trellis/tasks/09-03-prompt-audit-web-console/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-web-console/design.md`
   - `.trellis/tasks/09-03-prompt-audit-web-console/implement.md`
   - `.trellis/tasks/09-03-prompt-audit-web-console/implement.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-web-console/check.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/design.md`
   - `.trellis/tasks/09-03-prompt-audit-storage-api/implement.md`
4. 检查 `git status`、最近提交和现有改动。保留用户和其他子任务的工作，不得覆盖、回退或格式化
   无关文件。父任务是多子任务协作，实际 HEAD 和工作区源码永远高于本提示词中的时间点摘要。
5. 核实强依赖。编写本提示词时 `09-03-prompt-audit-storage-api` 已标记 `completed`，管理 API
   已落在 `dto/prompt_audit.go`、`controller/prompt_audit.go`、`router/api-router.go`，但新上下文
   必须以实际源码、测试和任务记录共同确认。若 DTO、路由或错误契约又发生变化，以实际冻结契约
   更新前端类型；若 API 缺失或语义未冻结，报告依赖阻塞，不得在本子任务越界补写后端。
6. 使用 `shadcn-ui` Skill 读取 `web/components.json` 和相关 Base UI 规则；该项目使用
   `base-nova`、Base UI、Tailwind CSS 变量和 Hugeicons。使用 `vercel-react-best-practices`
   检查请求并行、缓存、重渲染、不可变数组和包导入。国际化实施时使用 `i18n-translate`。
   不要凭 Radix 示例猜 Base UI API，尤其注意 `render`、`Select items`、ToggleGroup 数组值等差异。
7. 使用可用的 Serena 做符号级检索；若 Serena 不可用，使用 FastCtx 或 `rg` 降级。编码前至少
   核实安全设置 section registry、动态路由、SettingsPage、统一 `api` 客户端、React Query、
   表单、表格、分页、Sheet、AlertDialog、MultiSelect、CopyButton、错误处理和测试夹具的现有模式。
8. 规划门禁不可跳过。父任务与 web-console 子任务当前仍是 `planning`。如果最新规划总结尚未由
   用户在后续消息中明确批准，先按 `trellis-brainstorm` 输出 Goal、In Scope、Out of Scope、
   Acceptance Criteria、Key Decisions、Risks/Deferred Items 和 artifact status 的最终规划总结，
   然后停止等待明确批准。本交接提示词和“执行任务”措辞都不等同于对最新规划总结的批准。
9. 获得明确批准后执行：

   ```bash
   python3 ./.trellis/scripts/task.py start 09-03-prompt-audit-web-console
   ```

   随后使用 `trellis-before-dev` 注入项目规范，再开始修改业务代码。遵循当前上下文注入的 Trellis
   实施/检查调度方式，并使用已整理的 `implement.jsonl` 与 `check.jsonl`。

## 二、目标、依赖与严格边界

在系统设置“安全与限制”下新增 `/system-settings/security/prompt-audit`，提供一个独立的
`PromptAuditPage`，包含四个响应式页签：

1. 运行状态：展示有效模式、期望/生效配置版本、加载时间、启用节点数、degraded 告警、加载错误码
   与未覆盖的 Task Plugin。
2. 策略配置：总开关、全部/指定分组、完整输入/仅最新轮、保存 Pass、风险分类和保留期。
3. 节点池：有序增删改、启停、write-only Token、Token 状态、超时、输入上限和连通性探测。
4. 审计事件：多维筛选、服务端分页、风险/判定 Badge、按需详情、完整原文复制、单条和批量删除。

依赖关系为：`prompt-audit-core → prompt-audit-storage-api → prompt-audit-web-console`。本子任务
不依赖 HTTP Relay、Realtime 或 Task Plugin 门禁实现；runtime 中的 `uncovered_plugins` 可以为空或
非空，页面必须如实展示，不能伪造“全部已覆盖”。

严格不实现：

- 不修改 Go DTO、Controller、Router、Model、Service 或数据库迁移。
- 不接入任何 Relay、Realtime、Midjourney、视频或 Task Plugin 审计门禁。
- 不新增 UI 框架、拖拽库、表格库、状态库或请求库；现有依赖足够。
- 不把提示词审计配置塞进通用 `/api/option` 表单或通用 Option 响应。
- 不为普通用户或普通 Admin 降低 RootAuth 权限，不绕过 401/403。
- 不在浏览器持久化 Guard Token 或完整提示词，不使用 localStorage、sessionStorage、URL 参数或
  Zustand persist 保存敏感数据。
- 不修改 `relaykit/`，不做无关重构，不执行 `git commit` 或 `git push`，除非用户另行明确授权。

## 三、必须以实际源码为准的管理 API 契约

所有端点均位于 `/api/prompt-audit`，由 `middleware.RootAuth()` 保护，使用统一响应包络
`{ success, message?, data? }`。前端类型必须使用明确字段，禁止 `any`。

### 3.1 配置与运行状态

- `GET /api/prompt-audit/config`
  - `data.config` 是脱敏 `PublicConfig`。
  - `data.scanners` 是 `ScannerDefinition[]`。
  - `data.config_version` 是当前 CAS 版本。
- `PUT /api/prompt-audit/config`
  - 请求是全量配置更新，必须携带 GET 返回的 `expected_config_version`。
  - 成功时 `data` 直接是更新后的 `PublicConfig`，不是 GET 的嵌套结构；成功后仍应精确失效并重取
    config 与 runtime，避免遗漏 scanner catalog 或服务端规范化结果。
- `GET /api/prompt-audit/runtime`
  - `data` 包含 `mode`、`expected_config_version`、`active_config_version`、
    `config_loaded_at?`、`config_load_error?`、`degraded`、`enabled_endpoints`、
    `uncovered_plugins`。

`PublicConfig` 当前字段：

```ts
type PromptAuditPublicConfig = {
  enabled: boolean
  effective_mode: 'off' | 'blocking'
  latest_turn_only: boolean
  store_pass_events: boolean
  all_groups: boolean
  groups: string[]
  scanners: string[]
  retention_days: number
  strategy: 'priority'
  endpoints: PromptAuditPublicEndpoint[]
  config_version: number
  updated_at: number
  updated_by: number
  change_summary: string
}
```

公开节点只含以下字段，绝不含 Token 或密文：

```ts
type PromptAuditPublicEndpoint = {
  id: string
  name: string
  protocol: 'openai_compatible'
  base_url: string
  model: string
  timeout_ms: number
  input_limit: number
  enabled: boolean
  has_token: boolean
  token_status: 'configured' | 'missing' | 'invalid'
}
```

配置更新节点在上述非状态字段之外仅允许发送 `token?` 与 `delete_token?`。语义必须明确：

- 未编辑 Token：省略 `token` 和 `delete_token`，保留服务器已有密文。
- 输入新 Token：仅本次请求发送非空 `token`，成功或失败后立即清空表单中的明文。
- 用户明确点击“清除 Token”并二次确认：发送 `delete_token: true`，不得用空字符串含混表达。
- 禁止同时发送新 `token` 和 `delete_token: true`。
- 不能把 `has_token`、`token_status` 当作 PUT 字段回传。

### 3.2 节点探测

`POST /api/prompt-audit/endpoints/probe` 支持两种互斥用法：

- 已保存节点：发送 `{ endpoint_id }`。
- 未保存或当前草稿节点：发送 `protocol`、`base_url`、`model`、可选明文 `token`、`timeout_ms`、
  `input_limit`。探测不得隐式保存配置。

响应 `data` 为 `{ success, latency_ms, model, message, error_code? }`。外层 HTTP 200 不代表内层
探测成功；页面必须同时判断 `data.success`。不要在 toast、console、错误对象或测试快照中输出 Token。

### 3.3 事件列表、详情与删除

- `GET /api/prompt-audit/events`
  - 查询参数：`start_time`、`end_time`、`user_id`、`token_id`、`group`、`model`、`protocol`、
    `decision`、`risk_level`、`category`、`guard_endpoint_id`、`request_id`、`prompt_hash`、
    `p`、`page_size`。
  - 分页默认 20、后端上限 100；前端使用服务端分页，不加载全部事件后本地分页。
  - `data` 为 `{ items, total, page, page_size }`。
  - 列表项包含元数据、`redacted_preview`、`categories`、`matched_scanners`，绝不应包含
    `full_prompt`、`full_prompt_ciphertext`、`scanner_scores` 或 `scanner_evidence`。
- `GET /api/prompt-audit/events/:id`
  - 只有用户打开详情时才请求。
  - 响应带 `Cache-Control: no-store`，`data` 在列表字段基础上新增 `full_prompt`、可选
    `scanner_scores` 与 `scanner_evidence`。
- `DELETE /api/prompt-audit/events/:id`：单条不可恢复删除。
- `POST /api/prompt-audit/events/batch-delete`：请求 `{ ids: number[] }`，必须为 1..500 个明确 ID；
  成功 `data.deleted` 返回实际删除数量。

列表类型和解析逻辑必须使用白名单字段。即使服务端未来误加 `full_prompt`，React Query 的列表缓存
也不能保留该字段；使用 Zod 默认剥离未知字段或显式映射。详情不得预取。

### 3.4 CAS 冲突与错误

配置冲突为 HTTP 409，稳定错误码位于 `error.code`，值为
`prompt_audit_config_conflict`。发生冲突时：

1. 不自动重试 PUT，不用旧版本覆盖新版本。
2. 显示本地化的明确提示。
3. 失效并重新获取 config 与 runtime，以服务器版本重置表单。
4. 明确告知本次草稿未保存；不得假装保存成功。

其他错误使用统一 HTTP 客户端和项目既有 `handleServerError`/toast 习惯处理。UI 优先展示本地化、
不泄漏内部信息的说明；不要直接把未知对象、请求体或敏感字段序列化到界面或 console。

## 四、表单与产品规则

用 React Hook Form + Zod 实现，前端条件校验与后端 `NormalizeAndValidate` 对齐：

- 默认值：关闭、完整请求输入、保存 Pass、全部分组、永久保留、九类 scanner 全选、
  `strategy=priority`。
- `all_groups=false` 时，trim、去重后至少一个非空稳定分组字符串；优先复用已有 `/api/group/`
  数据与 MultiSelect 模式，同时保留服务端已有但当前分组列表暂缺的值，不能静默丢配置。
- scanner 至少选择一类。scanner ID 以 API catalog 为准，当前九类为 `violent`、
  `non_violent_illegal_acts`、`sexual_content_or_sexual_acts`、`pii`、
  `suicide_and_self_harm`、`unethical_acts`、`politically_sensitive_topics`、
  `copyright_violation`、`jailbreak`。
- `retention_days` 是大于等于 0 的整数；0 明确显示为“永久”。从数字输入读取时处理空值和
  `valueAsNumber` 的 NaN，不能让无效值绕过 schema。
- 节点 ID、名称非空且 ID 唯一。Base URL 只允许 http/https，必须有 host，禁止 userinfo、query
  和 fragment；允许管理员配置私网/loopback Guard。
- 协议固定为 `openai_compatible`；默认模型 `sileader/qwen3guard:0.6b`。
- `timeout_ms` 为 100..30000，0 可在请求契约中触发后端默认 3000，但 UI 应显示并提交明确规范值。
- `input_limit` 为 128..100000，UI 默认 4000，单位是 Unicode 字符。
- 启用审计时至少一个节点处于 enabled；已有节点若 `token_status=missing/invalid`，页面必须告警。
  不要在前端假设存在稳定加密密钥，最终启用校验以 API 返回为准。
- 节点顺序就是 priority 故障切换顺序。用“上移/下移”或现有无依赖排序组件实现，不新增拖拽库；
  所有数组更新保持不可变，不能原地 `sort`/`splice` React 状态。

配置初次加载或成功保存后再 `reset` 表单。不要用 effect 链模拟提交动作；保存、探测、删除和复制
逻辑放在明确的事件处理器或 mutation 回调中。派生状态在渲染时计算，避免重复 state 与漂移。

## 五、前端结构与数据流

建议在 `web/src/features/prompt-audit/` 内按职责组织：

```text
api.ts
types.ts
constants.ts
lib/schema.ts
lib/event-parser.ts
hooks/use-prompt-audit-config.ts
hooks/use-prompt-audit-runtime.ts
hooks/use-prompt-audit-events.ts
hooks/use-prompt-audit-mutations.ts
components/runtime-tab.tsx
components/policy-tab.tsx
components/endpoints-tab.tsx
components/events-tab.tsx
components/event-detail-sheet.tsx
components/delete-event-dialog.tsx
prompt-audit-page.tsx
```

这是建议结构，不要求机械创建空文件；遵循单一职责和约 200 行拆分准则，避免只有一个调用点且没有
稳定领域意义的 helper。测试必须放在模块专属 `__tests__/` 目录，不与生产文件平铺。

在 `web/src/features/system-settings/security/section-registry.tsx` 注册：

```text
id: prompt-audit
titleKey: Prompt Audit
path: /system-settings/security/prompt-audit（由 registry basePath + id 形成）
```

section 的 `build` 可忽略通用 `SecuritySettings` 参数并渲染独立 `PromptAuditPage`。不要创建新的
路由文件；现有 `/_authenticated/system-settings/security/$section` 会从
`SECURITY_SECTION_IDS` 自动接受该 section，系统侧边栏由 `getSecuritySectionNavItems` 自动生成。

React Query key 使用稳定层级，例如：

```ts
['prompt-audit', 'config']
['prompt-audit', 'runtime']
['prompt-audit', 'events', normalizedFilters]
['prompt-audit', 'event-detail', eventId]
```

要求：

- config 与 runtime 相互独立，应并行请求，不制造串行 waterfall。
- 保存配置后精确 invalidate config/runtime；删除后精确 invalidate events，并移除被删详情缓存。
- 筛选参数先规范化，避免等价对象产生不同 query key；空字符串不发送为查询条件。
- 页码、page size 和筛选改变时处理选择集，不能对已不在当前结果中的 ID误删。
- 批量选择用 `Set<number>` 或等价 O(1) 结构做重复判断，提交前转为去重数组并再次校验 1..500。
- detail query 仅在 Sheet 打开且 ID 有效时 `enabled`；设置 `gcTime: 0` 或同等策略。关闭 Sheet、
  切换事件、删除事件和组件卸载时，对精确 detail key 调用 `removeQueries`，并清除组件中的原文引用。
- 禁止 prefetch detail。关闭页面后，QueryClient 中不得残留 `full_prompt`。
- 包含明文 Token 的 mutation 设置最短可行生命周期（如 `gcTime: 0`），settled 后调用 reset，并
  立即清空表单 Token；不要把 mutation variables 复制到日志、开发提示或持久 store。
- 运行状态、配置、列表和详情分别提供加载、空、错误与重试状态。不要用一个全屏加载阻塞所有
  独立数据区；复用 Skeleton、Alert 和 Empty。

## 六、UI、可访问性与安全细节

- 复用已有 `@/components/ui` 与公共组件：Card、Tabs、Table、Form/Field、Switch、Select、
  MultiSelect、Badge、Alert、Sheet、AlertDialog、Pagination、ScrollArea、Skeleton、Spinner、
  CopyButton 和 Sonner。先检查实际组件 API，不复制旧版 shadcn 或 Radix 用法。
- 表单使用 `FieldGroup` + `Field`/现有 Form 组合；每个输入有可见 Label、描述和关联错误。
  invalid 同时设置视觉状态与 `aria-invalid`，disabled 同时反映到 Field 和控件。
- `TabsTrigger` 必须放在 `TabsList`；Tabs 在窄屏可横向滚动或采用项目既有响应式模式，不能截断到
  无法访问。每个页签有明确可访问名称和选中状态。
- degraded、Token invalid、探测失败使用 `Alert`；风险和 decision 使用 `Badge` 加文本，不只靠颜色。
  使用主题语义色与内建 variant，不写硬编码 red/green 色和手工 dark mode 覆盖。
- 详情使用 Sheet，必须有 `SheetTitle` 和描述；完整提示词使用保留换行、可滚动、可复制的只读区域，
  处理超长 Unicode 和 NUL 显示，不使用 `dangerouslySetInnerHTML`，不自动复制。
- 删除确认必须使用 `AlertDialog` 的 destructive 操作，明确不可恢复、数量和影响。单删与批删都需
  二次确认，键盘可以取消或确认，打开时焦点进入对话框，关闭后合理恢复焦点。
- 按钮加载态使用 `disabled` + `Spinner` 组合，不假设 Button 存在 `isLoading`。
- 使用项目配置的 Hugeicons；图标按钮必须有 `aria-label`，装饰图标 `aria-hidden`。不要从未知图标库
  或大型 barrel 随意导入；以仓库既有 Hugeicons 导入方式为准。
- 表格在窄屏提供可横向滚动或项目既有移动布局，不能让操作按钮、分页和预览不可达。
- 用户 ID、Token ID、request_id、hash 等筛选按确定字段提交；时间使用项目 Day.js 约定转换 Unix
  秒，明确包含边界，不能混用毫秒。

## 七、国际化

所有用户可见文案必须使用 React 组件内的 `useTranslation()` 和 `t('English source string')`，
不能硬编码中文或把常量 key 直接显示。同步更新以下七个 flat JSON locale：

- `web/src/i18n/locales/en.json`
- `web/src/i18n/locales/zh.json`
- `web/src/i18n/locales/zh-TW.json`
- `web/src/i18n/locales/fr.json`
- `web/src/i18n/locales/ru.json`
- `web/src/i18n/locales/ja.json`
- `web/src/i18n/locales/vi.json`

Scanner catalog 的 `label` 与 `description` 是英文源文案，可作为 i18n key，必须补齐七语种；不要只在
中文时使用后端 `label_zh` 而让其他语言退回未登记字符串。动态 key 若无法被同步脚本识别，显式加入
locale 或项目的 static keys，确保 `bun run i18n:sync` 后仍无缺失。

至少覆盖：菜单、四个页签、字段 Label/帮助/错误、默认/永久、九类风险、模式/风险/判定/Token
状态、加载/空/失败/degraded、探测、复制、删除确认、CAS 冲突、分页和筛选文案。不得修改任何
受保护品牌文案。

## 八、测试与验证

使用 Vitest + React Testing Library，从用户行为和稳定契约验证。新增测试放入相应模块的
`__tests__/`。不要添加大快照、固定 sleep、随机输入、纯 smoke 或只证明组件能渲染的测试。

至少覆盖：

1. section 注册后菜单项、深链接、刷新与非法 section 处理保持正确。
2. 配置响应映射、默认值和 Zod 条件校验：指定分组、scanner 非空、retention、节点 ID、URL、
   timeout、input limit、启用时至少一个节点。
3. Token 输入初始为空；configured/missing/invalid 只显示状态；保留、替换、明确清除三种 payload
   精确；明文不进入 query cache、持久存储、日志或快照。
4. 配置保存携带最新 `expected_config_version`；409 不重试、不显示成功、会重取 config/runtime
   并给出冲突提示。
5. 已保存和草稿节点探测的 payload、成功/失败/超时 UI；探测不触发配置保存。
6. 事件筛选与 `p/page_size` 参数、服务端分页、加载/空/错误状态；列表运行时解析会剥离未知
   `full_prompt` 字段，缓存中没有完整原文。
7. 详情关闭时不请求，打开时才请求；关闭、切换、删除和卸载后精确移除 detail cache。
8. 完整提示词保留换行、超长文本可滚动并能由明确按钮复制；不会以 HTML 注入渲染。
9. 单删/批删必须经过 destructive 二次确认，空选择和超过 500 被阻止，成功后清理选择与缓存。
10. degraded、未覆盖插件、Token invalid、风险与 decision 同时有可见文本，不只靠颜色。
11. 键盘、焦点、可访问名称、`aria-invalid`、`aria-selected`、禁用和加载状态。
12. 七种 locale 不缺键，语言切换后关键页面文案正确更新。

完成后在 `web/` 使用 Bun 执行，所有最新结果必须记录：

```bash
bun run test
bun run typecheck
bun run lint
bun run format:check
bun run copyright:check
bun run i18n:sync
bun run build
```

如全量测试超出单次 60 秒，先执行受影响测试并按目录拆分；后台单元测试命令不得超过 60 秒。
`i18n:sync` 可能改写 locale，执行后重新检查 diff，并再次运行必要的 typecheck、lint、格式与测试。
本任务不涉及数据库或 `relaykit`，无需声称完成三数据库矩阵或 relaykit 构建；这些属于其他子任务。

## 九、执行纪律与完成标准

1. 编码前列出实际 API 类型、query keys、表单状态、敏感数据生命周期、页面组件边界和拟修改文件。
   仓库事实可以直接确定的技术细节自行核实；若遇到产品范围、安全或 API 语义冲突，回到规划并请求
   用户决定，不得擅自扩展范围。
2. 小步实现并持续运行定向测试。代码直接、清晰、低嵌套；避免重复派生 state、无必要 memo、
   render 中创建大量不稳定对象、串行独立请求和原地修改数组。
3. 不新增依赖。只有现有组件确实缺失且用户批准后才允许安装；全局安装或更新核心依赖属于高风险
   操作，必须按仓库规则先取得明确确认。
4. 完成后使用 `trellis-check` 做独立质量检查，修复所有范围内问题并重新验证。
5. 检查 `git diff` 与 `git status`，确认没有 Token、完整提示词、密文、测试凭据、无关改动、TODO、
   占位组件、调试日志、死代码或旧兼容路径。
6. 更新 web-console 的 `prd.md`、`design.md`、`implement.md`，记录最终文件、API 映射、敏感缓存
   清理策略、测试和构建结果；只在验证完成后按 Trellis 流程更新任务状态。
7. 按根 `AGENTS.md` 将关键约束、最佳实践和经验写入 Serena memory；如果 Serena 不可用，明确说明
   使用了 FastCtx/rg 降级，不得伪造 memory 写入。
8. 只有以下条件全部满足，才可声明 web-console 完成：
   - 四个页签、菜单、深链接和刷新都可用；
   - 配置、runtime、probe、events、detail、delete 契约与实际后端一致；
   - CAS 冲突、degraded、权限和错误状态有确定体验；
   - Guard Token 始终 write-only，事件列表与缓存无完整提示词，详情关闭后缓存清除；
   - 删除二次确认、批量上限、键盘和焦点行为通过测试；
   - 七语种无缺失键，受影响测试、typecheck、lint、格式、版权与生产构建通过；
   - 未越界修改后端、协议门禁、数据库、依赖或受保护项目身份。

最终交付请用中文简洁报告：完成内容、关键文件、实际 API 契约适配、敏感数据保护、测试/类型/
lint/格式/i18n/构建结果、未完成项或风险，以及是否已满足 integration-check 子任务的前端前置条件。
任何未执行或失败的验证必须明确列出，不能用代码审查代替测试，也不能宣称未验证项已经完成。
