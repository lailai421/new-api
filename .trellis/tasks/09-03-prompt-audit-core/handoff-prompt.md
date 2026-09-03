# 09-03-prompt-audit-core 新上下文执行提示词

你正在仓库 `/Users/laiyanfei/code/python/ai-project/github/new-api` 中继续 Trellis
子任务 `09-03-prompt-audit-core`。请用中文完成整个协作过程，并把该子任务真正推进到
可交付状态；不要顺带实现其他提示词审计子任务。

## 一、先恢复上下文，不要直接编码

1. 完整读取仓库根目录 `AGENTS.md`，并遵守其中全部工程、安全、测试和项目治理约束。
2. 使用 `trellis-continue` Skill 恢复当前任务阶段；按 Skill 要求读取必要说明。
3. 当前父任务与 core 子任务均处于 `planning`。先完整读取以下权威资料，不能只依赖本提示词摘要：
   - `.trellis/tasks/09-03-prompt-audit/prd.md`
   - `.trellis/tasks/09-03-prompt-audit/design.md`
   - `.trellis/tasks/09-03-prompt-audit/implement.md`
   - `.trellis/tasks/09-03-prompt-audit/research/codebase-analysis.md`
   - `.trellis/tasks/09-03-prompt-audit-core/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-core/design.md`
   - `.trellis/tasks/09-03-prompt-audit-core/implement.md`
   - `.trellis/tasks/09-03-prompt-audit-core/implement.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-core/check.jsonl`
4. 检查 `git status`，保留用户和其他任务已有改动，不得覆盖或回退无关修改。
5. 使用可用的 Serena 做符号级检索；若 Serena 不可用，使用 FastCtx 或 `rg` 降级。
   编码前重点核实现有 JSON 包装、密钥配置、日志、HTTP Client、并发限制和测试惯例。
6. 规划阶段门禁不可跳过：如果最新规划总结尚未由用户在后续消息中明确批准，先按
   `trellis-brainstorm` 要求给出 Goal、In Scope、Out of Scope、Acceptance Criteria、
   Key Decisions、Risks/Deferred Items 和 artifact status 的最终规划总结，然后停止并等待
   用户明确批准。不要把本交接提示词本身视为批准。
7. 获得批准后，执行：

   ```bash
   python3 ./.trellis/scripts/task.py start 09-03-prompt-audit-core
   ```

   随后使用 `trellis-before-dev` 注入项目规范，再开始业务代码修改。

## 二、任务目标

实现一个独立、可复用的 `service/promptaudit/` 领域核心，为后续 storage-api、HTTP Relay、
Realtime 和 Task Plugin 子任务提供统一且稳定的配置、加密、Guard 调用、判定与失败关闭
契约。

本任务必须最先完成，并在交付前冻结以下公共契约：

- `ActiveConfig` 与脱敏的 `PublicConfig`
- `PromptSnapshot`
- `Decision`、Guard 错误类型与稳定错误码
- `ConfigStore`、`EventStore`、`Evaluator`、`Encryptor`
- `Manager.Active()`、`Manager.RuntimeState()`、`Manager.Reload()`

公共 API 的命名和具体签名应以仓库现状、Go 惯例及父/子设计为依据；不要机械照搬参考
仓库，也不要为了预想中的下游需求扩张接口。

## 三、严格范围

### 必须实现

- 新增 `service/promptaudit/`，按清晰职责组织 `types`、`config`、`manager`、`crypto`、
  `guard`、`qwen3guard`、`http_client` 及对应测试。
- 只保留 `off` 与 `blocking` 两种模式；不得保留 async、队列、Worker、租约或异步放行语义。
- 默认配置为：关闭、全部分组、审计完整输入、保存 Pass、永久保留、九类 scanner 全开、
  `priority` 节点策略。
- 实现配置规范化与确定性校验：
  - 分组使用精确字符串匹配；指定分组模式至少一个非空分组。
  - scanner 去重并稳定排序，至少启用一个分类。
  - 节点 ID 唯一；节点顺序必须保留管理员配置的优先级。
  - 启用时至少存在一个启用且凭据可用的 Guard 节点。
  - Guard 超时允许 100—30000ms，输入上限允许 128—100000 Unicode 字符。
  - 默认模型为 `sileader/qwen3guard:0.6b`，默认超时 3000ms，默认输入上限 4000 字符。
