# 新窗口执行提示词：09-04-prompt-audit-codex-cli-only

把本文件全文作为新上下文窗口的第一条用户消息粘贴即可。方案已经审核通过，不要重新询问是否创建任务、是否采用 `Originator`、是否需要签名认证或是否开始实施；直接按下面的流程启动并实现。

---

你是本仓库的实现代理。仓库路径：`/Users/laiyanfei/code/python/ai-project/github/new-api`。全程使用中文回复、中文分析和必要的中文代码注释。

## 任务

实现 Trellis 子任务 **`09-04-prompt-audit-codex-cli-only`**，父任务为 `09-03-prompt-audit`：

当全局提示词审计开启时，只允许标准 Codex CLI 客户端进入受审计保护的生成与任务提交链路。其他客户端必须在提示词送审、事件写入、计费和业务上游调用之前返回 HTTP 503。

权威文档必须全文读取并严格执行，不要另起方案：

- `.trellis/tasks/09-04-prompt-audit-codex-cli-only/prd.md`
- `.trellis/tasks/09-04-prompt-audit-codex-cli-only/design.md`
- `.trellis/tasks/09-04-prompt-audit-codex-cli-only/implement.md`
- `.trellis/tasks/09-04-prompt-audit-codex-cli-only/research/client-identification-and-gate-map.md`
- `.trellis/tasks/09-04-prompt-audit-codex-cli-only/implement.jsonl`
- `.trellis/tasks/09-04-prompt-audit-codex-cli-only/check.jsonl`

任务当前状态是 `planning`，规划产物已经齐全。**用户已于 2026-09-04 审核通过方案并授权实施。** 文档中“等待审核”“本轮不修改业务代码”等规划阶段措辞已被本次明确批准覆盖；新窗口应执行 Phase 1.3 核对、Phase 1.4 启动、Phase 2.1 实现和 Phase 2.2 检查，不要回到需求讨论。

## 启动顺序

1. 全文读取仓库根目录 `AGENTS.md` 并遵守全部项目规则。不要修改或替换受保护的项目、组织身份信息。
2. 读取 `.agents/skills/trellis-start/SKILL.md`，执行：

   ```bash
   python3 ./.trellis/scripts/get_context.py
   python3 ./.trellis/scripts/get_context.py --mode phase
   python3 ./.trellis/scripts/get_context.py --mode packages
   ```

3. 激活 Serena 项目，先读取 Serena 使用说明，再读取与提示词审计、风格规范和任务完成检查相关的 memory。代码检索优先使用 Serena，必要时降级到 FastCtx/`rg`。
4. 全文读取上述任务文档和 `.agents/skills/trellis-before-dev/SKILL.md`，按 skill 加载相关规范。重点读取：

   - `.trellis/spec/guides/cross-layer-thinking-guide.md`
   - `.trellis/spec/guides/code-reuse-thinking-guide.md`
   - `.trellis/spec/backend/quality-guidelines.md`

5. 核对任务上下文清单，不要推倒重写：

   ```bash
   python3 ./.trellis/scripts/task.py validate 09-04-prompt-audit-codex-cli-only
   ```

   `implement.jsonl` 和 `check.jsonl` 只能包含 spec/research 文件，不能加入业务代码路径。

6. 方案已批准，直接启动任务，不再向用户请求确认：

   ```bash
   python3 ./.trellis/scripts/task.py start 09-04-prompt-audit-codex-cli-only
   python3 ./.trellis/scripts/task.py current --source
   ```

   必须确认 current 指向 `.trellis/tasks/09-04-prompt-audit-codex-cli-only`，状态为 `in_progress`。

7. 完整执行 `trellis-before-dev` 后，按照当前 Codex/Trellis 配置派发 `trellis-implement` 实现代理。派发提示词必须以：

   ```text
   Active task: .trellis/tasks/09-04-prompt-audit-codex-cli-only
   ```

   开头，并明确实现代理不是独占工作区，不得回退他人改动，不得再套一层 implement/check 代理。
8. 实现完成后读取并执行 `.agents/skills/trellis-check/SKILL.md`，派发 `trellis-check` 做独立质量检查；发现问题必须修复并重新验证。
9. 不执行 `git commit`、`git push`、`task.py archive`，除非用户在新窗口明确授权。完成实现和检查后停下来汇报证据。

## 已冻结的产品决定

- 客户端身份采用兼容性识别，不是不可伪造认证。
- 入站 HTTP 请求头 `Originator` 是唯一硬判定依据。
- Header 名由 Go HTTP 机制按大小写不敏感读取；Header 值先 `TrimSpace`，再按 ASCII 大小写不敏感比较。
- 首版只允许以下两个**完整值**：

  - `codex_cli_rs`
  - `codex-cli`

- `CODEX_CLI_RS`、带首尾空白的等价值应通过。
- 空值、`curl`、`my-codex_cli_rs-wrapper`、`Codex CLI` 展示文案值均不通过。
- `User-Agent`、`Session_id`、`Thread_id`、`X-Codex-*` 等只能作为辅助信息，绝不能单独授权，也不要加入回退判定。
- 不使用子串、通配符、正则、请求体内容、模型名、系统提示词或 `You are Codex` 人设识别客户端。
- 请求头可以由调用方伪造，这是用户已接受的风险；不要扩展成签名、密钥、设备证明或伪安全认证。

