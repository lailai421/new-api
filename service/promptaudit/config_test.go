package promptaudit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.False(t, cfg.Enabled)
	assert.False(t, cfg.LatestTurnOnly)
	assert.True(t, cfg.StorePassEvents)
	assert.True(t, cfg.AllGroups)
	assert.Empty(t, cfg.Groups)
	assert.Equal(t, 0, cfg.RetentionDays)
	assert.Equal(t, StrategyPriority, cfg.Strategy)
	assert.Equal(t, AllScannerIDs, cfg.Scanners)
	assert.Empty(t, cfg.Endpoints)
	assert.Equal(t, int64(1), cfg.ConfigVersion)
}

func TestConfigMatchesGroup(t *testing.T) {
	tests := []struct {
		name     string
		cfg      ActiveConfig
		group    string
		expected bool
	}{
		{
			name: "disabled returns false",
			cfg: ActiveConfig{
				Enabled:   false,
				AllGroups: true,
			},
			group:    "default",
			expected: false,
		},
		{
			name: "all_groups matches any",
			cfg: ActiveConfig{
				Enabled:   true,
				AllGroups: true,
			},
			group:    "vip",
			expected: true,
		},
		{
			name: "specific group matches exact string",
			cfg: ActiveConfig{
				Enabled:   true,
				AllGroups: false,
				GroupsMap: map[string]struct{}{
					"vip":       {},
					"svip-plus": {},
				},
			},
			group:    "vip",
			expected: true,
		},
		{
			name: "specific group does not match unlisted",
			cfg: ActiveConfig{
				Enabled:   true,
				AllGroups: false,
				GroupsMap: map[string]struct{}{
					"vip": {},
				},
			},
			group:    "default",
			expected: false,
		},
		{
			name: "case sensitive exact match",
			cfg: ActiveConfig{
				Enabled:   true,
				AllGroups: false,
				GroupsMap: map[string]struct{}{
					"VIP": {},
				},
			},
			group:    "vip",
			expected: false,
		},
		{
			name: "empty groups map returns false",
			cfg: ActiveConfig{
				Enabled:   true,
				AllGroups: false,
				GroupsMap: map[string]struct{}{},
			},
			group:    "vip",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cfg.MatchesGroup(tt.group))
		})
	}
}

