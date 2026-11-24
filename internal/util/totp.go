package util

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// DecodeTOTPSecret 解析 RFC4648 Base32 私钥（无 padding，大小写与空格不敏感）。
// 生产环境由 ADMIN_TOTP_SECRET 提供，代码里不内置任何默认值。
func DecodeTOTPSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(secret, " ", "")))
	s = strings.TrimRight(s, "=")
	if s == "" {
		return nil, fmt.Errorf("totp secret is empty")
	}
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("totp secret is not valid base32: %w", err)
	}
	if len(b) < 16 {
		return nil, fmt.Errorf("totp secret must decode to at least 16 bytes, got %d", len(b))
	}
	return b, nil
}

func totpCode(secret []byte, step uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], step)
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1000000)
}

// VerifyTOTP 校验 6 位动态码，返回匹配到的时间步（用于防重放）。
// skew=1 表示接受前后各一个 30 秒窗口。
func VerifyTOTP(secret []byte, code string, at time.Time, skew int) (uint64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return 0, false
	}
	current := int64(at.Unix() / 30)
	if skew < 0 {
		skew = 0
	}
	for d := -int64(skew); d <= int64(skew); d++ {
		step := current + d
		if step < 0 {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(code), []byte(totpCode(secret, uint64(step)))) == 1 {
			return uint64(step), true
		}
	}
	return 0, false
}
