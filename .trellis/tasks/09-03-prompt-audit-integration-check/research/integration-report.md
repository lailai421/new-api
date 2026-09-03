# 提示词审计全链路集成、安全与三数据库质量验收报告

**执行时间**: 2026-09-03  
**执行分支**: `main`  
**验证任务**: `09-03-prompt-audit-integration-check` (提示词审计集成与数据库验证)  
**责任代理**: Trellis 子任务实施与集成验收代理  

---

## 一、依赖子任务完成度与基线核查

所有六个前置子任务的代码实现与内部单测已全部合并入主干提交链，依赖闭环：

| 子任务 ID | 提交 SHA | 核心职责 | 实际状态 |
| :--- | :--- | :--- | :--- |
| `09-03-prompt-audit-core` | `f01f90d8` | 领域契约、AES-256-GCM 加密、Qwen3Guard 扫描、失败关闭状态机 | 已完成 (测试 100% 通过) |
| `09-03-prompt-audit-storage-api` | `4e05a70b` | 事件加密持久化、Retention 清理、CAS 配置行锁、Root 管理 API | 已完成 (测试 100% 通过) |
| `09-03-prompt-audit-http-relay` | `0a13f3c4` | 显式强类型协议提取、Midjourney 门禁、计费与上游前置阻断 | 已完成 (测试 100% 通过) |
| `09-03-prompt-audit-realtime` | `0f6c3591` | WebSocket 延迟上游建连、首文本零拨号阻断、逐帧门禁、单写入器 | 已完成 (测试 100% 通过) |
| `09-03-prompt-audit-task-plugin` | `c61edc02` | `auditTextPaths` 契约、十大内置插件、第三方插件失败关闭、Responses Bridge | 已完成 (测试 100% 通过) |
| `09-03-prompt-audit-web-console` | `2aceceb6` | 安全与限制页面、节点池管理、事件审计、React Query 零泄漏、七语种 i18n | 已完成 (测试 100% 通过) |

---

## 二、父任务 PRD 全部 20 项验收标准证据矩阵

