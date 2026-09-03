# 09-03-prompt-audit-integration-check 新上下文执行提示词

你正在仓库 `/Users/laiyanfei/code/python/ai-project/github/new-api` 中继续 Trellis
子任务 `09-03-prompt-audit-integration-check`。请全程使用中文，把提示词审计的最终集成、
安全和三数据库质量门禁执行到可交付状态。该任务是父任务的独立最终验收，不是简单运行几条
测试后宣称完成；发现范围内缺陷时应定位根因、在原责任边界修复并重新验证，但不得削弱已批准的
失败关闭、安全、权限、完整原文留存或数据库兼容契约。

## 一、先恢复上下文，不能直接开始验收或修改代码

1. 完整读取仓库根目录 `AGENTS.md`。涉及前端时继续完整读取 `web/AGENTS.md`；若实际修改目录
   还有更深层 `AGENTS.md`，也必须读取。严格遵守中文、JSON 包装、三数据库、测试超时、
   `relaykit` 独立性、敏感数据保护和项目治理规则。
2. 使用 `trellis-continue` Skill 恢复当前工作流，并运行：

   ```bash
   python3 ./.trellis/scripts/task.py current --source
   ```

   新上下文没有活动任务，或指针仍指向父任务/其他子任务，都不代表需要创建新任务。始终以
   `.trellis/tasks/09-03-prompt-audit-integration-check` 为目标，不要重复创建任务，也不要启动父任务
   `09-03-prompt-audit`。
3. 按以下顺序完整读取原始权威材料，不能用本提示词摘要替代：
   - `.trellis/tasks/09-03-prompt-audit-integration-check/prd.md`
   - `.trellis/tasks/09-03-prompt-audit-integration-check/design.md`
   - `.trellis/tasks/09-03-prompt-audit-integration-check/implement.md`
   - `.trellis/tasks/09-03-prompt-audit-integration-check/implement.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-integration-check/check.jsonl`
   - `.trellis/tasks/09-03-prompt-audit-integration-check/task.json`
   - `.trellis/tasks/09-03-prompt-audit/prd.md`
   - `.trellis/tasks/09-03-prompt-audit/design.md`
   - `.trellis/tasks/09-03-prompt-audit/implement.md`
   - `.trellis/tasks/09-03-prompt-audit/research/codebase-analysis.md`
   - 六个实现子任务各自的 `prd.md`、`design.md`、`implement.md`、`task.json`、
     `implement.jsonl`、`check.jsonl` 和最终验收/验证记录：
     - `09-03-prompt-audit-core`
     - `09-03-prompt-audit-storage-api`
     - `09-03-prompt-audit-http-relay`
     - `09-03-prompt-audit-realtime`
     - `09-03-prompt-audit-task-plugin`
     - `09-03-prompt-audit-web-console`
   - 所有 `implement.jsonl`、`check.jsonl` 引用的规范和研究文件。
4. 检查 `git status --short`、当前 HEAD、最近提交、六个依赖任务状态及相关 diff。保护用户和其他
   任务的已有改动，不得覆盖、回退或格式化无关文件。本提示词生成时工作树干净，HEAD 为
   `2aceceb6`，已存在以下实现提交：
   - `f01f90d8`：prompt-audit-core；
   - `4e05a70b`：prompt-audit-storage-api；
   - `0a13f3c4`：prompt-audit-http-relay；
   - `0f6c3591`：prompt-audit-realtime；
   - `c61edc02`：prompt-audit-task-plugin；
   - `2aceceb6`：prompt-audit-web-console。

   这些只代表交接时快照，新上下文必须以实际 HEAD、源码、测试和任务文档为准。交接时
   storage-api、realtime、task-plugin、web-console 标记为 `completed`，core 与 http-relay 仍标记为
   `in_progress`。本集成任务强依赖六个实现子任务全部完成；如果状态或实际验收仍未完成，列出精确
   依赖缺口并停止，不得仅凭已有提交绕过依赖，也不得在集成任务中代替上游任务伪造完成状态。
