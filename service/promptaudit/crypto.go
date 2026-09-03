package promptaudit

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// 用途隔离上下文标识
	PurposeGuardToken = "prompt_audit:guard_token"
	PurposeFullPrompt = "prompt_audit:full_prompt"

	// 密文 Envelope 版本前缀
	EnvelopeVersionV1 = "v1:"
)

// HasStableCryptoSecret 检查部署环境是否显式配置了持久化的密钥环境变量。
// 若仅为进程启动时随机生成的默认值，返回 false，防止审计数据因重启无法解密。
func HasStableCryptoSecret() bool {
	return os.Getenv("CRYPTO_SECRET") != "" || os.Getenv("SESSION_SECRET") != ""
}

// AESGCMEncryptor 实现基于 AES-256-GCM 与用途隔离的 Encryptor 接口。
type AESGCMEncryptor struct {
	tokenKey  []byte
	promptKey []byte
}

// NewAESGCMEncryptor 通过主密钥派生不同用途的子密钥。
func NewAESGCMEncryptor(masterSecret string) (*AESGCMEncryptor, error) {
	trimmed := strings.TrimSpace(masterSecret)
	if trimmed == "" {
		return nil, errors.New("master crypto secret cannot be empty")
	}

	tokenKey := derivePurposeKey(trimmed, PurposeGuardToken)
	promptKey := derivePurposeKey(trimmed, PurposeFullPrompt)

	return &AESGCMEncryptor{
		tokenKey:  tokenKey,
		promptKey: promptKey,
	}, nil
}

// derivePurposeKey 使用 HMAC-SHA256 进行单向用途隔离派生，生成 32 字节 AES-256 密钥。
func derivePurposeKey(masterSecret, purpose string) []byte {
	h := hmac.New(sha256.New, []byte(masterSecret))
	h.Write([]byte(purpose))
	return h.Sum(nil)
}

func (e *AESGCMEncryptor) EncryptToken(plaintext string) (string, error) {
	return encryptAESGCM(e.tokenKey, []byte(plaintext))
}

func (e *AESGCMEncryptor) DecryptToken(ciphertext string) (string, error) {
	bytes, err := decryptAESGCM(e.tokenKey, ciphertext)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (e *AESGCMEncryptor) EncryptPrompt(plaintext string) (string, error) {
	return encryptAESGCM(e.promptKey, []byte(plaintext))
}

func (e *AESGCMEncryptor) DecryptPrompt(ciphertext string) (string, error) {
	bytes, err := decryptAESGCM(e.promptKey, ciphertext)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// encryptAESGCM 执行 AES-256-GCM 加密，并返回 "v1:<base64(nonce + ciphertext + tag)>" 格式 envelope。
func encryptAESGCM(key []byte, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate random nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return EnvelopeVersionV1 + base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptAESGCM 校验版本并解密 envelope。
func decryptAESGCM(key []byte, envelope string) ([]byte, error) {
	if !strings.HasPrefix(envelope, EnvelopeVersionV1) {
		return nil, errors.New("unsupported or missing envelope version")
	}

	rawB64 := strings.TrimPrefix(envelope, EnvelopeVersionV1)
	data, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return nil, fmt.Errorf("decode base64 ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("gcm decrypt failed: %w", err)
	}

	return plaintext, nil
}