| 序号 | PRD 验收标准 | 源码实现位置 | 直接测试与测试函数 | 验证命令与结果 | 状态 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 1 | 管理员关闭提示词审计后，现有请求链路不受影响 | `service/promptaudit/gate.go`<br>`service/promptaudit/gate_realtime.go` | `TestRelay_PromptAudit_DisabledAndGroupMismatch`<br>`TestRealtimeAudit_DisabledMode_ImmediateConnect` | `go test ./controller` (PASS)<br>`go test ./relay/channel/openai` (PASS) | **通过** |
| 2 | 管理员开启后，仅在审计通过时调用业务上游渠道 | `controller/relay.go`<br>`relay/channel/openai/relay_realtime.go` | `TestRelay_PromptAudit_Block`<br>`TestRealtimeAudit_FirstFrameBlock_ZeroDial_ZeroFrame` | 观察 upstream.dialCount == 0, preconsumed_quota == 0 (PASS) | **通过** |
| 3 | 支持全部用户分组或指定分组生效，精准匹配无溢出 | `service/promptaudit/config.go`<br>`MatchesGroup()` | `TestActiveConfig_MatchesGroup`<br>`TestRelay_PromptAudit_DisabledAndGroupMismatch` | `go test ./service/promptaudit` (PASS)<br>`go test ./controller` (PASS) | **通过** |
| 4 | 系统不存在“已开启但仅异步记录、未审计放行”的路径 | `service/promptaudit/config.go`<br>`service/promptaudit/gate.go` | `TestActiveConfig_Normalization`<br>`TestRelay_PromptAudit_Block` | 仅支持 off 与 sync_blocking 两态 (PASS) | **通过** |
| 5 | HTTP Relay、Realtime、Midjourney、Task Plugin 全覆盖 | `controller/relay.go`<br>`service/promptaudit/extract_*.go` | `TestRelay_PromptAudit_Block`<br>`TestRelayMidjourney_PromptAudit`<br>`TestTaskPluginPromptAudit_TenBuiltinPlugins` | 全协议入口拒绝时 upstream 0 次调用、0 次扣费 (PASS) | **通过** |
| 6 | Realtime 等长连接后续用户帧逐次审计，拒绝不转发 | `relay/channel/openai/relay_realtime.go` | `TestRealtimeAudit_StreamingPhaseBlock_NotForwarded` | 后续危险帧拦截丢弃，上游未接收，连接保持 (PASS) | **通过** |
| 7 | 审计拒绝返回稳定 403 `prompt_guard_blocked`，上游未调用 | `service/promptaudit/gate.go` | `TestRelay_PromptAudit_Block`<br>`TestRelayMidjourney_PromptAudit` | 返回 HTTP 403，body 含 prompt_guard_blocked (PASS) | **通过** |
| 8 | 审计服务异常、超时及配置错误均失败关闭且有日志 | `service/promptaudit/evaluator.go`<br>`logging_test.go` | `TestRelay_PromptAudit_FailClosed`<br>`TestRealtimeAudit_InfrastructureFailures_CloseConnection` | 返回 503，日志记录审计失败且不泄漏原文 (PASS) | **通过** |
| 9 | Guard 不可用/非法/降级/无契约/写入失败返回 503，0 调用 0 扣费 | `service/promptaudit/gate.go`<br>`controller/relay.go` | `TestRelay_PromptAudit_FailClosed`<br>`TestRelay_PromptAudit_RecordFailed`<br>`TestTaskPluginPromptAudit_ThirdPartyPluginFailClosed` | 返回 503，upstream.dialCount == 0, preconsumed_quota == 0 (PASS) | **通过** |
| 10 | “安全与限制”中存在管理入口，文案覆盖七种前端语言 | `web/src/features/system-settings/security/section-registry.tsx`<br>`web/src/i18n/locales/*.json` | `bun run i18n:sync`<br>`src/features/prompt-audit/__tests__/prompt-audit-page.test.tsx` | 七国语言完全同步，i18n:sync 报告 0 缺失 (PASS) | **通过** |
| 11 | 管理 API 仅允许 Root 访问，敏感配置/Token 不泄露 | `controller/prompt_audit.go`<br>`router/api-router.go` | `TestPromptAuditController_RootAuthGuard`<br>`TestPromptAuditController_ConfigLifecycleAndCAS` | 匿名 401、用户/管理员 403、Root 200；Token write-only 不回显 (PASS) | **通过** |
| 12 | 审计事件详情仅授权 Root 解密原文，列表仅返回脱敏预览 | `controller/prompt_audit.go`<br>`service/promptaudit/event_store.go` | `TestPromptAuditController_EventsQueryDetailAndDelete` | 列表不含 full_prompt，详情返回解密原文且带 `Cache-Control: no-store` (PASS) | **通过** |
| 13 | >64 KiB 提示词完整加密落库与逐字符解密还原（跨三库） | `model/prompt_audit.go`<br>`service/promptaudit/envelope.go` | `TestPromptAudit_ThreeDatabaseMatrix` | SQLite (TEXT)、MySQL 8.0 (LONGTEXT)、PG 18 (TEXT) 逐字符比对相等 (PASS) | **通过** |
| 14 | 成功/拒绝/异常场景下，完整提示词/Token 绝不进入应用日志 | `service/promptaudit/logger.go`<br>`service/promptaudit/evaluator.go` | `service/promptaudit/logging_test.go`<br>`TestPromptAuditController_ProbeEndpoint` | 动态日志捕获断言全字段不包含 Canary 敏感词与 Token (PASS) | **通过** |
| 15 | 默认永久保留，有限天数自动清理过期事件且幂等 | `model/prompt_audit.go`<br>`service/promptaudit/cleanup.go` | `TestPromptAudit_ThreeDatabaseMatrix`<br>`TestPromptAuditCleanup_RetentionDays` | retention_days=0 不删，有限天数按 cutoff 批量清理 (PASS) | **通过** |
| 16 | 授权管理员手动单删/批删，且删除后不可再读取 | `controller/prompt_audit.go`<br>`model/prompt_audit.go` | `TestPromptAuditController_EventsQueryDetailAndDelete`<br>`TestPromptAudit_ThreeDatabaseMatrix` | 单删成功、批删限制 1..500 校验严格，删除后返回 RecordNotFound (PASS) | **通过** |
| 17 | “保存通过事件”默认开启；关闭后 Pass 不落库但 Block/Error 必存 | `service/promptaudit/gate.go`<br>`model/prompt_audit.go` | `TestPromptAudit_ThreeDatabaseMatrix`<br>`TestRealtimeAudit_StorePassEventsBehavior` | store_pass=false 时 Pass 0 写入，Block/Error 正常写入 (PASS) | **通过** |
| 18 | `latest_turn_only` 仅改变送审文本范围，完整提示词留存不变 | `service/promptaudit/snapshot.go`<br>`extract_relay.go` | `TestExtractRelayRequest_GeneralOpenAI`<br>`TestExtractRelayRequest_Claude` | `ScanText` 仅取最新轮，`FullPrompt` 包含完整历史上下文 (PASS) | **通过** |
| 19 | 各协议显式提取规则，排除 URL/DataURL/Base64 载荷，不漫游 JSON | `service/promptaudit/extract_*.go` | `TestExtractRelayRequest_*`<br>`TestExtractRelayRequest_URLInPromptNotDropped` | 纯媒体 URL/Base64 正确排除，文本中嵌 URL 正常保留送审 (PASS) | **通过** |
| 20 | Task Plugin 明确声明 `auditTextPaths` 契约，缺失则失败关闭 | `pkg/jsplugin/registry.go`<br>`service/promptaudit/gate.go` | `TestTaskPluginPromptAudit_TenBuiltinPlugins`<br>`TestTaskPluginPromptAudit_ThirdPartyPluginFailClosed` | 十大内置插件显式映射，第三方缺失契约直接 503 拒绝 (PASS) | **通过** |