5. 使用可用的 Serena 做符号级检索和引用分析；若 Serena 不可用，使用 FastCtx 或 `rg` 降级。
   仓库事实能回答的问题必须先检索，不能向用户反问。记录实际入口、Gate、EventStore、计费、
   上游调用、管理 API、前端缓存和迁移调用链，不要根据旧文件名猜测实现。
6. 规划门禁不可跳过。该子任务当前为 `planning`。若没有证据表明用户已在最新最终规划摘要之后
   明确批准实施，先按 `trellis-brainstorm` 输出最终规划摘要，必须包含 Goal、In Scope、
   Out of Scope、Acceptance Criteria、Key Decisions、Risks/Deferred Items 和 artifact status，
   然后停止等待明确批准。本交接提示词或“执行任务”的初始措辞都不等于实施批准。
7. 用户随后明确批准最新规划摘要后，只启动本子任务：

   ```bash
   python3 ./.trellis/scripts/task.py start 09-03-prompt-audit-integration-check
   ```

   进入 `in_progress` 后按当前 Trellis 工作流执行。默认先派发 `trellis-implement` 完成验收、补测和
   必要修复，再派发 `trellis-check` 做独立全范围复核。每个派发提示词第一行必须是：

   ```text
   Active task: .trellis/tasks/09-03-prompt-audit-integration-check
   ```

   告诉代理它不是代码库中唯一的工作者，不得回退或覆盖他人改动；它已经是对应 Trellis 代理，
   不得递归派发同类 `trellis-implement` 或 `trellis-check`。若环境明确采用 inline 流程，则按
   breadcrumb 使用 `trellis-before-dev` 后由主会话执行，不要混用两种流程。

## 二、任务目标和完成判定

以父任务 PRD 的每一项验收标准为权威基线，建立“需求 → 入口/数据流 → 源码 → 测试 → 命令结果”
的可追溯证据矩阵，独立证明：

- 审计关闭时原请求链路不回归；审计开启且命中分组时不存在绕过同步门禁的提示词入口。
- Pass/Flag 只有在 Guard 判定及必要事件写入完成后才能进入计费和业务上游。
- Block 以及 Unavailable、Invalid、ConfigDegraded、RecordFailed、Unsupported 等失败关闭路径中，
  业务上游调用和预扣费次数均为 0。
- 完整提示词只以应用层密文落库，只由 Root 事件详情按需解密；Token、密文、原文、ScanText、
  Guard 请求/响应和内部异常不进入通用 API、普通日志、前端列表/缓存或客户端错误。
- 管理配置、权限、保留清理、手动删除、Task Plugin 契约、Realtime 状态机和七语种前端共同工作。
- SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 的真实实例验证矩阵完整通过。

只有六个依赖子任务真实完成、父/子验收证据齐全、所有要求的质量命令通过、三数据库真实矩阵通过，
且没有未解决的 P0/P1 安全或数据兼容缺陷时，才可以建议父任务完成。任何未执行、失败或因环境受限
无法完成的必要验证都必须明确列为阻塞，不得用代码审查、Mock、SQLite 单库或已有提交替代。

## 三、准确范围

### 3.1 必须执行

1. 建立父 PRD 全部验收项的证据矩阵，不得只复述六个子任务的自报结果。每项至少记录：需求 ID/
   原文、真实代码位置、直接测试、执行命令、最新结果和缺口。
2. 复核并直接测试全部提示词入口：
   - OpenAI Chat/Completions；
   - OpenAI Responses/Compaction；
   - Claude Messages；
   - Gemini；
   - Images、Embeddings、Rerank、Alpha Search、Audio 中明确上送的文本；
   - Midjourney 提交类动作；
   - OpenAI Realtime 首文本和连接存续期后续文本帧；
   - legacy `/v1/tasks/:key`、声明式 submit、dynamic→submit；
   - OpenAI Video JSON 与 multipart；
   - 被插件接管的 Responses stream/sync/background；
   - 十个内置 Task Plugin 及缺失/错误 `auditTextPaths` 的第三方插件。
