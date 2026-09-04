# OpenAI Cyber Abuse 调研

调研日期：2026-09-04。用于冻结提示词审计的 `cyber_abuse` 分类契约，不是实施清单。

## 1. OpenAI 把什么叫 Cyber Abuse

公开政策没有单独一篇叫 “Cyber Abuse Policy” 的完整清单。产品侧（Codex 警告邮件、账号执法）把这类违规标成 **Cyber Abuse**。对应到书面条款，核心是 Usage Policies 中的：

> destruction, compromise, or breach of another’s system or property, including malicious or abusive cyber activity or attempts to infringe on intellectual property rights of others

并列相关条款：

- illicit activities, goods, or services
- circumventing our safeguards
- unsolicited safety testing
- 知识产权侵权（破解、盗版工具）

来源：[Usage Policies](https://openai.com/policies/usage-policies/)

Model Spec 的根规则还要求模型不得协助非法行为，包括在用户已表明非法意图时，即使同样的技术信息在其他语境下可以给。

来源：[Model Spec](https://model-spec.openai.com/)

## 2. Codex / API 实际怎么执法

### 2.1 Codex 产品

GPT-5.3-Codex 起按 Preparedness Framework 的 **High Cybersecurity Capability** 处理。额外措施包括：

- 模型拒绝明确恶意请求（如偷凭证）
- **分类器监控** 可疑网络活动信号
- 高风险流量改送到更低网络能力的模型（文档写 GPT-5.2）
- 被 Safety Reasoner 标出的会话会进入自动分析，高风险会人工复核
- 处置：警告、限制前沿网络能力、停用或封号

来源：

- [Cyber Safety – Codex](https://developers.openai.com/codex/concepts/cyber-safety)
- [GPT-5.3-Codex System Card](https://deploymentsafety.openai.com/gpt-5-3-codex)
- [codex-1 System Card](https://cdn.openai.com/pdf/8df7697b-c1b2-4222-be00-1fd3298f351d/codex_system_card.pdf)

codex-1 系统卡写明 Codex 有两套专用监控：

1. **Malware-related prompt monitor**：检测生成被禁止恶意软件相关内容的尝试
2. **Disallowed task monitor**：检测任务/提示词是否违反政策（不限于木马，也包括违法站点等）

### 2.2 API

GPT-5.3-Codex 及更新模型在 API 上也有网络安全检查（与 Codex 产品侧不完全相同）：

- 监控可疑网络安全活动信号
- 超阈值会临时限制访问
- 错误码：`cyber_policy`
- 若未设置每用户 `safety_identifier`，可能整组织被限制
- 设了 `safety_identifier` 时，警告和限制可落到单个终端用户
- 文档写明：重复触发会发警告邮件；继续则停用对应模型访问

来源：

- [Cybersecurity checks](https://developers.openai.com/api/docs/guides/safety-checks/cybersecurity)
- [Safety checks](https://platform.openai.com/docs/guides/safety-checks)

对 new-api 的含义：即使用户“只是代理 Codex/Chat Completions”，**只要请求打到 OpenAI 高能力编码模型，OpenAI 仍会在他们那边再判一次**。网关侧拦截的价值是让脏请求根本到不了 OpenAI，从而减少账号级累计。

### 2.3 警告与封号

Help Center：[Why was my OpenAI account deactivated?](https://help.openai.com/articles/10562188-why-was-my-openai-account-deactivated)

- 违反 Usage Policies / ToS 可停用
- **Repeated Violations Despite Warnings**：先警告，期限内不改就停用
- 用户所述“三次警告封号”与“警告累积后停用”一致，具体次数以 OpenAI 当时执法为准，本方案按 **一次脏请求都尽量不让进上游** 来设计

## 3. OpenAI 已经公开打击过的行为（高召回样本）

以下来自 OpenAI 自己的威胁情报案例。这些不是“写个病毒.exe”这种直球，很多是 **积木式、可组装的双用途片段**，账号照样被封。

| 行为簇 | 公开案例中的具体请求 |
|---|---|
| 恶意软件加载器 | 把编译后的可执行文件转成 shellcode；内存加载器；反射 DLL |
| 免杀 / 绕过 | UAC / SmartScreen / Mark-of-the-Web 绕过；字符串隐藏；函数改名；OPSEC 修饰 |
| 凭证与钱包窃取 | 浏览器 cookie / 密码；Chrome/Edge DPAPI；App-Bound 解密骨架；LevelDB 钱包解析；剪贴板劫持 |
| 远控 | RAT 组件；视频/键鼠模拟；C2 心跳；HTTPS beacon；任务/结果 JSON 信封 |
| Android 恶意软件 | 调试 Android 木马；配套 C2 服务端 |
| 入侵工具 | RDP 爆破客户端；开源 RAT 使用协助；macOS ASEP 持久化 |
| 钓鱼 | 多语言钓鱼信；仿 reCAPTCHA；登录页 HTML 混淆 |
| 漏洞与利用 | 为攻击行动研究漏洞；编写利用链 |
| 增量开发 | 每个账号只问一小步，再换号把功能拼成完整木马（ScopeCreep） |

来源（节选）：

- [Russian-speaking malware tooling](https://openai.com/index/disrupting-malicious-uses-of-ai-russian-speaking-malware-tooling/)
- [Korean-language malware support](https://openai.com/index/disrupting-malicious-uses-of-ai-korean-language-malware-support/)
- [ScopeCreep](https://openai.com/index/disrupting-malicious-uses-of-ai-scopecreep/)
- [STORM-0817](https://openai.com/index/disrupting-malicious-uses-of-ai-storm-0817/)
- [Cyber threat actors](https://openai.com/index/disrupting-malicious-uses-of-ai-cyber-threat-actors/)
- [Phishing and scripting support](https://openai.com/index/disrupting-malicious-uses-of-ai-phishing-and-scripting-support/)

设计推论：分类器不能只认“帮我写木马”。必须把 **加载器、免杀、偷 cookie、C2、爆破、钓鱼页、脱壳打补丁** 以及“帮我把这段进攻代码改好/加密/藏字符串”都标成 Cyber Abuse。

## 4. 双用途：OpenAI 口头允许，执法上仍会标

政策文字区分：

- 希望支持：安全教育、CTF、渗透测试、防御工具、恶意软件分析、威胁情报
- 禁止：网络武器、可部署的恶意代码、未授权入侵

GPT-5.6 系统卡补充：允许双用途中的教育、人工主导的代码漏洞识别、调试、企业安全运营自动化、人工主导的应用安全；禁止高级恶意软件的开发与无差别部署，以及高风险双用途研究（例如对 **活的第三方系统** 做长程代理漏洞研究与链式利用）。

现实执法更严：

- [Codex #30271](https://github.com/openai/codex/issues/30271)：用户声称在 **自己的硬件** 上做逆向和漏洞分析，且已在 chatgpt.com/cyber 验证，仍收到 Codex **Cyber Abuse** 警告
- Cyber Safety 文档承认误报，并提供 Trusted Access（chatgpt.com/cyber）把流量改送到低能力模型，而不是保证不警告

对“保护 Codex 账号”的产品含义：

- “我有授权 / 这是我的设备 / 这是 CTF” **不能当作放行条件**
- 可运行的逆向、脱壳、利用、破解、未授权扫描，即使声称防御，也应在网关侧拦截
- 允许的是概念层安全知识和防御加固，而不是进攻载荷

## 5. 逆向 / 破解 与现有 `jailbreak` 不是一类

现有 `jailbreak` 定义是 **Prompt injection or jailbreak attempt**（LLM 越狱），见 `service/promptaudit/qwen3guard.go`。

OpenAI Cyber Abuse 里用户说的“越狱 / 破解 / 逆向”是：

- 软件逆向、反编译、反汇编、脱壳
- 许可证破解、注册机、补丁、DRM/反作弊绕过
- 设备越狱（iOS jailbreak、Android root、FRP、解锁 bootloader 的利用）
- 游戏内存修改、反作弊对抗

这些必须进 `cyber_abuse`。提示词里要写死：**设备/软件越狱不是 `jailbreak` 分类。**

破解同时可能命中 `copyright_violation`，应允许双标。

## 6. 和现有九类的差距

当前目录（Qwen3Guard 内容安全分类）：

`violent` / `non_violent_illegal_acts` / `sexual_content_or_sexual_acts` / `pii` / `suicide_and_self_harm` / `unethical_acts` / `politically_sensitive_topics` / `copyright_violation` / `jailbreak`

缺口：

- 没有 malware / exploit / C2 / credential theft / reverse engineering / cracking
- LLM 分类器 system prompt 锁死上述 9 个 id，模型被明确要求不要发明新类
- `jailbreak` 升级阻断，但 `non_violent_illegal_acts` 的 Controversial 只 Warn，请求仍会打到 OpenAI
- Qwen3Guard 0.6B 不会稳定输出网络攻击类别

因此：只靠把逆向“挤进”`non_violent_illegal_acts` **不能保护账号**。

## 7. 网关能挡什么、不能挡什么

能挡（本任务范围）：

- 经过 new-api 门禁、可提取为文本的用户提示词
- 在调用 OpenAI/Codex 上游 **之前** 同步 Block

不能挡：

- Codex / ChatGPT 直连 OpenAI、不经过 new-api
- 上游模型回复（父任务已排除响应审计）
- 二进制附件本身（只审文本字段）
- OpenAI 对已放行请求的二次分类、多会话累计、工具轨迹
- 分类器漏检、积木式请求被拆得完全像普通编程

结论：本功能是 **降低打到 OpenAI 的 Cyber Abuse 流量**，不是封号保险。方案必须把这个限制写进产品和页面，避免管理员以为勾了分类就绝对不会被警告。

## 8. 推荐拦截面（供分类契约使用）

### 8.1 必拦（Unsafe + cyber_abuse）

1. 恶意软件与进攻工具：木马、远控、勒索、擦除器、僵尸网络、加载器、加壳用于隐藏恶意代码、内存/shellcode 加载用于进攻、C2/beacon、为恶意软件做的持久化
2. 免杀与防护绕过：AMSI / EDR / AV / UAC / SmartScreen / MotW 绕过，用于让恶意代码跑起来
3. 凭证与资产窃取：偷密码、cookie、token、钱包、剪贴板劫持、键盘记录
4. 未授权访问：爆破、对真实系统写利用、提权用于入侵、钓鱼套件、凭证填充
5. 逆向与破解他人软件/设备：为绕过授权/DRM/反作弊而反编译、脱壳、打补丁、写注册机；设备越狱/root 利用
6. 积木式进攻：把 PE 转 shellcode、反射注入、解析浏览器登录库、给加载器藏字符串；调试/完善用户贴出来的木马、利用、破解器

### 8.2 也拦（Controversial + cyber_abuse，因升级规则等于 Block）

- 未提供可核验授权语境下的：逆向二进制、找 0-day、写 exploit、可运行的渗透载荷、还原可工作恶意软件
- 用户自称“这是我的设备/已授权/CTF”不构成放行

### 8.3 放行（不标 cyber_abuse）

- 安全编码：校验、鉴权、TLS、密钥管理、CSP
- 公开 CVE 的概念解释，不给可运行 PoC
- 普通业务开发：HTTP payload、React hook、依赖注入、反向代理、git revert
- 用官方安全 API（WebAuthn 等）
- 加日志、测试、重构，且不要求生成进攻载荷

## 9. 对实施的约束

- 主检测面必须是 **LLM 分类器的固定 prompt**，不是 Qwen3Guard 九类
- 需要独立 scanner id，且 Controversial 升级为 Block
- 需要中英高置信启发式，覆盖直球样本，减轻 Qwen3Guard-only 部署的盲区；禁止用 `hook`/`payload`/`exploit` 单字误伤正常编程
- 不把分类 prompt 做成管理端可编辑配置（沿用 LLM 分类器任务决议）
- 不承诺 100% 防封号；页面与 PRD 必须写清
