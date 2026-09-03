package promptaudit

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAESGCMEncryptor(t *testing.T) {
	_, err := NewAESGCMEncryptor("")
	require.Error(t, err)

	_, err = NewAESGCMEncryptor("   ")
	require.Error(t, err)

	enc, err := NewAESGCMEncryptor("test-master-secret-key-123456789")
	require.NoError(t, err)
	require.NotNil(t, enc)
}

func TestAESGCMEncryptor_EnvelopeAndRandomNonce(t *testing.T) {
	enc, err := NewAESGCMEncryptor("test-secret-123")
	require.NoError(t, err)

	plain := "hello prompt audit"
	c1, err := enc.EncryptToken(plain)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(c1, EnvelopeVersionV1))

	c2, err := enc.EncryptToken(plain)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(c2, EnvelopeVersionV1))

	// 每次加密应使用独立的随机 nonce，密文必须不同
	assert.NotEqual(t, c1, c2)

	// 解密还原
	d1, err := enc.DecryptToken(c1)
	require.NoError(t, err)
	assert.Equal(t, plain, d1)

	d2, err := enc.DecryptToken(c2)
	require.NoError(t, err)
	assert.Equal(t, plain, d2)
}

func TestAESGCMEncryptor_PurposeIsolation(t *testing.T) {
	enc, err := NewAESGCMEncryptor("master-secret-for-purpose-isolation")
	require.NoError(t, err)

	text := "secret message that needs purpose isolation"

	// 1. 使用 Token Key 加密
	tokenCipher, err := enc.EncryptToken(text)
	require.NoError(t, err)

	// Token 解密成功
	decryptedToken, err := enc.DecryptToken(tokenCipher)
	require.NoError(t, err)
	assert.Equal(t, text, decryptedToken)

	// 使用 Prompt Key 解密必须失败！
	_, err = enc.DecryptPrompt(tokenCipher)
	require.Error(t, err, "decrypting token ciphertext with prompt key must fail")

	// 2. 使用 Prompt Key 加密
	promptCipher, err := enc.EncryptPrompt(text)
	require.NoError(t, err)

	// Prompt 解密成功
	decryptedPrompt, err := enc.DecryptPrompt(promptCipher)
	require.NoError(t, err)
	assert.Equal(t, text, decryptedPrompt)

	// 使用 Token Key 解密必须失败！
	_, err = enc.DecryptToken(promptCipher)
	require.Error(t, err, "decrypting prompt ciphertext with token key must fail")
}

func TestAESGCMEncryptor_UnicodeAndLargePayloads(t *testing.T) {
	enc, err := NewAESGCMEncryptor("master-secret-unicode-and-large")
	require.NoError(t, err)

	tests := []struct {
		name  string
		plain string
	}{
		{
			name:  "chinese and emoji",
			plain: "你好，世界！🤖🔥 这是一个包含中文、emoji 和特殊标点的提示词。",
		},
		{
			name:  "newlines and control chars including NUL",
			plain: "line 1\nline 2\r\nline 3\t\x00\x01\x02end",
		},
		{
			name:  "large text exceeding 64KiB",
			plain: strings.Repeat("长提示词测试段落——为了保护业务模型与系统安全。\n", 2000), // ~80 KiB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := enc.EncryptPrompt(tt.plain)
			require.NoError(t, err)

			decrypted, err := enc.DecryptPrompt(ciphertext)
			require.NoError(t, err)
			assert.Equal(t, tt.plain, decrypted)
		})
	}
}

func TestAESGCMEncryptor_CorruptedCiphertext(t *testing.T) {
	enc, err := NewAESGCMEncryptor("master-secret-for-corruption-test")
	require.NoError(t, err)

	// 1. 无版本前缀
	_, err = enc.DecryptToken("invalid-prefix-ciphertext")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "envelope version")

	// 2. 损坏的 base64
	_, err = enc.DecryptToken("v1:???not-valid-base64???")
	require.Error(t, err)

	// 3. 密文过短（不足 nonce 长度）
	_, err = enc.DecryptToken("v1:YQ==") // base64("a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too short")

	// 4. 篡改密文字节
	valid, err := enc.EncryptToken("hello world")
	require.NoError(t, err)

	// 修改最后一个字符
	tampered := valid[:len(valid)-2] + "AA"
	_, err = enc.DecryptToken(tampered)
	require.Error(t, err)
}

func TestHasStableCryptoSecret(t *testing.T) {
	origCrypto := os.Getenv("CRYPTO_SECRET")
	origSession := os.Getenv("SESSION_SECRET")
	defer func() {
		_ = os.Setenv("CRYPTO_SECRET", origCrypto)
		_ = os.Setenv("SESSION_SECRET", origSession)
	}()

	_ = os.Unsetenv("CRYPTO_SECRET")
	_ = os.Unsetenv("SESSION_SECRET")
	assert.False(t, HasStableCryptoSecret())

	_ = os.Setenv("CRYPTO_SECRET", "custom-crypto-secret")
	assert.True(t, HasStableCryptoSecret())

	_ = os.Unsetenv("CRYPTO_SECRET")
	_ = os.Setenv("SESSION_SECRET", "custom-session-secret")
	assert.True(t, HasStableCryptoSecret())
}