3. 每类入口至少覆盖 Pass、Block 和一个基础设施/契约错误。直接观察 Gate 次数、必要事件写入、
   预扣费次数、业务上游拨号/HTTP/submit 次数和协议响应；不能只断言内部 Decision 或状态码。
4. 验证顺序必须是：明确协议提取 → 同步 Guard → 必要事件成功写入 → 计费/预扣 → 业务上游。
   审计不得随业务渠道 retry、Task submit retry、Responses 观察循环或 Realtime 生命周期重复执行。
5. 验证审计关闭、分组未命中和确定 NoPrompt 路径保持原行为且不写审计事件；未知、畸形或无法
   证明无文本的 Unsupported 路径在审计命中时稳定失败关闭。
6. 验证默认审计完整请求输入；`latest_turn_only` 只改变实际 `ScanText`，不改变事件保存的完整
   `FullPrompt`。system、developer、user、assistant/model、工具描述/参数/结果、附件文本及协议特有
   提示词字段按显式契约覆盖，URL、Data URL、Base64、模型名、回调地址、凭据和二进制内容被排除。
7. 独立复核 Realtime 状态机：首个危险文本零上游拨号、零上游帧、零预扣；首个 Block 后连接仍可
   接收安全文本；连接建立后的危险帧不写上游；基础设施/协议错误发送安全 error 后关闭；缓存有界、
   顺序不变、单写入器和资源回收无竞态。
8. 独立复核 Task Plugin：十个内置插件声明与实际 decode 输出一致；pinned generation 不漂移；
   query/retrieve/content 是 NoPrompt；第三方提交插件缺少/错误契约时失败关闭；Responses Bridge
   原始协议与规范化补充文本稳定合并、去重且只审计一次。
9. 验证配置和运行时：两态模式、精确分组、默认值、scanner、节点优先级、稳定密钥要求、CAS 冲突、
   expected/active version、Reload 和 degraded 行为。配置损坏、密钥变化、无有效节点时不得退化为
   关闭后放行。
10. 验证事件原子性和生命周期：Block/Error 必存，Pass/Flag 受 `store_pass_events` 控制；必要写入
    失败不放行；列表不含原文/密文，详情仅 Root 解密；默认永久、有限保留、cutoff 边界、重复清理、
    单删和最多 500 条批删均符合契约。
11. 验证管理 API 的未登录、普通用户、Admin、Root 权限矩阵；只有 Root 可访问专用 API，且只有详情
    返回 `full_prompt` 并带 `Cache-Control: no-store`。通用 `/api/option` 无法读取或更新秘密配置。
12. 验证前端菜单、深链接、四个页签、CAS 冲突、degraded、节点探测、筛选分页、详情按需请求、
    删除二次确认、Token write-only 和七种语言。关闭/切换/删除详情及组件卸载后，React Query 中
    不得残留完整提示词；列表解析必须剥离意外出现的敏感未知字段。
13. 做静态与动态泄漏检查。使用专门 canary 值覆盖 Prompt、Token、密文和 Guard 响应，检查日志、
    error wrapping、API JSON、runtime、管理日志、前端缓存和测试输出；不要在失败消息中打印 canary
    的完整敏感值。禁止只靠静态搜索得出“不泄漏”结论。
14. 执行真实三数据库矩阵、Go/前端质量命令和 `relaykit` 边界检查，精确记录版本、脱敏命令、时间、
    结果与证据。
15. 对发现的问题追踪到拥有该行为的原文件并做最小修复，补充保护真实契约的确定性回归测试，随后
    重跑受影响检查和完整门禁。若修复会改变已批准产品范围、公共 API、数据库迁移策略或安全取舍，
    先回到规划同步父/子文档并请求用户决策，不能自行扩张范围。

