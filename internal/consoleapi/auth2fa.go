package consoleapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aiporto/internal/util"
)

const (
	challengeTTL = 90 * time.Second
	sessionTTL   = 8 * time.Hour
)

// signToken 生成 HMAC 签名的短期令牌；pepper 来自服务端环境变量，不落库。
func (a *API) signToken(kind, user string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(kind + "|" + user + "|" + strconv.FormatInt(exp, 10)))
	mac := hmac.New(sha256.New, a.pepper)
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *API) verifyToken(token, kind string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	mac := hmac.New(sha256.New, a.pepper)
	_, _ = mac.Write([]byte(parts[0]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(parts[1])) != 1 {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	f := strings.Split(string(raw), "|")
	if len(f) != 3 || f[0] != kind {
		return "", false
	}
	exp, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil || exp < time.Now().Unix() {
		return "", false
	}
	return f[1], true
}

// 第一步：校验账号密码，只返回短时 challenge，不签发可访问后台的会话。
func (a *API) adminLogin(w http.ResponseWriter, r *http.Request) {
	if a.adminUser == "" || a.adminPass == "" || a.totpSecret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin auth not configured"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Username), []byte(a.adminUser)) != 1 ||
		subtle.ConstantTimeCompare([]byte(req.Password), []byte(a.adminPass)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mfa_required":    true,
		"challenge_token": a.signToken("mfa", a.adminUser, challengeTTL),
		"expires_in":      int(challengeTTL.Seconds()),
	})
}

// 第二步：校验 TOTP 动态码 + 防重放，成功后才签发 8 小时会话。
func (a *API) adminVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChallengeToken string `json:"challenge_token"`
		Code           string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	user, ok := a.verifyToken(req.ChallengeToken, "mfa")
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "challenge expired or invalid"})
		return
	}
	secret, err := util.DecodeTOTPSecret(a.totpSecret)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "mfa not configured"})
		return
	}
	step, ok := util.VerifyTOTP(secret, req.Code, time.Now(), 1)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid mfa code"})
		return
	}
	tag, err := a.pg.Exec(context.Background(), `
		INSERT INTO admin_totp_used(username, step) VALUES($1,$2) ON CONFLICT DO NOTHING`, user, int64(step))
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "mfa code already used"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": a.signToken("admin", user, sessionTTL),
		"expires_in": int(sessionTTL.Seconds()),
	})
}

// 管理后台所有业务接口只接受双因素会话令牌，不再接受 Basic Auth。
func (a *API) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.adminUser == "" || a.adminPass == "" || a.totpSecret == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin auth not configured"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		user, ok := a.verifyToken(token, "admin")
		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(a.adminUser)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "mfa session required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

