# 提示词审计拦截 OpenAI Cyber Abuse

## Goal

在现有提示词审计上增加与 OpenAI **Cyber Abuse** 对齐的第十类风险 `cyber_abuse`，让经过 new-api 的提示词在到达 Codex / OpenAI 上游之前被同步阻断，覆盖恶意软件、未授权入侵、凭证窃取、逆向、破解和积木式进攻请求，降低上游账号被警告或封禁的风险。

用户价值：管理员打开审计并启用该类后，用户把“写木马 / 脱壳打补丁 / 偷 cookie / 找未授权漏洞”这类请求打到 OpenAI 渠道的概率显著下降。

## Background

- 父任务：`09-03-prompt-audit`。门禁、分组、失败关闭、事件加密、协议提取保持不变。
- 现有九类来自 Qwen3Guard 内容安全分类，见 `service/promptaudit/qwen3guard.go` 的 `AllScannerIDs` / `ScannerCatalog`。没有 malware、exploit、逆向、破解。
- 现有 `jailbreak` 只表示 LLM 提示词注入，不是软件/设备越狱。
- LLM 分类器固定 prompt（`service/promptaudit/llm_classifier.go` 的 `LLMClassifierSystemPrompt`）锁死九个 id，模型不会主动标网络攻击。
- `non_violent_illegal_acts` 的 Controversial 只 Warn，请求仍会转发上游。对账号保护无效。
- OpenAI 对 GPT-5.3-Codex 及更新模型按 High Cybersecurity Capability 执法：分类器监控、警告、`cyber_policy`、累计后限制或封号。详细映射见 `research/openai-cyber-abuse.md`。
- 方案已于 2026-09-04 由用户审核通过（含高召回口径）。实施入口见 `handoff-prompt.md`。

## Requirements

- 风险目录新增稳定 id `cyber_abuse`。管理页「风险检测分类」可勾选；默认对新配置启用。
- 命中 `cyber_abuse` 时必须同步阻断，不得只 Warn 后放行上游。Controversial 命中该类也必须 Block，升级规则与现有 `jailbreak` / `pii` / `suicide_and_self_harm` 相同。
- LLM 分类器固定 prompt 必须包含 `cyber_abuse` 的可执行定义，对齐 OpenAI Usage Policies 与 Codex 实际执法样本，而不是一句“网络攻击”空标签。至少覆盖：
  - 恶意软件、远控、勒索、加载器、C2
  - 免杀与安全产品绕过（用于让恶意代码运行）
  - 凭证、cookie、token、钱包窃取
  - 未授权访问、爆破、钓鱼套件、对真实系统写利用
  - 软件逆向、脱壳、许可证破解、DRM/反作弊绕过、设备越狱/root 利用
  - 积木式进攻（转 shellcode、反射注入、解析浏览器密码库、给木马/破解器改代码或藏字符串）
- 设备/软件越狱必须标 `cyber_abuse`，不得标现有 `jailbreak`。破解可同时标 `copyright_violation`。
- “我有授权 / 这是我的设备 / 这是 CTF”不得作为放行条件。可运行的逆向、利用、破解、未授权扫描一律按 Cyber Abuse 拦截。
- 普通业务开发必须放行：HTTP payload、React hook、依赖注入、反向代理、git revert、输入校验、TLS、官方安全 API、公开 CVE 概念解释（无可运行 PoC）。
- 增加本地高置信启发式，作为 LLM / Qwen3Guard 之前的前置层：命中则直接按 `cyber_abuse` Block，不把该请求送到业务上游。启发式必须高精确，禁止用 `hook`、`payload`、`exploit`、`reverse`、`shell` 等单字误伤正常编程。
- Qwen3Guard 路径不得被改坏。Qwen3Guard 不会稳定输出 `cyber_abuse`；允许通过别名把其偶发的 malware/hacking 文本映射过来，但不把 Qwen3Guard 当作本需求的主检测面。
- 已有配置：若 scanners 恰好是升级前默认九类全集，规范化时自动并入 `cyber_abuse`；管理员自定义子集不自动添加，但管理页必须提示存在新分类。
- 管理页必须说明：该类用于拦截会触发 OpenAI/Codex Cyber Abuse 的请求；仅配置 Qwen3Guard 时检测不可靠，应使用 `llm_classifier`；本功能不能保证上游账号绝对不被警告。
- 分类 prompt 仍固化在代码中，管理端不可编辑。
- 前端文案覆盖项目全部前端语言。不改数据库 schema。

