# 提示词审计仅允许 Codex CLI — 技术设计

## 1. 设计目标

在不增加配置和数据库状态的前提下，把 Codex CLI 兼容性检查作为提示词审计的前置门禁：全局审计关闭时无影响；开启时非 Codex CLI 统一 503，并保证 Guard、事件、计费及业务上游调用均为 0。

## 2. 边界与不变量

| 保持不变 | 本任务改变 |
|---|---|
| 审计 `off` / `blocking` 两态 | `blocking` 开启后增加 Codex CLI 前置门禁 |
| 配置 degraded、Guard 异常继续失败关闭 | 新增 `prompt_audit_codex_cli_required` 503 |
| 分组仅决定合法 Codex 请求是否送审 | 非 Codex 限制早于分组匹配 |
| Prompt 提取、Snapshot、分类和事件结构 | 非 Codex 不再进入这些阶段 |
| 计费、渠道选择、重试语义 | 非 Codex 在这些阶段之前短路且禁止重试 |
| Realtime 提示词仍逐帧审计 | Realtime 客户端身份在 Upgrade 前检查 |

不新增 Option、数据库列、前端设置或 `relaykit/` API。

## 3. 身份识别契约

`service/promptaudit` 单点拥有纯判定函数和客户端前置门禁：

1. 从原始入站请求读取 `Originator`；请求或 Header 缺失视为不匹配。
2. `strings.TrimSpace` 后做 ASCII 小写。
3. 仅完整匹配 `codex_cli_rs` 或 `codex-cli`。
4. 不读取请求体，不解析 User-Agent，不使用 Session/Thread/X-Codex 头回退。
5. 不在日志或错误中输出实际头值。

`Originator` 是自报信号，不是鉴权凭据。显式允许集合优于正则/子串，可以防止误接受 `my-codex_cli_rs-wrapper`，也便于官方标识变更时通过代码审查和测试更新。

## 4. 统一门禁结果

客户端前置门禁返回现有领域错误载体，至少包含：

- Code：`prompt_audit_codex_cli_required`
- HTTPStatus：503
- Retryable：false
- Cause/安全消息：`Prompt audit is enabled; only Codex CLI requests are accepted.`

判定顺序统一为：

```text
Manager 不存在             → 视为审计未启用，保持原行为
配置 degraded              → 保持既有 prompt_audit_config_degraded 503
ActiveConfig.Enabled=false → 保持原行为
Originator 非允许值         → prompt_audit_codex_cli_required 503
Originator 允许             → 继续既有分组与审计流程
```

配置 degraded 优先于客户端错误，避免错误身份提示掩盖审计系统故障。

## 5. 数据流

### 5.1 普通 HTTP / Midjourney / Task / Task Plugin

```text
TokenAuth / Distribute
        │
        ▼
现有协议 gate
        │
        ├─ Manager nil / audit off → 原行为
        ├─ degraded                → 原 503
        ├─ 非 Codex Originator      → 新 503，立即返回
        └─ Codex Originator         → group match → Extract → Snapshot
                                               → Evaluator → EventStore
                                               → billing → upstream
```

四类 gate 在相同位置调用同一前置门禁，不复制 header 值判断。Task 的 `ContextKeyTaskAuditDone` 仍保留最前面的内部幂等短路；该标记只能在一次合法审计完成后写入。

### 5.2 Realtime

```text
GET /v1/realtime
        │
        ▼
统一客户端前置门禁
        ├─ 非 Codex / degraded → HTTP JSON 503（尚未 Upgrade）
        └─ off / Codex         → WebSocket Upgrade
                                  └─ 原 ShouldAuditRealtime / 帧级审计
```

身份检查不尝试审计握手中的占位请求，也不替代帧级提示词审计。合法 Codex Realtime 的上游连接延迟策略保持不变。

## 6. 协议错误映射

- OpenAI Relay：`error.code=prompt_audit_codex_cli_required`，`error.message` 使用固定安全消息，503，SkipRetry。
- Claude Relay：沿用 `{type:"error", error:{...}}` 外壳，错误码和固定消息一致。
- Midjourney：沿用 `code/description/result/type=prompt_audit_error` 外壳，HTTP 503；description 使用固定消息，code/result 保留稳定错误码。
- Task：沿用 `TaskError`，`Code` 为稳定错误码、`Message` 为固定消息、`StatusCode=503`、`LocalError=true`。
- Task Plugin Responses Bridge：在现有错误映射 switch 中加入新错误码，输出 OpenAI 风格 503 和固定消息。
- Realtime 未升级场景：使用普通 OpenAI HTTP JSON 错误，不调用 WebSocket writer。

## 7. 测试设计

### 7.1 纯识别测试

确定性表格覆盖：两个允许值、大小写、首尾空白、空值、未知值、子串伪装、只有 User-Agent/Session/X-Codex 头。只断言公开行为，不锁定函数内部实现。

### 7.2 Gate 顺序测试

- off + 无 Originator：放行，Evaluator/EventStore 为 0。
- enabled + 非 Codex + group mismatch：503，证明身份限制早于分组。
- enabled + Codex + group mismatch：放行且不送审，证明分组语义未改变。
- enabled + Codex + group hit：进入原 Evaluator 流程。
- degraded + 任意客户端：仍返回 config degraded。

### 7.3 跨入口测试

HTTP Relay、Midjourney、Task、Task Plugin 分别断言 503/错误结构，并断言 Evaluator、EventStore、预扣费、业务上游均为 0。Realtime 用 controller 级测试证明 503 在 Upgrade 前产生；不能只测纯函数。

### 7.4 回归夹具

现有用于验证 Allow/Block/Guard 错误/事件写入的审计测试请求补充合法 `Originator`。新非 Codex 用例明确不设置或设置未知值。禁止通过把 Manager 设为关闭、修改分组或弱化断言来让旧测试通过。

## 8. 兼容、上线与回滚

- 行为变化是有意的全局收紧：启用审计后，Claude Code、Cursor、curl、自建 SDK、网页 Playground 及其他非 Codex 客户端的受保护提交会得到 503。
- 非提交 API 不受影响；管理员仍可关闭审计恢复原兼容范围。
- 无数据库迁移和历史数据处理，部署采用普通应用滚动发布。
- 回滚只需移除客户端门禁、错误码与对应测试夹具变更；不会留下持久化数据副作用。

## 9. 风险与处置

| 风险 | 处置 |
|---|---|
| Originator 可伪造 | 明确定位为兼容性门禁，不宣传为安全认证；防伪另建任务 |
| 官方 Codex 改标识 | 显式允许集合 + 契约测试；出现新值时做受审兼容变更，不使用通配符 |
| 忘记某个审计入口 | 以五类入口清单逐项测试，并检索所有 `promptaudit.Check*` / `ShouldAuditRealtime` 调用 |
| Realtime 已 Upgrade 才报错 | controller 级测试必须验证 HTTP 503 和零 WebSocket 连接 |
| 旧测试被新门禁提前截断 | 下游行为夹具统一加入合法 Originator，并继续断言 Evaluator/事件/计费 |
| 503 被渠道重试 | Relay 错误保持 SkipRetry，Task 错误保持 LocalError；测试断言上游调用为 0 |

