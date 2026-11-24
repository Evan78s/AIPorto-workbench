package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unicode/utf8"
)

// NewID 生成一个 UUID v4 形状的字符串，用作 request_id 及各实体 id。
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// HashKey 用服务端 pepper 计算客户 key 的 HMAC-SHA256 十六进制摘要。
func HashKey(pepper []byte, token string) string {
	m := hmac.New(sha256.New, pepper)
	_, _ = m.Write([]byte(token))
	return hex.EncodeToString(m.Sum(nil))
}

// EncryptKey 使用 AES-256-GCM 加密上游 key，布局为 nonce(12) || ciphertext || tag。
func EncryptKey(masterKey, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// DecryptKey 是 EncryptKey 的逆操作。
func DecryptKey(masterKey, blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize()+gcm.Overhead() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// EstimateTokens 是保守的本地 token 估算：按字符数/2 计，对中文偏保守、对英文偏高。
// 只用于预扣上限和上游未返回 usage 时的兜底，真实计费应以服务端 tokenizer 为准。
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return utf8.RuneCountInString(s)/2 + 1
}

// KeyHint 生成可安全展示的掩码，例如 sk-wb-1a2b…9f8e。
func KeyHint(token string) string {
	if len(token) <= 12 {
		return "***"
	}
	return token[:8] + "…" + token[len(token)-4:]
}