### 3.2 明确不包含

- 不增加异步只记录后放行、队列、Worker、Redis 原文载荷或补偿审计。
- 不审核业务上游生成内容，不审核图片、音频、视频等二进制载荷本身。
- 不用递归遍历任意 JSON 字符串来制造“覆盖全部”的假象。
- 不通过降低失败关闭级别、跳过事件写入、放宽 RootAuth、截断原文或减少测试来让检查通过。
- 不做与提示词审计无关的重构、依赖升级、全仓格式化或性能优化。
- 不修改、删除或替换任何受保护的 new-api / QuantumNous 项目身份、品牌、元数据或归属信息。
- 不自动执行 `git commit`、`git push` 或破坏性数据库/文件操作。提交和高风险操作必须按仓库规则
  单独获得用户明确确认。

## 四、必须保持的跨层不变量

对每条实际提交路径验证以下数据流，而不是逐文件孤立检查：

```text
客户端输入
  -> 鉴权、分组和请求解析
  -> 协议显式提取 / NoPrompt / Unsupported
  -> PromptSnapshot（完整原文仅在受控内存边界）
  -> Guard 同步判定
  -> 必要事件加密并成功写入
  -> token 估算与预扣费
  -> 业务上游拨号/请求/submit
  -> 原有响应、结算和任务生命周期
```

以下不变量必须有直接观察证据：

1. Block 和所有失败关闭结果发生时，业务上游及预扣计数为 0，且不触发渠道 retry/禁用。
2. Allow/Warn 只在必要事件落库成功后放行；`store_pass_events=false` 时才允许 Pass/Flag 不写库。
3. 一个逻辑请求或 Realtime 文本帧只审计一次；业务重试不重复审计。
4. 审计范围按最终生效分组精确匹配；空列表、不存在分组、解析失败不得意外扩大或缩小范围。
5. 完整原文无截断、Unicode/NUL 往返一致；超长文本全部按 rune 分片扫描，不只扫描前 N 字符。
6. optional scalar 的显式 `0`、`0.0`、`false` 和 multipart 边界不因提取、Body 复用或重序列化丢失。
7. 所有 JSON 编解码使用 `common.*` 包装；不得在新增业务代码直接调用 `encoding/json` 的
   Marshal/Unmarshal。
8. Prompt、ScanText、Token、密文、Guard Body/Response、节点内部信息不进入非专用边界。
9. Router → Controller → Service → Model 分层未被集成修复破坏，`relaykit/` 不依赖根模块。

## 五、三数据库真实验证矩阵

本任务明确涉及数据库最终验收。必须在真实 SQLite、MySQL 5.7.8+ 和 PostgreSQL 9.6+ 实例上
验证，不能用 Mock、驱动编译通过或单一方言代替。先检查仓库已有数据库测试工具、Docker/CI 配置、
最新发布版标签与迁移夹具，复用现有机制，不另造互不兼容的框架。

每种数据库都要记录精确服务版本、驱动/DSN 的非敏感部分、启动方式和实际命令，并验证：

1. 空库启动/AutoMigrate，随后再运行至少一次证明幂等。
2. 使用“最新发布版”程序创建含代表性既有数据的数据库，再由当前代码升级；升级后重复迁移至少
   一次，证明数据、索引、约束、唯一性和其他业务表未受损。不能把当前 HEAD 创建的库冒充升级库。
3. 保存超过 64 KiB、包含多字节 Unicode 和 NUL 的完整提示词，验证加密落库后解密逐字符相等，
   并确认 MySQL 使用 LONGTEXT、PostgreSQL/SQLite 使用兼容 TEXT。
