package promptaudit

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractMidjourneyRequest_QueryAndNotifyReturnNoPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	modes := []int{
		relayconstant.RelayModeMidjourneyNotify,
		relayconstant.RelayModeMidjourneyTaskFetch,
		relayconstant.RelayModeMidjourneyTaskFetchByCondition,
		relayconstant.RelayModeMidjourneyTaskImageSeed,
		relayconstant.RelayModeSwapFace,
	}

	for _, mode := range modes {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/mj/task/123/fetch", nil)

		info := &relaycommon.RelayInfo{
			RelayMode:       mode,
			OriginModelName: "midjourney",
		}

		segments, modelName, err := ExtractMidjourneyRequest(c, info)
		require.ErrorIs(t, err, ErrNoPrompt)
		assert.Nil(t, segments)
		assert.Equal(t, "midjourney", modelName)
	}
}

func TestExtractMidjourneyRequest_Submissions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	submissionModes := []int{
		relayconstant.RelayModeMidjourneyImagine,
		relayconstant.RelayModeMidjourneyAction,
		relayconstant.RelayModeMidjourneyModal,
		relayconstant.RelayModeMidjourneyShorten,
		relayconstant.RelayModeMidjourneyDescribe,
		relayconstant.RelayModeMidjourneyEdits,
		relayconstant.RelayModeMidjourneyBlend,
		relayconstant.RelayModeMidjourneyUpload,
		relayconstant.RelayModeMidjourneyChange,
		relayconstant.RelayModeMidjourneySimpleChange,
		relayconstant.RelayModeMidjourneyVideo,
	}

	for _, mode := range submissionModes {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		base64Canary := "data:image/png;base64," + strings.Repeat("M", 300)
		bodyBytes, err := common.Marshal(map[string]any{
			"prompt":      "画一个赛博朋克城市的街道 --v 6.0",
			"content":     "附带的指令说明文本",
			"base64Array": []string{base64Canary},
			"maskBase64":  base64Canary,
		})
		require.NoError(t, err)

		c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/action", bytes.NewReader(bodyBytes))
		c.Request.Header.Set("Content-Type", "application/json")

		info := &relaycommon.RelayInfo{
			RelayMode:       mode,
			OriginModelName: "midjourney",
		}

		segments, modelName, err := ExtractMidjourneyRequest(c, info)
		require.NoError(t, err)
		assert.Equal(t, "midjourney", modelName)
		assert.Len(t, segments, 2)

		full := JoinSegments(segments)
		assert.Contains(t, full, "画一个赛博朋克城市的街道")
		assert.Contains(t, full, "附带的指令说明文本")
		assert.NotContains(t, full, "data:image/png;base64")
		assert.NotContains(t, full, strings.Repeat("M", 100))

		// 验证请求体可复用性：审计提取后业务层依然能完整读取
		var rereadReq taskdto.MidjourneyRequest
		err = common.UnmarshalBodyReusable(c, &rereadReq)
		require.NoError(t, err)
		assert.Equal(t, "画一个赛博朋克城市的街道 --v 6.0", rereadReq.Prompt)
		assert.Equal(t, "附带的指令说明文本", rereadReq.Content)
		assert.Len(t, rereadReq.Base64Array, 1)

		// 再次验证原生 Body 未被耗尽
		readBytes, readErr := io.ReadAll(c.Request.Body)
		require.NoError(t, readErr)
		assert.NotEmpty(t, readBytes)
	}
}

func TestExtractMidjourneyRequest_EmptySubmissionAndUnknown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. 提交动作但是无文本
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	bodyBytes, _ := common.Marshal(map[string]any{
		"prompt":  "   ",
		"content": "",
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeMidjourneyImagine,
		OriginModelName: "midjourney",
	}
	_, _, err := ExtractMidjourneyRequest(c, info)
	require.ErrorIs(t, err, ErrNoPrompt)

	// 2. 未知模式返回 ErrUnsupportedProtocol
	unknownInfo := &relaycommon.RelayInfo{
		RelayMode:       99999,
		OriginModelName: "midjourney",
	}
	_, _, err = ExtractMidjourneyRequest(c, unknownInfo)
	require.ErrorIs(t, err, ErrUnsupportedProtocol)

	// nil 参数
	_, _, err = ExtractMidjourneyRequest(nil, nil)
	require.ErrorIs(t, err, ErrUnsupportedProtocol)
}
