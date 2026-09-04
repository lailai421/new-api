package promptaudit

import (
	"strings"
)

// CyberAbuseRule 描述单条网络滥用启发式匹配规则。
type CyberAbuseRule struct {
	Name    string // 规则名或短标签（作为审计证据，不回显用户原文）
	Pattern string // 规范化后的小写匹配子串
}

// CyberAbuseRules 是本地高置信启发式规则表。
// 规则原则：高精确度、覆盖典型恶意软件、攻击工具、凭证窃取与逆向破解，严禁使用通用单词单字误伤正常编程。
var CyberAbuseRules = []CyberAbuseRule{
	// 1. 工具与家族名（直球命中）
	{Name: "tool:mimikatz", Pattern: "mimikatz"},
	{Name: "tool:cobalt_strike", Pattern: "cobalt strike"},
	{Name: "tool:meterpreter", Pattern: "meterpreter"},
	{Name: "tool:bloodhound", Pattern: "bloodhound"},
	{Name: "tool:sliver_c2", Pattern: "sliver c2"},
	{Name: "tool:empire_c2", Pattern: "empire c2"},
	{Name: "tool:impacket_secretsdump", Pattern: "impacket secretsdump"},

	// 2. 中文直球攻击与黑产术语
	{Name: "cn:注册机", Pattern: "注册机"},
	{Name: "cn:免杀", Pattern: "免杀"},
	{Name: "cn:过杀软", Pattern: "过杀软"},
	{Name: "cn:远控木马", Pattern: "远控木马"},
	{Name: "cn:勒索软件", Pattern: "勒索软件"},
	{Name: "cn:窃取cookie", Pattern: "窃取cookie"},
	{Name: "cn:窃取_cookie", Pattern: "窃取 cookie"},
	{Name: "cn:浏览器密码", Pattern: "浏览器密码"},
	{Name: "cn:脱壳", Pattern: "脱壳"},
	{Name: "cn:破解补丁", Pattern: "破解补丁"},
	{Name: "cn:卡密生成", Pattern: "卡密生成"},
	{Name: "cn:绕过drm", Pattern: "绕过drm"},
	{Name: "cn:绕过_drm", Pattern: "绕过 drm"},

	// 3. 英文多词高置信短语（避免单字误伤）
	{Name: "en:license_keygen", Pattern: "license keygen"},
	{Name: "en:license_crack", Pattern: "license crack"},
	{Name: "en:drm_bypass", Pattern: "drm bypass"},
	{Name: "en:amsi_bypass", Pattern: "amsi bypass"},
	{Name: "en:uac_bypass", Pattern: "uac bypass"},
	{Name: "en:smartscreen_bypass", Pattern: "smartscreen bypass"},
	{Name: "en:mark_of_the_web", Pattern: "mark of the web"},
	{Name: "en:reflective_dll", Pattern: "reflective dll"},
	{Name: "en:shellcode_loader", Pattern: "shellcode loader"},
	{Name: "en:c2_beacon", Pattern: "c2 beacon"},
	{Name: "en:command_and_control", Pattern: "command and control"},
	{Name: "en:steal_cookies", Pattern: "steal cookies"},
	{Name: "en:dump_credentials", Pattern: "dump credentials"},
	{Name: "en:browser_dpapi", Pattern: "browser dpapi"},
	{Name: "en:chrome_login_data", Pattern: "chrome login data"},
	{Name: "en:reverse_engineer_this", Pattern: "reverse engineer this"},
	{Name: "en:decompile_this_apk", Pattern: "decompile this apk"},
	{Name: "en:unpack_this_binary", Pattern: "unpack this binary"},
}

// MatchCyberAbuseHeuristics 对输入文本执行全文大小写不敏感的本地启发式匹配。
// 返回命中的规则名称和是否命中布尔值。
func MatchCyberAbuseHeuristics(text string) (string, bool) {
	if text == "" {
		return "", false
	}
	lower := strings.ToLower(text)
	for _, rule := range CyberAbuseRules {
		if strings.Contains(lower, rule.Pattern) {
			return rule.Name, true
		}
	}
	return "", false
}
