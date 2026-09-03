package relay

import (
	"fmt"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/promptaudit"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func WssHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	defer func() {
		if info != nil && info.TargetWs != nil {
			_ = info.TargetWs.Close()
		}
	}()

	statusCodeMappingStr := c.GetString("status_code_mapping")

	// 审计开启且命中分组时，上游连接延迟到首个文本帧通过审计前才拨号
	if promptaudit.ShouldAuditRealtime(c, info) {
		info.TargetWsDialer = func() (*websocket.Conn, error) {
			resp, err := adaptor.DoRequest(c, info, nil)
			if err != nil {
				return nil, err
			}
			if resp == nil {
				return nil, fmt.Errorf("upstream websocket response is nil")
			}
			conn, ok := resp.(*websocket.Conn)
			if !ok {
				return nil, fmt.Errorf("upstream response is not a websocket connection")
			}
			info.TargetWs = conn
			return conn, nil
		}
	} else {
		resp, err := adaptor.DoRequest(c, info, nil)
		if err != nil {
			return types.NewError(err, types.ErrorCodeDoRequestFailed)
		}
		if resp != nil {
			info.TargetWs = resp.(*websocket.Conn)
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, nil, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	if usage != nil {
		if realtimeUsage, ok := usage.(*dto.RealtimeUsage); ok && realtimeUsage != nil {
			service.PostWssConsumeQuota(c, info, info.UpstreamModelName, realtimeUsage, "")
		}
	}
	return nil
}