func TestNormalizeAndValidate_Groups(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllGroups = false
	cfg.Groups = []string{" vip ", "default", "vip", "  ", "svip"}

	err := cfg.NormalizeAndValidate(true)
	require.NoError(t, err)

	// 应去重、剔除空白并稳定排序
	assert.Equal(t, []string{"default", "svip", "vip"}, cfg.Groups)

	// 指定分组但全为空字符应报错
	cfg.Groups = []string{" ", "   "}
	err = cfg.NormalizeAndValidate(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one non-empty group")
}

func TestNormalizeAndValidate_Scanners(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Scanners = []string{"jailbreak", "VIOLENT", "Prompt-Injection", "personal identifying information"}

	err := cfg.NormalizeAndValidate(true)
	require.NoError(t, err)

	// 应归一化、去重并按 AllScannerIDs 顺序排列
	assert.Equal(t, []string{"violent", "pii", "jailbreak"}, cfg.Scanners)

	// 未知分类应报错
	cfg.Scanners = []string{"non_existent_category"}
	err = cfg.NormalizeAndValidate(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown scanner category")

	// 空分类列表应报错
	cfg.Scanners = []string{}
	err = cfg.NormalizeAndValidate(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one scanner category")
}

func TestNormalizeAndValidate_Endpoints(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Endpoints = []Endpoint{
		{
			ID:         "ep-1",
			Name:       "Primary Guard",
			BaseURL:    "http://127.0.0.1:8000/v1/",
			TimeoutMS:  5000,
			InputLimit: 2000,
			Enabled:    true,
		},
		{
			ID:      "ep-2",
			Name:    "Secondary Guard",
			BaseURL: "https://guard.example.com",
			Enabled: false,
		},
	}

	err := cfg.NormalizeAndValidate(true)
	require.NoError(t, err)

	assert.Equal(t, "http://127.0.0.1:8000", cfg.Endpoints[0].BaseURL)
	assert.Equal(t, DefaultGuardModel, cfg.Endpoints[0].Model)
	assert.Equal(t, ProtocolOpenAICompatible, cfg.Endpoints[0].Protocol)
	assert.Equal(t, 5000, cfg.Endpoints[0].TimeoutMS)
	assert.Equal(t, 2000, cfg.Endpoints[0].InputLimit)

	assert.Equal(t, "https://guard.example.com", cfg.Endpoints[1].BaseURL)
	assert.Equal(t, DefaultTimeoutMS, cfg.Endpoints[1].TimeoutMS)
	assert.Equal(t, DefaultInputLimit, cfg.Endpoints[1].InputLimit)

	// 重复节点 ID 应报错
	cfg.Endpoints = append(cfg.Endpoints, Endpoint{
		ID:      "ep-1",
		Name:    "Duplicate",
		BaseURL: "http://127.0.0.1:8001",
	})
	err = cfg.NormalizeAndValidate(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate endpoint id")
}

func TestNormalizeAndValidate_EnabledConstraints(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Endpoints = []Endpoint{
		{
			ID:      "node-1",
			Name:    "Node 1",
			BaseURL: "http://127.0.0.1:8000",
			Enabled: true,
		},
	}

	// 开启审计但无稳定密钥，应返回 ErrorCodeEncryptionKeyRequired 错误
	err := cfg.NormalizeAndValidate(false)
	require.Error(t, err)
	var gErr *GuardError
	require.ErrorAs(t, err, &gErr)
	assert.Equal(t, ErrorCodeEncryptionKeyRequired, gErr.Code)
	assert.Equal(t, 400, gErr.HTTPStatus)

	// 开启审计且有稳定密钥，应通过
	err = cfg.NormalizeAndValidate(true)
	require.NoError(t, err)

	// 开启审计但所有节点均被禁用，应报错
	cfg.Endpoints[0].Enabled = false
	err = cfg.NormalizeAndValidate(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one enabled endpoint")
}

func TestConfigToPublicAndActive(t *testing.T) {
	encryptor, err := NewAESGCMEncryptor("test-secret-key-123456789012345678")
	require.NoError(t, err)

	tokenCipher, err := encryptor.EncryptToken("secret-token-123")
	require.NoError(t, err)

	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.AllGroups = false
	cfg.Groups = []string{"vip"}
	cfg.Endpoints = []Endpoint{
		{
			ID:              "ep-configured",
			Name:            "Configured Node",
			BaseURL:         "http://127.0.0.1:8000",
			TokenCiphertext: tokenCipher,
			Enabled:         true,
		},
		{
			ID:              "ep-invalid",
			Name:            "Invalid Node",
			BaseURL:         "http://127.0.0.1:8001",
			TokenCiphertext: "v1:corrupted-ciphertext",
			Enabled:         true,
		},
		{
			ID:      "ep-missing",
			Name:    "Missing Token Node",
			BaseURL: "http://127.0.0.1:8002",
			Enabled: false,
		},
	}

	// 1. ToPublic
	pub := cfg.ToPublic(encryptor)
	assert.True(t, pub.Enabled)
	assert.Equal(t, ModeBlocking, pub.EffectiveMode)
	assert.Equal(t, HasStableCryptoSecret(), pub.HasStableCryptoSecret)
	require.Len(t, pub.Endpoints, 3)

	assert.True(t, pub.Endpoints[0].HasToken)
	assert.Equal(t, "configured", pub.Endpoints[0].TokenStatus)

	assert.True(t, pub.Endpoints[1].HasToken)
	assert.Equal(t, "invalid", pub.Endpoints[1].TokenStatus)

	assert.False(t, pub.Endpoints[2].HasToken)
	assert.Equal(t, "missing", pub.Endpoints[2].TokenStatus)

	// 2. ToActive
	act := cfg.ToActive(encryptor)
	assert.True(t, act.Enabled)
	assert.True(t, act.MatchesGroup("vip"))
	assert.False(t, act.MatchesGroup("default"))

	require.Len(t, act.Endpoints, 3)
	assert.Equal(t, "secret-token-123", act.Endpoints[0].Token)
	assert.False(t, act.Endpoints[0].TokenInvalid)

	assert.True(t, act.Endpoints[1].TokenInvalid)
	assert.Empty(t, act.Endpoints[1].Token)

	assert.False(t, act.Endpoints[2].TokenInvalid)
	assert.Empty(t, act.Endpoints[2].Token)

	// EnabledEndpoints 仅返回启用的且凭据有效的节点
	enabledNodes := act.EnabledEndpoints()
	require.Len(t, enabledNodes, 1)
	assert.Equal(t, "ep-configured", enabledNodes[0].ID)
}

func TestNormalizeAndValidate_Protocol(t *testing.T) {
	// 1. 空协议归一为 openai_compatible，默认模型 DefaultGuardModel，默认超时 3000ms
	cfg1 := DefaultConfig()
	cfg1.Endpoints = []Endpoint{
		{
			ID:      "ep-empty-proto",
			Name:    "Empty Proto",
			BaseURL: "http://127.0.0.1:8000",
		},
	}
	err := cfg1.NormalizeAndValidate(true)
	require.NoError(t, err)
	assert.Equal(t, ProtocolOpenAICompatible, cfg1.Endpoints[0].Protocol)
	assert.Equal(t, DefaultGuardModel, cfg1.Endpoints[0].Model)
	assert.Equal(t, DefaultTimeoutMS, cfg1.Endpoints[0].TimeoutMS)

	// 2. 非法协议拒绝
	cfg2 := DefaultConfig()
	cfg2.Endpoints = []Endpoint{
		{
			ID:       "ep-bad-proto",
			Name:     "Bad Proto",
			BaseURL:  "http://127.0.0.1:8000",
			Protocol: "unknown_proto",
		},
	}
	err = cfg2.NormalizeAndValidate(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported protocol: unknown_proto")

	// 3. llm_classifier 协议：提供 model 时默认超时 8000ms
	cfg3 := DefaultConfig()
	cfg3.Endpoints = []Endpoint{
		{
			ID:       "ep-llm",
			Name:     "LLM Classifier",
			BaseURL:  "https://api.deepseek.com",
			Protocol: ProtocolLLMClassifier,
			Model:    "deepseek-chat",
		},
	}
	err = cfg3.NormalizeAndValidate(true)
	require.NoError(t, err)
	assert.Equal(t, ProtocolLLMClassifier, cfg3.Endpoints[0].Protocol)
	assert.Equal(t, "deepseek-chat", cfg3.Endpoints[0].Model)
	assert.Equal(t, DefaultLLMTimeoutMS, cfg3.Endpoints[0].TimeoutMS)

	// 4. llm_classifier 协议：空 model 必须报错，禁止回填 DefaultGuardModel
	cfg4 := DefaultConfig()
	cfg4.Endpoints = []Endpoint{
		{
			ID:       "ep-llm-no-model",
			Name:     "LLM No Model",
			BaseURL:  "https://api.deepseek.com",
			Protocol: ProtocolLLMClassifier,
			Model:    "   ",
		},
	}
	err = cfg4.NormalizeAndValidate(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing model for protocol llm_classifier")

	// 5. 节点池中混合支持两种协议
	cfg5 := DefaultConfig()
	cfg5.Endpoints = []Endpoint{
		{
			ID:       "ep-qwen",
			Name:     "Qwen Guard",
			BaseURL:  "http://127.0.0.1:8000",
			Protocol: ProtocolOpenAICompatible,
		},
		{
			ID:       "ep-deepseek",
			Name:     "DeepSeek LLM",
			BaseURL:  "https://api.deepseek.com",
			Protocol: ProtocolLLMClassifier,
			Model:    "deepseek-chat",
		},
	}
	err = cfg5.NormalizeAndValidate(true)
	require.NoError(t, err)
	assert.Equal(t, ProtocolOpenAICompatible, cfg5.Endpoints[0].Protocol)
	assert.Equal(t, DefaultGuardModel, cfg5.Endpoints[0].Model)
	assert.Equal(t, ProtocolLLMClassifier, cfg5.Endpoints[1].Protocol)
	assert.Equal(t, "deepseek-chat", cfg5.Endpoints[1].Model)
}