4. 配置首次创建、正常 CAS、旧版本冲突和真实并发更新，不发生静默覆盖。
5. 事件稳定分页、全部筛选项、索引存在性与顺序语义。
6. `retention_days=0` 不自动删除；有限保留的 cutoff 前/等于/后边界；重复清理幂等；单删和
   1..500 批删行为正确。
7. 已有用户、Token、渠道、Option 及其他代表数据保持可读，审计事件身份快照不依赖外键。
8. 如果实际实现让主库与独立日志库共享受影响迁移/查询路径，按根 `AGENTS.md` 同时覆盖单独配置的
   日志数据库；否则记录为何不适用。

只使用明确的临时测试数据库/容器，绝不能对不明 DSN 或生产环境执行迁移、删除和批量更新。创建、
删除容器/卷、删除数据库或执行其他高风险操作前，按根 `AGENTS.md` 的危险操作模板说明精确目标、
影响和恢复方式，并取得用户明确确认。凭据不得出现在提交、报告、日志或聊天中。任一引擎、新建、
升级、重复迁移或行为项没有真实执行，均为完成阻塞。

## 六、测试与质量门禁

新增或大幅改写的 Go 测试使用 `github.com/stretchr/testify/require` 做前置和致命断言，使用
`github.com/stretchr/testify/assert` 做非致命比较。测试必须保护用户可观察行为、API 契约、计费/
审计安全不变量或数据兼容性；使用确定性表驱动、`httptest`、fake upstream 和显式计数器。禁止随机
输入、大循环、真实 sleep、时间比较、日志-only、纯 smoke 或只提高覆盖率的测试。

1. 先对实际修改的 Go 文件执行 `gofmt`，不要格式化无关文件。
2. 按包或 `-run` 拆分 Go 测试，每条后台单元测试命令显式设置不超过 60 秒的超时。根据真实受影响
   包执行，至少覆盖与下列等价的集合：

   ```bash
   go test -count=1 -timeout 55s ./service/promptaudit
   go test -count=1 -timeout 55s ./model
   go test -count=1 -timeout 55s ./controller
   go test -count=1 -timeout 55s ./middleware
   go test -count=1 -timeout 55s ./pkg/jsplugin
   go test -count=1 -timeout 55s ./relay
   go test -count=1 -timeout 55s ./relay/channel/openai
   go test -race -count=1 -timeout 55s ./relay/channel/openai
   ```

   若包级命令超时，继续按测试集拆分，不能删除超时或改用一个可能长时间阻塞的全仓命令。再执行
   与实际范围相称的 `go vet`、根模块构建和静态检查，并记录所有跳过项及原因。
3. 在 `web/` 先核对 `package.json` 的真实脚本，再使用 Bun 执行父任务要求的全部现有质量脚本：

   ```bash
   bun run test
   bun run typecheck
   bun run lint
   bun run format:check
   bun run copyright:check
   bun run i18n:sync
   bun run build
   ```

   不得臆造不存在的脚本或改用 npm/pnpm。`i18n:sync` 可能修改 locale；运行后检查 diff，并重跑受
   影响的格式、类型、测试和构建。若全量测试超过 60 秒，按真实测试入口拆分并保留超时约束。
4. 检索新增/修改业务代码，确认没有直接 `encoding/json.Marshal/Unmarshal`、提示词/Token/Guard
   Body/Response 日志、密文序列化、通用 Option 暴露、前端 console 输出、TODO、占位实现、旧兼容
   旁路或只为测试添加的生产后门。
5. 以提示词审计开发前的实际基线和当前 HEAD 检查 `relaykit/` 是否被修改。若有任何改动，必须审查
   模块独立性并执行：

   ```bash
   cd relaykit && GOWORK=off go build ./...
   ```

   根模块成功不能替代该命令。

## 七、缺陷处置、证据记录与最终交付