- 实现不可变运行时配置快照、expected/active config version、loaded_at、load_error 和
  degraded 状态：配置不存在时安全默认关闭；配置存在但解析、解密或激活失败时必须
  degraded，不能退化成关闭并放行。
- 实现 AES-256-GCM 版本化 Base64 envelope：每次随机 nonce，通过稳定部署密钥做用途隔离
  派生，Guard Token 与完整提示词必须使用不同上下文。启用前确认部署显式提供稳定的
  `CRYPTO_SECRET` 或 `SESSION_SECRET`；进程随机默认值不允许启用审计。
- 实现按 Unicode rune 的完整分片，优先处理最新用户片段；不得按字节截断，也不得只审计
  前 N 字符。
- 实现全局并发上限（设计默认 64）和单节点并发上限（设计默认 16）、请求取消、整体有界
  超时、有序节点故障切换和最高风险结果聚合；任一分片 Block 可以立即终止。
- Guard 节点请求固定使用 OpenAI Chat Completions 兼容协议：

  ```json
  {
    "model": "sileader/qwen3guard:0.6b",
    "messages": [{"role": "user", "content": "<chunk>"}],
    "temperature": 0,
    "max_tokens": 64,
    "seed": 42
  }
  ```

- Guard HTTP Client 不继承系统代理，不跟随跨主机重定向，TLS 最低 1.2，响应体上限
  256 KiB；Base URL 只允许 HTTP(S)，禁止 userinfo、query 和 fragment。Root 配置的私网及
  loopback Guard 必须可用。
- 节点失败语义：429、5xx、网络错误和超时按配置顺序切换下一节点；4xx 配置错误与非法
  Guard 响应不得盲目切换或错误重试；容量耗尽视为 unavailable。
- 严格解析 Qwen3Guard：只能接受一个 `Safety:` 和一个 `Categories:` 主字段；Safety 仅允许
  Safe、Controversial、Unsafe，缺失、重复或未知值均为非法响应。
- 判定必须遵循父设计：
  - Safe → Pass / Low / Allow。
  - Controversial → Flag / Medium / Warn；命中 jailbreak、pii、suicide_and_self_harm
    时升级为 Block。
  - Unsafe → 命中启用分类、出现未知分类或没有给出已知分类时 Block；只命中管理员禁用的
    已知分类时 Flag / High / Warn。
  - Allow 和 Warn 都表示审计通过；Block 不通过。
- 九类 scanner ID 与父任务及 `sub2api` 参考实现保持一致，不能产生配置或事件语义漂移。
- 所有 JSON 编解码必须通过 `common.Marshal`、`common.Unmarshal`、
  `common.UnmarshalJsonStr` 或 `common.DecodeJson`；业务代码不得直接调用
  `encoding/json` 的 Marshal/Unmarshal。
- 核心日志只允许安全元数据，不得包含 Guard Token、`ScanText`、`FullPrompt`、Guard
  请求体、完整响应或密文。

### 明确不实现

- 不新增 GORM 事件表，不修改 `model/option.go`、`model/main.go` 或数据库迁移。
- 不实现具体 Option 持久化、事件查询/删除/清理或 Root 管理 API。
- 不修改 Controller、Router、Relay、Realtime、Task Plugin 或前端。
- 不实现任何具体协议的提示词提取器；核心只消费统一 `PromptSnapshot`。
- 不修改 `relaykit/`，也不能让 `relaykit/` 依赖根模块。
- 不实现父任务其他六个子任务，不提前接线业务上游或计费链路。
- 不修改任何受保护的 new-api / QuantumNous 项目身份、品牌、元数据或归属信息。

## 四、实现前必须核实的仓库证据

不要凭猜测设计接口。至少检查：

- `common/json.go` 的 JSON 包装接口及项目调用方式。
- `common.CryptoSecret` 的定义、初始化来源和默认随机值判定方式。
- 项目现有结构化日志接口，确保测试可验证敏感内容不泄漏。
- 现有 HTTP transport、超时、响应体限制、并发 semaphore/bulkhead 与原子快照模式，优先复用
  稳定实现，不新增不必要依赖。
