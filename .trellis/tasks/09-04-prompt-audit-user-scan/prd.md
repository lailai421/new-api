# 提示词审计只扫描用户输入

## Goal

让提示词审计在开启时仍然同步阻断违规**用户输入**，并且只把用户提示词送给 Guard、只把用户提示词加密落库。Codex CLI 等客户端内置人设、system、developer、assistant、工具结果既不送审也不入库。

用户价值：团队继续用 Codex CLI 时，可以打开审计，而不被 DeepSeek 费用、8 秒超时 503、以及人设/工具结果把事件表撑爆。

## Background

- 父任务：`09-03-prompt-audit`。门禁、分组、失败关闭、事件加密算法保持不变。
- 生产（`72.60.29.71`，2026-09-04 15:53–15:59 CST）开启审计后 5.5 分钟写入 281 条事件：全部来自 `wjxx-codex` 的 `/v1/responses`；预览 100% 以 `You are Codex` / `You are a coding agent running in the Codex CLI` 开头；270 条 `prompt_guard_unavailable`（容器日志 `latency_ms≈8001`）返回 503；6 条 `cyber_abuse` 403；5 条最短请求放行。原文长度约 3.3 万–6.6 万字符，主要是人设和上下文。详见 `research/production-evidence.md`。
- 根因：默认把 `instructions` + 全文切片送 `llm_classifier`，并把整包原文加密进 `full_prompt_ciphertext`。人设每条请求重复，工具循环反复落库，成本和存储都会顶不住。
- 用户已确认：扫描范围为**该请求里全部用户输入（含历史 user），不含人设**；**落库同样只保存用户提示词**，不要做人设指纹名单。
- 本轮只写方案。用户审核并明确批准前，不执行 `task.py start`，不修改业务代码。

## Requirements

- R1. 默认送审文本只包含本请求已提取分段中 `role=user`（或等价 `User=true`）的全部内容，含多轮历史 user。不得把 system、developer、assistant、tool、Responses `instructions`、function_call、function_call_output 送给远程 Guard。
- R2. 「仅审计最新用户输入」开关继续存在且默认关闭。开启后只送审最新一轮连续 user 分段，仍不含人设、assistant、工具结果。不得再把「找不到 user 就退回全文」当作兜底。
- R3. 无用户输入（例如纯工具循环、只剩人设）时：不调用远程 Guard，不因缺 user 而 503；按 Allow 进入后续转发。此类 Allow **不得写事件**（即使 StorePass 开启），避免空壳或人设行堆积。
- R4. 事件加密正文、详情解密结果、`prompt_hash`、`prompt_length`、`message_count` 只反映该请求的用户提示词（默认含全部历史 user）。不得把人设、system、developer、assistant、工具结果写入 `full_prompt_ciphertext`。列表预览来自实际送审的用户文本。`latest_turn_only` 只缩小送审范围，不把已提取的其他 user 段落从落库中丢掉。
- R5. 相同送审文本在同一配置版本下，短时间内的 Allow / Flag / Block 判定必须能命中进程内缓存，避免 Codex 重试和工具循环重复打 DeepSeek。不可用、超时、非法响应不得缓存。缓存不得保存提示词原文。
- R6. 送入远程分类器的文本必须有明确上限；超出时截断后再调用，不得按用户全文无限切片。本地启发式仍可使用未截断的用户送审文本。
- R7. Guard 失败关闭事件必须写下真实耗时。不得再出现容器日志约 8000ms、库内 `latency_ms=0` 的不一致。失败事件若有用户提示词，只保存用户提示词。
- R8. Responses 输入项必须按 item `type` / `role` 区分 user 与非 user。禁止把无 role 的 function_call_output 等项默认当成 user 送审或落库。不要为了落库去抽取工具 `output` / `arguments` 全文。
- R9. 不新增「把人设重新纳入扫描或落库」的管理开关。不把超时默认值、分片默认值当作本任务的主修复手段。失败关闭策略不变。不回写、不迁移已落库的历史密文。
- R10. 前端说明必须写清：默认扫描并保存该请求全部用户提示词，排除客户端人设与工具结果；「仅审计最新轮」只改变送审范围，详情里仍是该请求全部 user。文案覆盖项目全部前端语言。
- R11. 本任务修正父任务：默认送审与默认落库都改为「该请求全部 user 输入」，不再要求把 system/工具结果整包入库。

## Acceptance Criteria

