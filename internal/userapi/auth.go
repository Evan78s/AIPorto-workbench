package userapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"aiporto/internal/util"
	"github.com/jackc/pgx/v5"
)

// register 第一步：校验邮箱未注册，生成验证码发邮件，验证码只存 HMAC 摘要。
func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(email) {
		writeJSON(w, 400, map[string]string{"error": "invalid email"})
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 128 {
		writeJSON(w, 400, map[string]string{"error": "password must be 8-128 characters"})
		return
	}
	if a.mailer == nil {
		writeJSON(w, 503, map[string]string{"error": "email service is not configured, please contact admin"})
		return
	}
	var exists int
	err := a.pg.QueryRow(r.Context(), `SELECT 1 FROM users WHERE email=$1`, email).Scan(&exists)
	if err == nil {
		writeJSON(w, 409, map[string]string{"error": "email already registered"})
		return
	}

	code, err := newVerifyCode()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "generate code failed"})
		return
	}
	ctx := r.Context()
	// 同一邮箱同一用途旧的验证码直接作废。
	if _, err := a.pg.Exec(ctx,
		`UPDATE email_tokens SET used_at=now() WHERE email=$1 AND purpose='register' AND used_at IS NULL`, email); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	_, err = a.pg.Exec(ctx, `
		INSERT INTO email_tokens(id, email, purpose, code_hash, expires_at)
		VALUES($1,$2,'register',$3,$4)`,
		util.NewID(), email, hashCode(a.pepper, email, "register", code), time.Now().Add(codeTTL))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := a.mailer.SendVerifyCode(email, code, codeTTL); err != nil {
		log.Printf("[userapi] register: send code to %s failed: %v", email, err)
		writeJSON(w, 502, map[string]string{"error": "send email failed: " + err.Error()})
		return
	}
	log.Printf("[userapi] register: verification code sent to %s (expires in %s)", email, codeTTL)
	writeJSON(w, 200, map[string]any{
		"email": email,
		"message": "验证码已发送，5 分钟内有效；验证时需再次提交密码",
		"expires_in": int(codeTTL.Seconds()),
	})
}

