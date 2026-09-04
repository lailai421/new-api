# 提示词审计只扫描用户输入 — 实施计划

## 实施边界

实现 `prd.md` 与 `design.md` 的完整范围。不得把人设指纹写进代码，不得改失败关闭为超时放行，不得把缓存做到 Redis，不得新增「扫描/保存 system」配置项，不得改事件表 schema，不得回写历史密文，不得在本任务改生产机配置。

本轮规划完成后必须等待用户审核；未获批准前不执行 `task.py start`，不修改业务代码。

## 依赖

父任务领域核心、存储、门禁、LLM 分类器和 cyber_abuse 启发式已存在。本任务改：送审构造、落库正文、Responses 角色映射、空 user 不写事件、Evaluate 缓存/截断、失败耗时、前端说明。

## 阶段 1：用户送审文本、落库正文、预览

### 目标

R1–R4、AC1–AC3、AC12。

### 主要改动

- `snapshot.go`：`JoinUserSegments` 生成 FullPrompt（全部 user、原顺序）；`SelectUserScanSegments` 生成 ScanText；`PromptHash` / `PromptLength` / `MessageCount` 基于 FullPrompt；`RedactedPreview` 基于 ScanText；无 user 不退回全文。
- `types.go`：`AuditScope` 注释改为 `user` | `latest_turn`；`MaxRemoteScanRunes`。
- 热路径停止使用会把人设拼进 FullPrompt 的 `JoinSegments(全部段)`。

### 验证

```bash
go test ./service/promptaudit/ -count=1 -timeout 60s
```

必须覆盖：

- 人设 + 历史 user + 最新 user：FullPrompt / ScanText 都只有 user，人设不在 FullPrompt
- preview 不以人设开头
- `latest_turn_only=true`：ScanText 仅最新 user，FullPrompt 仍含全部 user
- 无 user：FullPrompt 与 ScanText 为空
- 原先「JoinSegments / FullPrompt 含 system」的 snapshot 断言改为「仅 user」；纯提取测试仍可断言分段列表里存在 system（提取 ≠ 落库）

## 阶段 2：Responses 角色映射

### 目标

AC9。工具输出不得变成 user，也不得被抽进落库正文。

### 主要改动

- `extractResponsesInput`：按 `type` 映射；无 role 的 `function_call_output` 不得标成 user。
- **不要**把 `output` / `arguments` 抽成文本分段。
- Codex 形态夹具：`instructions` + user + 无 role 的 `function_call_output`。

### 验证

ScanText 与 FullPrompt 只有 user 句；function_call_output 正文不出现。

## 阶段 3：Evaluate 短路、不写空事件、截断、缓存、耗时

### 目标

AC4–AC8。

### 主要改动

- `guard.go`：空 ScanText → Allow 且不 HTTP；启发式用未截断 ScanText；远程用截断文本；缓存于 `globalSem` 之前。
- `event_store.go` / `gate.go`：Allow/Flag 且 FullPrompt 为空则不 Insert；失败拷贝 `LatencyMS`；密文只加密 FullPrompt（此时已是用户文本）。
- 缓存 TTL 10 分钟，容量 4096，可测时钟，禁止 Sleep。

### 验证

```bash
go test ./service/promptaudit/ ./controller/ -count=1 -timeout 60s
```

必须覆盖：

- 空用户 Allow 后 `prompt_audit_events` 行数为 0
- 有用户的 Allow 在 StorePass=true 时仍落库，解密为人设不在、user 在
- 超 8000 rune 只按截断长度分片，落库长度仍为未截断用户文本
- 缓存命中 / 配置版本失效 / 错误不缓存
- 失败 Record 的 `latency_ms` 非 0

## 阶段 4：前端说明与 i18n

### 目标

AC10。

### 主要改动

- `policy-tab.tsx`：说明改为保存用户提示词，不是整包请求。
- 列表 `audit_scope` 支持 `user`。
- i18n：en / zh / zh-TW / fr / ru / ja / vi。

### 验证

```bash
cd web && bun run i18n:sync
cd web && bun run build
```

更新 `prompt-audit-page.test.tsx` 中相关文案。

## 阶段 5：回归

```bash
go test ./service/promptaudit/ ./controller/ -count=1 -timeout 60s
cd relaykit && GOWORK=off go build ./...
```

无 schema 变更，不做三数据库迁移矩阵。

## 风险文件

- `service/promptaudit/snapshot.go` — 送审与落库正文
- `service/promptaudit/event_store.go` — 加密对象与空事件跳过
- `service/promptaudit/extract_relay.go` — 误把 tool 当 user 会污染密文
- `service/promptaudit/guard.go` — 超时与成本
- `web/src/features/prompt-audit/components/policy-tab.tsx` — 管理员对「保存什么」的理解

## 回滚

回退镜像。旧事件密文保持原样。新代码写入的用户-only 密文用旧代码仍可解密（算法不变，只是明文更短）。

## 批准前检查

- [ ] 用户已审核更新后的 `prd.md` / `design.md` / `implement.md`（含「只保存用户提示词」）
- [ ] 未执行 `task.py start`，未改业务代码