- [ ] AC1. 构造含 Responses `instructions`（Codex 人设）+ 多轮 user/assistant/tool 的请求时，远程 Guard 收到的文本不含人设、assistant、工具结果，但包含该请求全部 user 段落。
- [ ] AC2. 列表 `redacted_preview` 来自用户送审文本；人设在前、用户在后时，预览不得以 `You are Codex` / `You are a coding agent running in the Codex CLI` 开头（除非用户自己那么写）。
- [ ] AC3. `latest_turn_only=false` 时 `audit_scope=user`；为 true 时 `audit_scope=latest_turn` 且只把最新一轮 user 送审。详情解密正文含该请求全部 user，**不得**含人设、assistant、工具结果。
- [ ] AC4. 只有 system/instructions、没有 user 时，Evaluate 不打 Guard HTTP，返回 Allow，业务上游可调用，且 **不插入** `prompt_audit_events` 行。
- [ ] AC5. `latest_turn_only=true` 且没有 user 时，不得退回扫描或落库人设全文。
- [ ] AC6. 同一 `config_version` + 同一送审文本的第二次 Allow/Block 不发起 Guard HTTP；修改配置版本后必须重新调用。超时/不可用结果不得被第二次请求当成成功缓存。
- [ ] AC7. 用户送审文本超过远程上限时只截断送远程，不按未截断长度无限分片；本地启发式仍能命中未截断用户文本中的规则。落库保存未截断的用户提示词。
- [ ] AC8. Guard 超时或不可用时，事件 `latency_ms` 为实际耗时（毫秒级，允许与墙钟有小误差），不得恒为 0；密文仍只有用户提示词。
- [ ] AC9. Responses 中 `type=function_call_output` 且无 `role` 的项不进入 ScanText，也不出现在解密正文。
- [ ] AC10. 管理页说明与 i18n（en / zh / zh-TW / fr / ru / ja / vi）表述：保存的是用户提示词，不是整包请求。无新配置项、无数据库 schema 变更。
- [ ] AC11. 现有失败关闭、分组、加密算法、Qwen3Guard / LLM 分类器协议、Probe 行为不被本任务改坏；原先假定 FullPrompt 含 system 的测试改为断言只含 user。
- [ ] AC12. `prompt_hash` / `prompt_length` / `message_count` 按落库的用户提示词计算，不按含人设的整包请求计算。

## Out of Scope

- 不维护 Codex / Claude Code / Cursor 等人设指纹或前缀黑名单。
- 不把超时改到 30s、不把 `input_limit` 改到十万作为主方案。
- 不把失败关闭改成超时放行。
- 不把扫描缓存做到 Redis，不把提示词或 ScanText 原文写入缓存。
- 不新增「扫描或保存 system/人设」管理开关。
- 不审计上游模型响应，不审核图片/音频/视频二进制。
- 不改变事件表结构、加密方案、分组匹配、保留期。
- 不解密、不回写、不批量清理本任务上线前已经入库的历史密文（管理员可用现有删除/保留期处理）。
- 不在本任务切换生产配置或重新开启生产审计。
- 不修改受保护的项目身份、品牌或归属信息。

## Confirmed Decisions

- 2026-09-04：扫描范围选「该请求全部 user 输入（含历史 user），不含人设」，不是「只审最新一轮 user」。
- 2026-09-04：用角色/协议字段排除人设，不用 Codex 文案指纹。
- 2026-09-04：`latest_turn_only` 保留，作为更窄的可选送审模式；默认 false 时扫描全部 user。
- 2026-09-04：无 user 时跳过远程 Guard 并 Allow，避免工具循环 503。
- 2026-09-04：进程内短缓存 Allow/Flag/Block；不缓存失败；不落 Redis。
- 2026-09-04：远程送审设上限并截断；本地启发式用未截断用户文本。
- 2026-09-04：修复失败事件耗时落库。
- 2026-09-04：落库只保存用户提示词；人设、system、developer、assistant、工具结果不入库。`latest_turn_only` 不减少已提取 user 段落的落库。
- 2026-09-04：无用户输入的 Allow 不写事件。
- 2026-09-04：不上线时回写历史 281 条已含人设的密文。
- 2026-09-04：本轮只写方案，批准前不写业务代码。

## Notes

- 本任务为复杂任务，实施前以本 `prd.md`、`design.md`、`implement.md` 为共同基线。
- 本任务修正父任务 `09-03-prompt-audit` 的默认送审范围和默认落库范围。
- 生产证据与字段锚点见 `research/production-evidence.md`。
