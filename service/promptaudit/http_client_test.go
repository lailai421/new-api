package promptaudit

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBaseURL(t *testing.T) {
	allowed := []struct {
		raw      string
		expected string
	}{
		{"http://127.0.0.1:8000", "http://127.0.0.1:8000"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"http://10.0.0.1:9000/", "http://10.0.0.1:9000"},
		{"http://192.168.1.100:8000", "http://192.168.1.100:8000"},
		{"https://guard.example.com", "https://guard.example.com"},
		{"https://guard.example.com/v1", "https://guard.example.com"},
		{"https://guard.example.com/v1/", "https://guard.example.com"},
		{"https://guard.example.com/custom/path/v1", "https://guard.example.com/custom/path"},
	}

	for _, tt := range allowed {
		t.Run("allowed_"+tt.raw, func(t *testing.T) {
			norm, err := NormalizeBaseURL(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, norm)
		})
	}

	blocked := []string{
		"",
		"   ",
		"ftp://guard.example.com",
		"ws://guard.example.com",
		"https://user:pass@guard.example.com",
		"https://guard.example.com?query=secret",
		"https://guard.example.com/#fragment",
		"http://",
		"://invalid-url",
	}

	for _, raw := range blocked {
		t.Run("blocked_"+raw, func(t *testing.T) {
			_, err := NormalizeBaseURL(raw)
			require.Error(t, err)
		})
	}
}

func TestChatCompletionsURL(t *testing.T) {
	url, err := ChatCompletionsURL("https://guard.example.com/v1/")
	require.NoError(t, err)
	assert.Equal(t, "https://guard.example.com/v1/chat/completions", url)
}

func TestNewSecureHTTPClient_TransportSecurity(t *testing.T) {
	client, err := NewSecureHTTPClient(ActiveEndpoint{
		BaseURL:   "http://127.0.0.1:8000",
		TimeoutMS: 2500,
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	// 显式禁用系统代理
	assert.Nil(t, transport.Proxy)
	// TLS 最低版本为 1.2
	assert.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
}

func TestNewSecureHTTPClient_CrossHostRedirectForbidden(t *testing.T) {
	// 目标服务器（另一主机）
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("target reached"))
	}))
	defer targetServer.Close()

	// 重定向服务器（跨主机重定向到 targetServer）
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	client, err := NewSecureHTTPClient(ActiveEndpoint{
		BaseURL:   redirectServer.URL,
		TimeoutMS: 2000,
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, redirectServer.URL, nil)
	require.NoError(t, err)

	_, err = client.Do(req)
	// 跨主机重定向必须失败
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cross-host redirect not allowed")
}

func TestNewSecureHTTPClient_SameHostRedirectAllowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/destination", http.StatusFound)
			return
		}
		if r.URL.Path == "/destination" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := NewSecureHTTPClient(ActiveEndpoint{
		BaseURL:   server.URL,
		TimeoutMS: 2000,
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/redirect", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