## 固定错误契约

- 稳定错误码：`prompt_audit_codex_cli_required`
- HTTP 状态：`503 Service Unavailable`
- 固定安全消息：

  ```text
  Prompt audit is enabled; only Codex CLI requests are accepted.
  ```

- 错误和日志不得回显收到的 `Originator`、User-Agent、其他请求头、Prompt、Token 或 Guard 数据。
- OpenAI、Claude、Midjourney、Task、Task Plugin 必须沿用各自现有错误外壳。
- Relay 错误必须保持 `ErrOptionWithSkipRetry()`；Task 错误必须保持 `LocalError: true`，禁止把本地 503 当成渠道错误重试。

## 强制执行顺序

所有受保护提交入口遵循同一顺序：

```text
Manager 不存在             → 保持原行为
配置 degraded              → 原 prompt_audit_config_degraded 503
ActiveConfig.Enabled=false → 保持原行为
Originator 非允许值         → prompt_audit_codex_cli_required 503
Originator 允许             → 继续既有 group match 与提示词审计
```

客户端检查必须位于“确认审计开启”之后、“分组匹配和 Prompt 提取”之前。即使请求分组不在审计范围内，只要全局审计已经开启，非 Codex CLI 仍必须 503；合法 Codex CLI 才继续由原分组规则决定是否送审。

配置 degraded 的错误优先级高于客户端身份错误，不得用新错误掩盖审计系统故障。

## 必须覆盖的入口

统一判定逻辑由 `service/promptaudit` 单点维护，以下入口全部复用，禁止各自复制字符串判断：

1. `CheckRelayRequest`：普通 OpenAI、Claude、Gemini、Responses 等 HTTP Relay。
2. `CheckMidjourneyRequest`：Midjourney 提交。
3. `CheckTaskRequest`：原生 Task 提交。
4. `CheckTaskPluginProtocolRequest`：Task Plugin Responses Bridge 提交。
5. Realtime：必须在 `controller/relay.go` 调用 `upgrader.Upgrade` **之前**执行同一前置门禁。

Realtime 客户端身份检查不替代文本帧审计。合法 Codex Realtime 仍保持：

- `CheckRelayRequest` 跳过 Realtime 占位请求；
- `ShouldAuditRealtime` 决定是否延迟上游拨号；
- `CheckRealtimeEvent` 对客户端文本帧逐帧审计。

非 Codex Realtime 必须返回普通 OpenAI 风格 HTTP JSON 503，不建立客户端 WebSocket，不调用 WSS error writer，也不建立上游 WebSocket。

## 零副作用不变量

非 Codex CLI 被拒绝时，必须同时满足：

- 不提取 PromptSegment；
- 不构造 PromptSnapshot；
- `Evaluator.Evaluate` 调用数为 0；
- `EventStore.Record` 调用数为 0；
- `prompt_audit_events` 不新增记录；
- 预扣费次数为 0；
- 渠道选择/重试次数为 0；
- HTTP 或 WebSocket 业务上游调用次数为 0；
- 响应和日志无敏感信息泄漏。

## 实现要求

- `service/promptaudit/types.go` 增加单一稳定错误码和安全消息来源。
- 在 `service/promptaudit` 中建立具有稳定领域含义的统一客户端识别/前置门禁函数；多个入口复用该函数，符合项目对 helper 和 DRY 的要求。
- 优先复用现有 `GuardError`、Relay/Midjourney/Task 错误包装，不为一个错误状态建立平行错误体系。
- `controller/plugin_protocol.go` 的审计错误映射加入新错误码和固定消息。
- Realtime 在 Upgrade 前完成身份门禁；不要破坏合法 Codex 的原 WebSocket 生命周期和帧级失败关闭。
- Task 的 `ContextKeyTaskAuditDone` 幂等语义保持不变；该标记只能代表此前已经完成合法审计。
- 不记录因客户端类型被拒绝的审计事件，因为请求没有送审、也没有形成 PromptSnapshot。
- 代码保持直接、早返回、低嵌套；中文注释只说明关键时序和信任边界。

## 测试要求

新增或调整 Go 测试必须使用 `testify/require` 做 setup/fatal 断言，使用 `testify/assert` 做非致命断言。测试必须确定性，禁止随机输入、Sleep、日志式 smoke test 或只刷覆盖率。

### 客户端识别契约

表格测试至少覆盖：

- `codex_cli_rs`
- `CODEX_CLI_RS`
- `codex-cli`
- 两端带空白的允许值
- Header 缺失/空值
- `curl`
- `my-codex_cli_rs-wrapper`
- 只有 Codex User-Agent
- 只有 Session/Thread/X-Codex 头
- `Originator: Codex CLI` 不通过

### 门禁顺序