// verifyEmail 第二步：核对验证码，创建用户 + 租户 + 订阅 + 会话令牌。
func (a *API) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)
	if !validEmail(email) || len(code) != 6 {
		writeJSON(w, 400, map[string]string{"error": "invalid email or code"})
		return
	}

	ctx := r.Context()
	tx, err := a.pg.Begin(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)

	if len(req.Password) < 8 || len(req.Password) > 128 {
		writeJSON(w, 400, map[string]string{"error": "password must be 8-128 characters"})
		return
	}
	// 先按邮箱取最新一条有效记录，再单独比对验证码：
	// 这样输错时能定位到记录并递增 attempts，日志也能区分具体原因（验证码本身永不打印）。
	var tokenID, storedHash string
	var attempts int
	err = tx.QueryRow(ctx, `
		SELECT id::text, code_hash, attempts FROM email_tokens
		WHERE email=$1 AND purpose='register' AND used_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC LIMIT 1`, email).Scan(&tokenID, &storedHash, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		log.Printf("[userapi] verify: no active code for %s (expired, used, or never sent)", email)
		writeJSON(w, 400, map[string]string{"error": "验证码已过期或已被使用，请重新发送验证码"})
		return
	}
	if err != nil {
		log.Printf("[userapi] verify: db error for %s: %v", email, err)
		writeJSON(w, 500, map[string]string{"error": "verify code failed"})
		return
	}
	if attempts >= 5 {
		log.Printf("[userapi] verify: too many attempts for %s", email)
		writeJSON(w, 429, map[string]string{"error": "尝试次数过多，请重新发送验证码"})
		return
	}
	if subtleCompare(storedHash, hashCode(a.pepper, email, "register", code)) != 1 {
		// 输错：递增失败次数，5 次后锁定，防止暴力枚举 6 位验证码。
		if _, err := tx.Exec(ctx, `UPDATE email_tokens SET attempts=attempts+1 WHERE id=$1`, tokenID); err != nil {
			log.Printf("[userapi] verify: bump attempts failed for %s: %v", email, err)
		}
		log.Printf("[userapi] verify: wrong code for %s (attempt %d/5)", email, attempts+1)
		writeJSON(w, 400, map[string]string{"error": "验证码错误，请检查后重试（5 次错误后将锁定）"})
		return
	}
	pwHash, err := hashPassword(req.Password)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "hash password failed"})
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE email_tokens SET used_at=now() WHERE id=$1`, tokenID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// 用户 + 租户：一对一。
	userID := util.NewID()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users(id, email, password_hash, email_verified_at)
		VALUES($1,$2,$3,now())`, userID, email, pwHash); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeJSON(w, 409, map[string]string{"error": "email already registered"})
			return
		}
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	tenantID := util.NewID()
	if _, err := tx.Exec(ctx, `INSERT INTO tenants(id, name, owner_user_id) VALUES($1,$2,$3)`,
		tenantID, "用户-"+email, userID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// 会话令牌明文只回一次。
	token := newSessionToken()
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_sessions(id, user_id, token_hash, expires_at)
		VALUES($1,$2,$3,$4)`, util.NewID(), userID, hashSession(a.pepper, token), time.Now().Add(sessionTTL)); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{
		"session_token": token, "user_id": userID, "tenant_id": tenantID,
		"message": "注册成功，请保存会话令牌（仅显示一次）",
	})
}

// login 邮箱 + 密码登录，签发新会话。
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	var userID, pwHash, status, verified string
	err := a.pg.QueryRow(r.Context(), `
		SELECT id::text, password_hash, status,
		       COALESCE(email_verified_at::text,'') FROM users WHERE email=$1`, email).
		Scan(&userID, &pwHash, &status, &verified)
	if err != nil || status != "active" || !checkPassword(pwHash, req.Password) {
		writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
		return
	}
	token := newSessionToken()
	_, err = a.pg.Exec(r.Context(), `
		INSERT INTO user_sessions(id, user_id, token_hash, expires_at)
		VALUES($1,$2,$3,$4)`, util.NewID(), userID, hashSession(a.pepper, token), time.Now().Add(sessionTTL))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"session_token": token, "user_id": userID})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	_, err := a.pg.Exec(r.Context(), `UPDATE user_sessions SET revoked_at=now() WHERE token_hash=$1`,
		hashSession(a.pepper, token))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "logged_out"})
}

// hasNicknameColumn 检测 users.nickname 是否存在，兼容尚未应用 005 迁移的旧库。
func (a *API) hasNicknameColumn(ctx context.Context) bool {
	var exists bool
	err := a.pg.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.columns
		               WHERE table_name='users' AND column_name='nickname')`).Scan(&exists)
	return err == nil && exists
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	var email, nickname, created, planName string
	var verified bool
	var err error
	if a.hasNicknameColumn(r.Context()) {
		err = a.pg.QueryRow(r.Context(), `
			SELECT u.email, COALESCE(u.nickname,''), to_char(u.created_at,'YYYY-MM-DD HH24:MI'),
			       (u.email_verified_at IS NOT NULL),
			       COALESCE((SELECT p.name FROM tenants t
			                 JOIN subscriptions s ON s.tenant_id=t.id
			                 LEFT JOIN plans p ON p.id=s.plan_id
			                 WHERE t.owner_user_id=u.id LIMIT 1),'未购买')
			FROM users u WHERE u.id=$1`, userIDFrom(r)).Scan(&email, &nickname, &created, &verified, &planName)
	} else {
		// 旧库降级：昵称暂用邮箱前缀，不影响登录与主流程。
		err = a.pg.QueryRow(r.Context(), `
			SELECT u.email, to_char(u.created_at,'YYYY-MM-DD HH24:MI'),
			       (u.email_verified_at IS NOT NULL),
			       COALESCE((SELECT p.name FROM tenants t
			                 JOIN subscriptions s ON s.tenant_id=t.id
			                 LEFT JOIN plans p ON p.id=s.plan_id
			                 WHERE t.owner_user_id=u.id LIMIT 1),'未购买')
			FROM users u WHERE u.id=$1`, userIDFrom(r)).Scan(&email, &created, &verified, &planName)
		nickname = strings.SplitN(email, "@", 2)[0]
	}
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "user not found"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"user_id": userIDFrom(r), "email": email, "nickname": nickname,
		"created_at": created, "email_verified": verified, "plan": planName,
	})
}
