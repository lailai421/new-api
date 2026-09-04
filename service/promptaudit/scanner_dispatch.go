package promptaudit

import (
	"context"
	"fmt"
)

// DispatchScanner 根据节点的 Protocol 字段将扫描请求分发给具体的 Scanner 实现。
type DispatchScanner struct {
	qwenScanner *OpenAICompatibleScanner
	llmScanner  *LLMClassifierScanner
}

func NewDispatchScanner() *DispatchScanner {
	return &DispatchScanner{
		qwenScanner: NewOpenAICompatibleScanner(),
		llmScanner:  NewLLMClassifierScanner(),
	}
}

func (d *DispatchScanner) Scan(ctx context.Context, endpoint ActiveEndpoint, chunk string, enabledScanners []string) (*NormalizedResult, error) {
	switch endpoint.Protocol {
	case ProtocolLLMClassifier:
		return d.llmScanner.Scan(ctx, endpoint, chunk, enabledScanners)
	case ProtocolOpenAICompatible, "":
		return d.qwenScanner.Scan(ctx, endpoint, chunk, enabledScanners)
	default:
		return nil, &GuardError{
			Code:       ErrorCodeUnsupportedProtocol,
			HTTPStatus: 400,
			Retryable:  false,
			Cause:      fmt.Errorf("unsupported guard protocol: %s", endpoint.Protocol),
		}
	}
}