---

## 三、三数据库真实矩阵实测验证

### 1. 验证环境与容器版本
- **SQLite**: SQLite 3 (内置/纯 Go 驱动 `github.com/glebarez/sqlite`)
- **MySQL**: 官方镜像 `mysql:8.0` (Docker 临时容器 `new-api-test-mysql`，端口 `127.0.0.1:13306`)
- **PostgreSQL**: 官方镜像 `postgres:18-alpine` (Docker 临时容器 `new-api-test-pg`，端口 `127.0.0.1:15432`)

### 2. 验证命令与结果
```bash
go test -v -count=1 -run "PromptAudit.*Matrix" ./model
```
执行输出：
```text
=== RUN   TestPromptAudit_ThreeDatabaseMatrix
=== RUN   TestPromptAudit_ThreeDatabaseMatrix/SQLite-3
=== RUN   TestPromptAudit_ThreeDatabaseMatrix/MySQL-8.0
=== RUN   TestPromptAudit_ThreeDatabaseMatrix/PostgreSQL-18
--- PASS: TestPromptAudit_ThreeDatabaseMatrix (0.20s)
    --- PASS: TestPromptAudit_ThreeDatabaseMatrix/SQLite-3 (0.00s)
    --- PASS: TestPromptAudit_ThreeDatabaseMatrix/MySQL-8.0 (0.10s)
    --- PASS: TestPromptAudit_ThreeDatabaseMatrix/PostgreSQL-18 (0.09s)
=== RUN   TestPromptAudit_UpgradeMatrix
=== RUN   TestPromptAudit_UpgradeMatrix/SQLite-3-Upgrade
=== RUN   TestPromptAudit_UpgradeMatrix/MySQL-8.0-Upgrade
=== RUN   TestPromptAudit_UpgradeMatrix/PostgreSQL-18-Upgrade
--- PASS: TestPromptAudit_UpgradeMatrix (0.17s)
    --- PASS: TestPromptAudit_UpgradeMatrix/SQLite-3-Upgrade (0.00s)
    --- PASS: TestPromptAudit_UpgradeMatrix/MySQL-8.0-Upgrade (0.07s)
    --- PASS: TestPromptAudit_UpgradeMatrix/PostgreSQL-18-Upgrade (0.10s)
PASS
ok  	github.com/QuantumNous/new-api/model	0.866s
```