- audit off + 无 Originator：不因新功能拒绝。
- enabled + 非 Codex + group mismatch：503，Evaluator/EventStore 为 0。
- enabled + Codex + group mismatch：按原分组放行，不送审。
- enabled + Codex + group hit：进入原 Evaluator。
- degraded + 任意客户端：仍返回 `prompt_audit_config_degraded`。

### 跨入口

- HTTP Relay、Midjourney、Task、Task Plugin 都要验证 503、错误码和固定消息。
- 每个拒绝用例验证 Evaluator=0、EventStore=0；controller 级用例继续验证预扣费=0、业务上游=0。
- Realtime 必须有 controller 级测试：状态 503、不是 101、客户端/上游 WebSocket 连接均为 0。
- 错误响应和日志不得包含 canary Prompt、Token、Originator 或 User-Agent。

### 旧测试夹具

实现后，现有审计测试中要继续验证 Allow、Block、Guard 失败、事件写入、Realtime 延迟拨号等下游行为的请求，必须显式补充合法 `Originator`。非 Codex 新用例故意不设置或设置未知值。

不要通过关闭 Manager、改成 group mismatch、删除原断言或放宽预期来让旧测试通过。特别检查：

- `service/promptaudit/gate_test.go`
- `service/promptaudit/gate_task_test.go`
- `service/promptaudit/gate_realtime_test.go`
- `controller/relay_prompt_audit_test.go`
- `controller/prompt_audit_task_plugin_test.go`
- `relay/channel/openai/relay_realtime_audit_test.go`

只修改真正经过客户端前置门禁的测试夹具；纯 Config、Snapshot、Guard/Evaluator 单测不应无意义增加 HTTP Header。

## 验证命令

单元测试命令必须显式设置不超过 55 秒的超时：

```bash
go test ./service/promptaudit -count=1 -timeout 55s
go test ./controller -count=1 -timeout 55s -run 'TestRelay.*PromptAudit|TestRelayMidjourney.*PromptAudit|Test.*TaskPlugin.*Audit|Test.*CodexCLI'
go test ./relay/channel/openai -count=1 -timeout 55s -run 'Test.*Realtime.*Audit|Test.*PromptAudit'
go test ./service/promptaudit ./controller ./relay/channel/openai -count=1 -timeout 55s
go build ./...
cd relaykit && GOWORK=off go build ./...
```

先对修改的 Go 文件执行 `gofmt`。如果组合测试无法在限制内完成，拆成单包命令，不得用超过 55 秒的测试超时掩盖问题。

本任务不修改数据库、GORM、模型、迁移或持久化格式，因此无需运行 SQLite/MySQL/PostgreSQL 三数据库矩阵。若实施中实际触及数据库范围，立即停止实现、回到规划并补充三库验证，不能自行扩大任务。

## 明确不做

- 不新增客户端认证密钥、签名、设备证明或远程认证服务。
- 不增加管理员可配置 Originator 白名单。
- 不把 User-Agent、模型名、路径、Prompt 或人设当作客户端身份。
- 不接受 `Originator` 子串、通配符、正则或任意 `Codex CLI` 展示文本。
- 不修改提示词提取、分类器、Guard 节点、缓存、事件表或计费规则。
- 不限制管理 API、模型列表、任务查询/回调、文件下载、健康检查等非提示词提交接口。
- 不改前端配置或 i18n。
- 不改数据库 schema、AutoMigrate 或历史审计事件。
- 不改 `relaykit/` 公共 API。
- 不修改 `one-api.db`、`one-api.db-shm`、`one-api.db-wal` 等本地运行时数据库文件。
- 不 SSH 修改生产环境，不改生产审计配置。
- 不修改、删除或替换受保护的项目与组织身份信息。
- 不顺手重构无关审计、Relay、Task 或 WebSocket 代码。

## 工作区与协作约束

- 当前工作区已有其他任务和本地数据库变更；不要回退、覆盖或格式化无关文件。
- 实现代理不是独占工作区。发现他人并行修改时，调整实现以兼容，不得使用 destructive git 操作。
- 所有 JSON 编解码继续使用 `common.Marshal` / `common.Unmarshal` 等包装；禁止在业务代码直接调用 `encoding/json` marshal/unmarshal。
- 不创建只为缩短函数、只有一个调用方且没有稳定领域语义的机械 helper。
- 新错误码、Originator 允许值和规范化必须只有一处来源。
- 不执行 `git reset --hard`、`git checkout --`、删除文件、批量移动或其他危险操作。

## 完成标准

1. 对照 `prd.md` 的 AC1–AC10，逐项给出测试名、命令或代码证据。
2. 说明 HTTP Relay、Midjourney、Task、Task Plugin、Realtime 五类入口的覆盖情况。
3. 明确报告非 Codex 请求的 Evaluator/EventStore/预扣费/业务上游均为 0。
4. 报告所有实际执行的测试和构建命令；未执行的检查如实说明，禁止声称已验证。
5. 通过 Trellis check 后，按项目要求把本任务关键约束和经验写入 Serena memory。
6. 停在实现与质量检查完成状态，等待用户决定是否提交或归档。

