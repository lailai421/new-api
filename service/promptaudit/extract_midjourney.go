package promptaudit

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

// ExtractMidjourneyRequest 提取 Midjourney 请求中的提示词文本。
// 查询、通知、获取类动作及 SwapFace 返回 ErrNoPrompt；
// 提交类动作使用 common.UnmarshalBodyReusable 安全读取 dto.MidjourneyRequest，提取 Prompt 和 Content，排除 base64Array、maskBase64 等。
func ExtractMidjourneyRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo) (segments []PromptSegment, modelName string, err error) {
	if relayInfo == nil || c == nil {
		return nil, "", ErrUnsupportedProtocol
	}

	modelName = relayInfo.OriginModelName
	if modelName == "" {
		modelName = "midjourney"
	}

	// 1. 识别查询、通知、获取类动作及 SwapFace，确定无待审文本
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify,
		relayconstant.RelayModeMidjourneyTaskFetch,
		relayconstant.RelayModeMidjourneyTaskFetchByCondition,
		relayconstant.RelayModeMidjourneyTaskImageSeed,
		relayconstant.RelayModeSwapFace:
		return nil, modelName, ErrNoPrompt
	}

	// 2. 识别提交类动作
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyImagine,
		relayconstant.RelayModeMidjourneyAction,
		relayconstant.RelayModeMidjourneyModal,
		relayconstant.RelayModeMidjourneyShorten,
		relayconstant.RelayModeMidjourneyDescribe,
		relayconstant.RelayModeMidjourneyEdits,
		relayconstant.RelayModeMidjourneyBlend,
		relayconstant.RelayModeMidjourneyUpload,
		relayconstant.RelayModeMidjourneyChange,
		relayconstant.RelayModeMidjourneySimpleChange,
		relayconstant.RelayModeMidjourneyVideo:

		var mjReq taskdto.MidjourneyRequest
		if err := common.UnmarshalBodyReusable(c, &mjReq); err != nil {
			return nil, modelName, ErrUnsupportedProtocol
		}

		if prompt := strings.TrimSpace(mjReq.Prompt); prompt != "" && !looksLikeMediaPayload(prompt) {
			segments = append(segments, PromptSegment{
				Role:    RoleUser,
				Content: prompt,
				User:    true,
			})
		}

		if content := strings.TrimSpace(mjReq.Content); content != "" && !looksLikeMediaPayload(content) {
			segments = append(segments, PromptSegment{
				Role:    RoleUser,
				Content: content,
				User:    true,
			})
		}

		normalized := NormalizeSegments(segments)
		if len(normalized) == 0 {
			return nil, modelName, ErrNoPrompt
		}
		return normalized, modelName, nil

	default:
		return nil, modelName, ErrUnsupportedProtocol
	}
}
