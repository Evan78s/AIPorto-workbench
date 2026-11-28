package userapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// hashPassword 用 bcrypt 存储密码，绝不存明文。
func hashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

func checkPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// userTokenPrefix 是用户会话令牌前缀，便于与控制台 cw-、客户 key sk-wb- 区分。
const userTokenPrefix = "us-"

// newSessionToken 生成随机令牌明文，数据库只存哈希。
func newSessionToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return userTokenPrefix + hex.EncodeToString(b)
}

// hashSession 用 pepper 计算会话令牌摘要。
func hashSession(pepper []byte, token string) string {
	m := hmac.New(sha256.New, pepper)
	_, _ = m.Write([]byte("user-session:" + token))
	return hex.EncodeToString(m.Sum(nil))
}

// newVerifyCode 生成 6 位数字验证码。
func newVerifyCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n < 0 {
		n = -n
	}
	return fmt.Sprintf("%06d", n%1000000), nil
}

func hashCode(pepper []byte, email, purpose, code string) string {
	m := hmac.New(sha256.New, pepper)
	_, _ = m.Write([]byte(strings.ToLower(email) + ":" + purpose + ":" + code))
	return hex.EncodeToString(m.Sum(nil))
}

// subtleCompare 常量时间比较两个哈希，避免通过响应时间推断正确验证码。
func subtleCompare(a, b string) int {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b))
}

const sessionTTL = 7 * 24 * time.Hour
const codeTTL = 5 * time.Minute

func validEmail(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	if len(email) < 5 || len(email) > 254 {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at >= len(email)-1 {
		return false
	}
	if strings.ContainsAny(email[:at], " \t\r\n") {
		return false
	}
	return strings.Contains(email[at+1:], ".") && !strings.ContainsAny(email[at+1:], " \t\r\n")
}