1. 在子任务目录创建或更新
   `.trellis/tasks/09-03-prompt-audit-integration-check/research/integration-report.md`，集中记录：
   - 六个依赖的实际状态、提交和验收证据；
   - 父 PRD 验收映射；
   - 协议/入口 Pass、Block、Error 矩阵；
   - Gate、EventStore、计费和业务上游的顺序/次数证据；
   - 权限和敏感信息泄漏检查；
   - 三数据库精确版本、新建/升级/重复迁移和行为结果；
   - Go、前端、race、静态、构建和 `relaykit` 命令结果；
   - 发现的缺陷、修复提交范围、重验结果和剩余风险。
2. 报告中只记录脱敏命令和安全元数据。禁止粘贴 DSN 凭据、Token、完整提示词、密文、Guard 原始
   请求/响应或其他敏感值。失败证据用稳定错误码、哈希、长度和 canary 标识描述。
3. 每发现一个问题，先判断责任模块和违反的父/子验收项；做最小根因修复并添加回归测试，不保留旧的
   绕过兼容代码。修复后更新证据矩阵并重跑该模块及所有受影响下游检查。
4. 实施完成后使用 `trellis-check` 对本集成任务及父任务全部范围做独立复核并直接修复问题。检查不能
   只看最后一批 diff，必须覆盖六个实现子任务、完整跨层数据流、三库矩阵、前端敏感缓存和所有父
   PRD 验收项。
5. 检查后运行 `trellis-update-spec`，判断并沉淀值得长期保留的全入口门禁、敏感审计数据、三数据库
   迁移或跨协议验收经验；没有需要更新时也要明确记录判断，不能伪造规范更新。
6. 更新 integration-check 的 `prd.md`、`design.md`、`implement.md` 和 `task.json` 所需交付记录，
   保证最终实现、命令和证据一致。若发现父任务设计与已落地行为冲突，先同步相关规划资料并重新走
   审批门禁，不能静默改写产品规则。
7. 按根 `AGENTS.md` 将关键约束、最佳实践和经验写入 Serena memory；Serena 不可用时明确说明使用
   FastCtx/`rg` 降级，不能声称已经写入。
8. 检查 `git diff`、`git status` 和所有新增文件，确认没有敏感数据、无关改动、占位符、TODO、调试
   日志、测试凭据、旧旁路或被覆盖的并行工作。
9. 不自动执行 `git commit` 或 `git push`。按 Trellis 工作流先向用户展示分批提交计划、每批文件及
   所有未识别脏文件，等待用户明确确认后才可提交；禁止 push。提交后再按 `trellis-finish-work`
   完成收尾提醒。

只有以下条件全部满足，才可声明 integration-check 完成：

- 六个实现子任务都已完成且实际契约冻结，不只是存在提交；
- 父 PRD 每项验收标准均有代码、直接测试和最新命令证据；
- 所有 HTTP、Midjourney、Realtime、视频和 Task Plugin 入口不存在绕过；
- 所有 Block/失败关闭路径均证明上游调用和预扣费为 0；
- 完整原文、Token、密文和 Guard 数据的权限、存储、日志、API 与浏览器缓存边界通过检查；
- SQLite、MySQL、PostgreSQL 的真实新建、发布版升级、重复迁移和行为矩阵全部通过；
- Go、race、vet/构建、前端 test/typecheck/lint/format/copyright/i18n/build 及必要的 relaykit 检查通过；
- 所有发现的范围内缺陷已修复并重验，没有未解决的高风险项；
- Trellis 独立质量检查已通过，且没有越过任务范围或覆盖他人改动。

最终交付请用中文简洁报告：验收结论、父 PRD 证据覆盖率、入口矩阵、关键缺陷与修复、敏感数据
检查、精确数据库版本和矩阵结果、执行过的精确命令与结果、未执行/失败项及影响、`relaykit`/数据库
影响、剩余风险，以及是否具备建议父任务完成的条件。只要任一必要依赖、测试、真实数据库验证或
安全证据缺失，就必须明确标为阻塞，不能宣称“全部完成”或“三数据库兼容”。
