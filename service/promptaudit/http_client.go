package promptaudit

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NormalizeBaseURL 校验并规范化 Guard 节点地址。
// 规则：仅允许 http/https 协议；禁止凭据、查询参数和片段；支持自托管私网与 loopback 节点。
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid base url: %s", raw)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid base url scheme (only http/https allowed): %s", parsed.Scheme)
	}

	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("base url must not contain userinfo, query parameters, or fragment")
	}

	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("invalid base url hostname: %s", raw)
	}

	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if strings.HasSuffix(strings.ToLower(path), "/v1") {
		path = path[:len(path)-3]
	}
	parsed.Path = path
	parsed.RawPath = ""

	return strings.TrimRight(parsed.String(), "/"), nil
}

// ChatCompletionsURL 构造节点 chat completions 地址。
func ChatCompletionsURL(base string) (string, error) {
	normalized, err := NormalizeBaseURL(base)
	if err != nil {
		return "", err
	}
	return normalized + "/v1/chat/completions", nil
}

// NewSecureHTTPClient 为 Guard 节点创建安全独立的 HTTP Client。
// 约束：不继承系统代理，不跟随跨主机重定向，TLS 最低 1.2，支持私网/loopback。
func NewSecureHTTPClient(endpoint ActiveEndpoint) (*http.Client, error) {
	if _, err := NormalizeBaseURL(endpoint.BaseURL); err != nil {
		return nil, err
	}

	timeoutMS := endpoint.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = DefaultTimeoutMS
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond

	dialer := &net.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 nil, // 显式禁用代理，避免绕过本地网络策略
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext:           dialer.DialContext,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
				return fmt.Errorf("cross-host redirect not allowed: %s -> %s", via[0].URL.Host, req.URL.Host)
			}
			return nil
		},
	}, nil
}