- `sub2api` 固定参考提交 `5097b31457e6dc9f49e5f5c9c72b925ce79543b3`
  中 `backend/internal/securityaudit/` 下的 `prompt_config.go`、`prompt_guard.go`、
  `prompt_qwen3guard.go` 和相关测试。只借鉴产品语义与边界，不复制 PostgreSQL 异步队列。
- 下游子任务文档对 `PromptSnapshot`、Decision、错误码和 Store 接口的实际消费需求；发现契约
  冲突时先更新规划资料并重新请求审批，不要自行改变父任务产品决策。

## 五、质量与测试要求

- 采用直接、可读、低嵌套的实现；遵守 SOLID、DRY、关注点分离和 YAGNI。
- 不要为缩短调用者而新增只有一个调用点、没有稳定领域意义的包级 helper。
- 必要注释使用中文，解释安全边界、失败关闭和并发语义，不写复述代码的注释。
- 新测试使用 `github.com/stretchr/testify/require` 做前置及致命断言，使用
  `github.com/stretchr/testify/assert` 做非致命值比较。
- 使用确定性表驱动测试和 `httptest` Guard 节点/内存 Store；禁止随机压力循环、sleep、
  时间性能比较、日志式断言或只为覆盖率存在的测试。
- 测试至少覆盖：
  - 默认值、规范化、精确分组匹配及所有配置边界。
  - 稳定密钥缺失、用途隔离、随机 nonce、错误密钥/损坏密文和完整 Unicode 往返。
  - 配置缺失与 degraded 的区别、expected/active version 和 Reload 失败后的安全行为。
  - Safe、Controversial、Unsafe、未知/禁用分类，以及重复/缺失/畸形主字段。
  - Unicode 分片不丢字符、优先片段、最高风险聚合和 Block 提前终止。
  - 429、5xx、网络错误、超时的有序 failover；4xx 和非法响应不错误重试。
  - 全局/单节点并发隔离、context 取消、响应体超限、重定向和 URL 安全约束。
  - 日志中不出现 Token、提示词正文或 Guard 完整响应。
- 每条后台测试命令不得超过 60 秒。至少执行并记录：

  ```bash
  gofmt -w service/promptaudit/*.go
  go test -timeout 55s ./service/promptaudit
  go vet ./service/promptaudit
  ```

  如果修改了其他包，再补充对应的定向测试。不要用根目录 `go test ./...` 替代定向验证；若
  完整测试会超过 60 秒，应按包拆分。只有实际触及 `relaykit/` 时才需执行其独立构建，但本
  子任务原则上不得触及它。

## 六、执行纪律与完成标准

1. 先形成精确实施计划和拟冻结接口，再编码；若父/子文档存在冲突，以父任务已确认产品
   决策为准，并显式记录冲突处理。
2. 小步修改并持续运行定向测试；不要修改无关文件，不做顺手重构，不引入新依赖，除非
   现有能力确实无法满足且已说明必要性。
3. 实现后使用 `trellis-check` 做独立质量检查；修复所有范围内问题后重新运行验证。
4. 检查 `git diff` 和 `git status`，确认没有敏感数据、无关改动、占位符、TODO 或兼容性残留。
5. 更新 core 任务文档，记录最终冻结的公共接口、关键安全决策、实际验证命令和结果；如果
   接口变化影响下游，必须同步所有相关子任务文档。
6. 按根 `AGENTS.md` 要求把本次关键约束、最佳实践和经验写入 Serena memory；若 Serena
   不可用，明确说明降级情况，不得伪造已写入。
7. 不执行 `git commit` 或 `git push`，除非用户另行明确授权。
8. 只有以下条件全部满足，才可声明 core 子任务完成：
   - core PRD 的每项验收标准都有实现和测试证据；
   - 公共契约已经冻结并足以支撑四个依赖子任务；
   - 定向测试、静态检查与敏感信息检查全部通过；
   - 没有越过本子任务范围；
   - Trellis 质量门禁已通过。

最终交付请用中文简洁报告：完成内容、冻结接口、关键文件、测试与静态检查结果、未完成项或
风险、是否已满足下游子任务启动条件。任何未执行的验证都必须明确列为阻塞项，不能宣称
“全部完成”或“三数据库兼容”；本 core 子任务本身不涉及数据库变更，因此不要无依据启动或
声称通过三数据库验证。