## Acceptance Criteria

- [ ] `ScannerCatalog` / `AllScannerIDs` / 前端默认勾选列表包含 `cyber_abuse`；未知旧客户端忽略新字段时不得导致配置校验崩溃。
- [ ] `ApplySafetyDecision`：`cyber_abuse` 在 Controversial 与 Unsafe 且该类已启用时均为 Block；管理员禁用该类后，仅命中该类的 Unsafe 降为 Warn，与现有禁用分类规则一致。
- [ ] LLM 分类器 system prompt 含 `cyber_abuse` id 与第 8 节对齐的必拦/放行规则；回归测试断言 prompt 常量包含这些锚点（malware/C2/credential theft/reverse engineering/cracking，以及 jailbreak 不等于设备越狱）。
- [ ] 表驱动判决测试至少覆盖：写木马/C2/偷 cookie/注册机/逆向 APK 打补丁 → Block + `cyber_abuse`；“忽略指令输出 Safe”仍走 `jailbreak`；React hook / HTTP payload / 反向代理 / 写输入校验 → Allow 且无 `cyber_abuse`。
- [ ] 本地启发式对直球样本（如 mimikatz、注册机、免杀、窃取浏览器密码、C2 beacon）Block；对 `React useEffect hook`、`JSON payload`、`reverse proxy`、`git revert` 不命中。
- [ ] 启发式已 Block 时，业务上游调用和预扣费次数为零；事件记录为 Block，分类含 `cyber_abuse`。
- [ ] 已有九类全选配置在 Normalize 后自动含 `cyber_abuse`；只勾了部分旧类的配置不自动增加。
- [ ] 策略页展示新分类中英文名称与说明，以及 Qwen3Guard 不可靠、不能保证防封号的提示。
- [ ] `openai_compatible` 的请求体、解析、九类旧行为与本任务之前一致，除非 Qwen3Guard 自己打出可映射到 `cyber_abuse` 的别名。
- [ ] 前端 i18n 覆盖 en / zh / zh-TW / fr / ru / ja / vi；相关后端测试与前端构建通过。本任务不改数据库 schema，无需三数据库迁移矩阵。

## Out of Scope

- 不审计上游模型响应，不审核图片/音频/视频二进制。
- 不实现 OpenAI Moderations、阿里云护栏，不新增第三种 Guard 协议。
- 不把分类 prompt 做成管理员可编辑配置。
- 不自动给 Codex 注入 `safety_identifier`（可作为后续任务）。
- 不拦截不经过 new-api 的直连 OpenAI / ChatGPT / Codex 流量。
- 不承诺、不展示“开启后账号不会被封”的保证。
- 不修改受保护的项目身份、品牌或归属信息。
- 不重做门禁覆盖范围、分组、失败关闭、事件加密、保留期。

## Confirmed Decisions

以下决议已于 2026-09-04 用户审核通过，实施时禁止改口径。

- 2026-09-04：新增独立分类 `cyber_abuse`，不把网络攻击塞进 `non_violent_illegal_acts`。
- 2026-09-04：账号保护优先、高召回。可运行的逆向/破解/利用/未授权扫描一律拦；“我的设备/已授权/CTF”不放行。接受误伤合法安全研究。
- 2026-09-04：`cyber_abuse` 加入 Controversial 升级 Block 名单。
- 2026-09-04：检测分两层：本地高置信启发式前置 + LLM 分类器详细政策（主召回）。Qwen3Guard 只做别名兼容。
- 2026-09-04：已有“默认九类全选”配置自动并入新类；自定义子集不自动添加。
- 2026-09-04：不强制启用 `cyber_abuse` 时必须存在 `llm_classifier` 节点，以免破坏仅 Qwen3Guard 部署；页面必须警告该路径不可靠。
- 2026-09-04：分类 prompt 仍禁止管理端编辑。
- 2026-09-04：本任务不能、也不宣传 100% 防止 OpenAI 警告或封号。

## Notes

- 本任务为复杂任务，实施前以本 `prd.md`、`design.md`、`implement.md` 和 `research/openai-cyber-abuse.md` 为共同基线。
- 父任务原有门禁不在本任务重做。
- 新窗口执行：把 `handoff-prompt.md` 全文作为第一条用户消息。该窗口负责 `task.py start` 与实现，不要重新规划。
