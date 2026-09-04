# 生产证据：开启全文审计后的 Codex 流量

采集时间：2026-09-04。主机 `root@72.60.29.71`。只使用 `prompt_audit_events` 元数据、`redacted_preview` 与 `new-api` 容器日志，未解密 `full_prompt_ciphertext`。

## 窗口

- 事件时间：2026-09-04 07:53:50–07:59:19 UTC（北京时间 15:53:50–15:59:19）
- 条数：281
- 当时配置要点：`latest_turn_only=false`，`store_pass_events=true`，分组 `gpt-pro`，节点 `llm_classifier` + `deepseek-v4-flash`，`timeout_ms=8000`，`input_limit=4000`
- 采集时配置已重新 `enabled=false`

## 流量形态

- 用户全部为 `wjxx-codex`
- 协议全部 `openai_responses`，路径全部 `/v1/responses`，模型全部 `gpt-5.6-sol`
- `audit_scope` 全部 `full`
- 预览三种前缀：`You are Codex, an agent based on GPT-5`（202）、`You are a coding agent running in the Codex CLI`（65）、`You are Codex, a coding agent based on GPT-5`（14）
- 原文长度约 33601–66023 字符；不同 `prompt_hash` 仅 14 个，说明大量重试/工具循环

## 判定

| 判定 | 条数 | HTTP | 日志耗时 |
|---|---|---|---|
| unavailable | 270 | 503 | 全部约 8001–8007ms |
| block / cyber_abuse | 6 | 403 | 约 7.0–7.9s，同一 49429 字符请求重试 6 次 |
| allow | 5 | 200 | 约 6.5–7.2s，最短约 33k 字符 |

库内 unavailable 的 `latency_ms=0`、`chunk_total=0`，因为失败 Decision 没有 `Result`。以容器日志为准。

## 与代码的对应

- 预览用人设开头：`BuildPromptPreview(fullPrompt)`，`fullPrompt` 以 Responses `instructions` 开头
- 超时：全文 / 4000 串行分片，9–17 次分类调用打不满 8s
- 重试成本：无 ScanText 判定缓存
- 6 条 block 的 backend 是 `llm-classifier-openai`，不是本地 `heuristic-cyber-abuse`

## 存储含义

281 条密文按 3.3 万–6.6 万字符计，约等于反复保存同一份 Codex 人设。只保存用户提示词后，单条正文通常会降到实际用户原话长度；无 user 的工具循环 Allow 不再写行。

上线前已入库的密文本任务不回写，需管理员删除或等保留期。

## 对方案的约束

不要用人设字符串匹配来跳过扫描。要用 user 角色过滤。不要靠加大超时来「顶过去」，那会继续为重复人设付费。不要把人设/工具结果继续写入 `full_prompt_ciphertext`。