### 3. 三库矩阵关键点验证确认
1. **初次 AutoMigrate 建表与二次幂等性**: 三个数据库初次建表及重复执行 AutoMigrate 均 100% 成功无报错、无重复索引报错。
2. **Schema 字段类型实测确认**:
   - MySQL 8.0: `INFORMATION_SCHEMA.COLUMNS` 实际数据类型确认为 `longtext`。
   - PostgreSQL 18: `information_schema.columns` 实际数据类型确认为 `text`。
   - SQLite 3: 字段类型确认为 `text`。
3. **>64 KiB 跨库长文本加密与解密一致性**:
   - 写入 70 KiB 包含高位多字节 Unicode、Emoji、特殊控制符的密文，经 AES-256-GCM 落库后读取解密，逐字符断言与原串严格一致。
4. **Option CAS 乐观锁并发与冲突阻断**:
   - 验证 `expectedVersion` 不匹配时立即返回 `ErrPromptAuditConfigConflict` 并回滚事务，杜绝并发覆盖。
5. **事件筛选与稳定分页**:
   - 覆盖用户 ID、分组、协议、模型、决策、风险级别、时间戳范围复合筛选，在三个数据库上均按主键降序稳定分页。
6. **Retention 清理与物理删除**:
   - 验证 `retention_days=0` 永久保留；有限天数对超过截止时间的历史事件按批物理清理，未过期事件完好留存；手动单删及批量删除（1..500）按预期工作。
7. **历史版本升级迁移验证**:
   - 模拟旧版无 `prompt_audit_events` 表的场景，执行 AutoMigrate，已有业务数据完整保留，审计表增量创建成功，二次迁移幂等。

---

## 四、安全与敏感数据防泄漏验证

1. **通用 Option 接口隔离**:
   - 在 `controller/prompt_audit_test.go` 中实测 `TestPromptAuditController_OptionSecretIsolation`：
     - `GET /api/option/` 响应绝对不包含 `PromptAuditConfigSecret` 及敏感 Token。
     - `PUT /api/option/` 尝试修改 `PromptAuditConfigSecret` 返回 HTTP 403 Forbidden 并提示使用专用接口。
2. **节点 Token Write-Only**:
   - 配置查询/更新响应中 `token` 字段全部隐去，仅返回 `has_token: true/false`；
   - 节点探测响应只返回探测结果，绝对不回显探测 Token。
3. **动态日志零泄漏验证**:
   - `service/promptaudit/logging_test.go` 捕获全部 logger 输出，断言日志键值对中绝对不含 `token`、`scan_text`、`full_prompt`，Canary 原文字符串无任何泄漏。
4. **前端 React Query 缓存零残留**:
   - 事件列表 DTO 字段剥离完整原文；
   - 事件详情接口强制输出 `Cache-Control: no-store`；
   - 前端详情抽屉关闭、切换事件或组件卸载时，立即触发 `queryClient.removeQueries` 物理抹除内存缓存。

---

## 五、完整质量命令门禁执行记录

