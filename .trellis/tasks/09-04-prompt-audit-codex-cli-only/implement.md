# 提示词审计仅允许 Codex CLI — 实施计划

## 实施边界

完整实现 `prd.md` 与 `design.md`。不得新增可配置白名单、不得用 User-Agent 或请求体内容替代 `Originator`、不得实现伪安全认证、不得改数据库、不得改 Guard/Prompt 提取/计费规则、不得修改生产环境配置。

规划完成后必须等待用户审核；未获得对本版方案的明确批准前，不运行 `task.py start`，不修改业务代码。

## 阶段 1：建立单点客户端识别与错误契约

### 目标

实现 R1–R3、R8、R10 的领域契约。

### 主要改动

- `service/promptaudit/types.go`：新增稳定错误码 `ErrorCodeCodexCLIRequired` 和固定安全消息的单一来源。
- `service/promptaudit/`：新增可供 controller 与各 gate 复用的客户端识别/前置门禁函数。
- 允许值仅为规范化后的 `codex_cli_rs`、`codex-cli`；完整值匹配，不做子串、正则或 User-Agent 回退。
- 使用现有错误载体表达 code/status/message/retryable，不为一个状态再创建平行错误体系。

### 测试

- 表格测试覆盖允许值、大小写/空白、空值、未知值、子串伪装和辅助头不可授权。
- 审计关闭、Manager nil、配置 degraded 的优先级分别有明确断言。

## 阶段 2：接入 HTTP、Midjourney 与 Task 门禁

### 目标

实现 R4–R6、R8–R10 的非 Realtime 路径。

### 主要改动

- `service/promptaudit/gate.go`：在确认配置开启之后、分组判断和 Prompt 提取之前，将统一门禁接入 `CheckRelayRequest`、`CheckMidjourneyRequest`、`CheckTaskRequest`、`CheckTaskPluginProtocolRequest`。
- 扩展现有 Relay/Midjourney/Task 错误包装，使新错误返回固定安全消息；保持 Relay SkipRetry 和 Task LocalError。
- `controller/plugin_protocol.go`：在 Task Plugin 审计错误映射中加入新错误码和固定消息。
- 不记录被客户端门禁拒绝的 `prompt_audit_events`，因为请求未形成 PromptSnapshot、也未送审。

### 测试

- 每个入口至少一个非 Codex 503 用例。
- group mismatch 的非 Codex 仍 503；合法 Codex 仍按分组放行。
- 每个拒绝用例断言 Evaluator=0、EventStore=0；controller 级用例继续断言预扣费和业务上游=0。
- 响应体和日志不得包含请求头、Prompt 或 Token。

## 阶段 3：Realtime 在 Upgrade 前拦截

### 目标

实现 R7，并保持合法 Codex 的原帧级审计。

### 主要改动

- `controller/relay.go`：在 `upgrader.Upgrade` 之前调用统一客户端前置门禁，并通过现有 OpenAI HTTP 错误外壳返回 503。
- 保持 `CheckRelayRequest` 对 Realtime 占位请求的既有跳过；合法 Codex 仍由 `ShouldAuditRealtime`、延迟上游拨号和 `CheckRealtimeEvent` 审计文本帧。
- 新门禁返回后不得调用 WSS error writer，因为连接尚未升级。

### 测试

- controller 级 Realtime 请求无/错误 Originator：HTTP 503、稳定错误码/消息、没有 101 Upgrade。
- 断言客户端和业务上游 WebSocket 拨号均为 0。
- 合法 Codex Realtime 的既有延迟拨号、Block、Guard 503 和成功转发测试继续通过。

## 阶段 4：更新审计测试夹具并做回归审查

### 主要改动

- 对所有意在验证客户端门禁之后行为的 gate/controller/realtime/task 测试请求补充标准 `Originator`。
- 新非 Codex 测试明确保留空值或未知值。
- 检索所有 `CheckRelayRequest`、`CheckMidjourneyRequest`、`CheckTaskRequest`、`CheckTaskPluginProtocolRequest`、`ShouldAuditRealtime` 调用，确认入口覆盖与无重复字符串判定。

### 审查门

- 不允许因新门禁而删除原有 Block、Allow、失败关闭、事件写入或 Realtime 回归断言。
- 不允许将识别逻辑散落到 controller、router 或各协议错误映射中。
- 不允许记录实际 Originator/User-Agent。

## 阶段 5：验证

每条单元测试命令都设置不超过 55 秒的超时：

```bash
go test ./service/promptaudit -count=1 -timeout 55s
go test ./controller -count=1 -timeout 55s -run 'TestRelay.*PromptAudit|TestRelayMidjourney.*PromptAudit|Test.*TaskPlugin.*Audit|Test.*CodexCLI'
go test ./relay/channel/openai -count=1 -timeout 55s -run 'Test.*Realtime.*Audit|Test.*PromptAudit'
```

格式化、静态编译和模块边界：

```bash
gofmt -w <本任务修改的 Go 文件>
go test ./service/promptaudit ./controller ./relay/channel/openai -count=1 -timeout 55s
go build ./...
cd relaykit && GOWORK=off go build ./...
```

本任务不修改数据库、GORM、迁移、模型或持久化格式，因此不触发 SQLite/MySQL/PostgreSQL 三数据库矩阵；若实施中实际触及这些范围，必须回到规划并补充完整数据库验证。

## 风险文件

- `service/promptaudit/types.go` — 稳定错误契约。
- `service/promptaudit/gate.go` — HTTP/Midjourney/Task/Task Plugin 审计热路径。
- `service/promptaudit/gate_realtime.go` — Realtime 生效条件与回归边界。
- `controller/relay.go` — WebSocket Upgrade 前置顺序、HTTP 错误输出。
- `controller/plugin_protocol.go` — Responses Bridge 错误映射。
- `service/promptaudit/*gate*_test.go`、`controller/*prompt_audit*_test.go`、`relay/channel/openai/*audit*_test.go` — 现有审计行为夹具。

## 回滚点

1. 移除统一客户端门禁调用和新错误映射。
2. 删除新错误码/消息及新增测试，恢复仅为该门禁补充的测试 Originator。
3. 重新执行原提示词审计回归。没有数据库或事件数据需要回迁。