### 1. 后端单元与集成测试（单条超时 <= 55s）
| 测试命令 | 耗时 | 结果 |
| :--- | :--- | :--- |
| `go test -count=1 -timeout 55s ./service/promptaudit` | 0.483s | **PASS** |
| `go test -count=1 -timeout 55s ./model` | 10.169s | **PASS** |
| `go test -count=1 -timeout 55s ./controller` | 1.319s | **PASS** |
| `go test -count=1 -timeout 55s ./middleware` | 0.565s | **PASS** |
| `go test -count=1 -timeout 55s ./pkg/jsplugin` | 0.396s | **PASS** |
| `go test -count=1 -timeout 55s ./relay` | 0.571s | **PASS** |
| `go test -count=1 -timeout 55s ./relay/channel/openai` | 1.699s | **PASS** |
| `go test -race -count=1 -timeout 55s ./relay/channel/openai` | 2.681s | **PASS** |

### 2. 后端静态分析与独立构建
| 检查项 | 命令 | 结果 |
| :--- | :--- | :--- |
| Go 静态检查 | `go vet ./service/promptaudit ./model ./controller ./middleware ./pkg/jsplugin` | **PASS** (0 错误 0 告警) |
| 全仓根构建 | `go build -v ./...` | **PASS** (成功输出二进制) |
| `relaykit` 独立构建 | `cd relaykit && GOWORK=off go build ./...` | **PASS** (零根模块依赖) |

### 3. 前端质量脚本 (`web/`)
| 检查项 | 命令 | 结果 |
| :--- | :--- | :--- |
| 单元与集成测试 | `bun run test` | **PASS** (66 个测试文件、431 项测试通过，18.5s) |
| TypeScript 类型检查 | `bun run typecheck` | **PASS** (`tsgo -b` 0 错误) |
| 提示词审计组件 Lint | `bunx oxlint -c .oxlintrc.json src/features/prompt-audit ...` | **PASS** (0 warnings, 0 errors) |
| 代码格式检查 | `bun run format:check` | **PASS** (1156 files 格式完全一致) |
| 版权信息头检查 | `bun run copyright:check` | **PASS** (1134 files 版权头合规) |
| 多语言同步校验 | `bun run i18n:sync` | **PASS** (七语种文案 100% 对齐) |
| 生产包构建 | `bun run build` | **PASS** (Rsbuild 成功产出 dist) |

---

## 六、缺陷处置与回归修复

在集成验收阶段发现并完成的缺陷修复：
1. **全局数据库测试状态污染修复**:
   - **问题**: `model/prompt_audit_matrix_test.go` 在运行多库测试时覆写全局 `model.DB` 与数据库类型，未在 `t.Cleanup` 中恢复，导致后续依赖全局 SQLite 内存库的其他 model 测试找不到系统表。
   - **修复**: 在 `TestPromptAudit_ThreeDatabaseMatrix` 与 `TestPromptAudit_UpgradeMatrix` 添加 `t.Cleanup` 恢复原始 `DB`、`LOG_DB` 及数据库类型，并在 `controller/prompt_audit_test.go` 与 `controller/prompt_audit_task_plugin_test.go` 同步规范化清理逻辑。
   - **重验**: `go test ./model` 与 `go test ./controller` 全部绿色通过。
2. **通用 Option 接口防泄漏端点补充**:
   - **完善**: 增加端点级直接测试 `TestPromptAuditController_OptionSecretIsolation`，证明 `/api/option` 路由对提示词审计敏感秘密具备强隔离与只写保护。

---

## 七、最终结论

- 提示词审计（Prompt Audit）所有入口链路、配置行锁、事件存储、加密安全、失败关闭与前端管理均符合父任务 PRD 与设计规范；
- SQLite 3、MySQL 8.0、PostgreSQL 18 三数据库真实矩阵全部验证通过；
- 后端与前端全部质量门禁 100% 通过；
- 系统具备向主代理交付并建议父任务 `09-03-prompt-audit` 完成的全部必要条件。
