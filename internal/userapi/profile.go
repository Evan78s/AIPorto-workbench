package userapi

import (
	"net/http"
	"strings"
)

// updateProfile 修改昵称。昵称只允许非空字符串、最长 32 字符，邮箱不可改（邮箱是登录标识）。
func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	nickname := strings.TrimSpace(req.Nickname)
	if len([]rune(nickname)) > 32 {
		writeJSON(w, 400, map[string]string{"error": "昵称最多 32 个字符"})
		return
	}
	if !a.hasNicknameColumn(r.Context()) {
		writeJSON(w, 503, map[string]string{"error": "昵称功能需先执行数据库迁移（migrations/005_user_profile.sql），请联系管理员"})
		return
	}
	_, err := a.pg.Exec(r.Context(), `UPDATE users SET nickname=$1, updated_at=now() WHERE id=$2`,
		nickname, userIDFrom(r))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"nickname": nickname})
}

// changePassword 修改密码：必须校验旧密码，成功后吊销当前用户所有会话，要求重新登录。
// 这样可防止密码被盗后攻击者保持登录态。
func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	if len(req.NewPassword) < 8 || len(req.NewPassword) > 128 {
		writeJSON(w, 400, map[string]string{"error": "新密码长度须为 8-128 个字符"})
		return
	}
	var pwHash string
	if err := a.pg.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1`,
		userIDFrom(r)).Scan(&pwHash); err != nil {
		writeJSON(w, 404, map[string]string{"error": "user not found"})
		return
	}
	if !checkPassword(pwHash, req.OldPassword) {
		writeJSON(w, 400, map[string]string{"error": "旧密码不正确"})
		return
	}
	newHash, err := hashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "hash password failed"})
		return
	}
	ctx := r.Context()
	tx, err := a.pg.Begin(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash=$1, updated_at=now() WHERE id=$2`,
		newHash, userIDFrom(r)); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE user_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`,
		userIDFrom(r)); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"status": "password_changed", "relogin_required": true})
}

// myPlan 我的套餐详情：套餐额度 + 本月用量（含剩余），以及客户 Key 数量。
// 用量直接查 usage_logs，不依赖 v_usage_cost 视图，避免旧库未建视图时报错。
func (a *API) myPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var tenantID string
	if err := a.pg.QueryRow(ctx, `SELECT id::text FROM tenants WHERE owner_user_id=$1`,
		userIDFrom(r)).Scan(&tenantID); err != nil {
		writeJSON(w, 404, map[string]string{"error": "tenant not found"})
		return
	}
	var planName, planCode string
	var monthlyLimit, rpmLimit, concLimit, dailyLimit int64
	var price float64
	var currency, status string
	err := a.pg.QueryRow(ctx, `
		SELECT COALESCE(p.name,'未设置'), COALESCE(p.code,''),
		       COALESCE(p.monthly_token_limit,0)::bigint, COALESCE(p.rpm_limit,0)::bigint,
		       COALESCE(p.concurrency_limit,0)::bigint, COALESCE(p.daily_token_limit,0)::bigint,
		       COALESCE(p.price_per_month,0)::float8, COALESCE(p.currency,'CNY'),
		       COALESCE(s.status,'none')
		FROM subscriptions s
		LEFT JOIN plans p ON p.id=s.plan_id
		WHERE s.tenant_id=$1`, tenantID).
		Scan(&planName, &planCode, &monthlyLimit, &rpmLimit, &concLimit, &dailyLimit, &price, &currency, &status)
	if err != nil {
		// 没有订阅记录也要给用户一个可用响应，而不是报错。
		writeJSON(w, 200, map[string]any{
			"plan": "未购买", "status": "none", "monthly_token_limit": 0,
			"month_tokens": 0, "remaining_tokens": 0, "key_count": 0,
		})
		return
	}
	var monthTokens int64
	_ = a.pg.QueryRow(ctx, `
		SELECT COALESCE(SUM(prompt_tokens + completion_tokens),0)::bigint
		FROM usage_logs WHERE tenant_id=$1 AND created_at >= date_trunc('month', now())`, tenantID).Scan(&monthTokens)
	var keyCount int
	_ = a.pg.QueryRow(ctx, `SELECT COUNT(*)::int FROM api_keys WHERE tenant_id=$1 AND status='active'`, tenantID).Scan(&keyCount)
	remaining := monthlyLimit - monthTokens
	if remaining < 0 {
		remaining = 0
	}
	writeJSON(w, 200, map[string]any{
		"tenant_id": tenantID, "plan": planName, "plan_code": planCode, "status": status,
		"monthly_token_limit": monthlyLimit, "month_tokens": monthTokens, "remaining_tokens": remaining,
		"rpm_limit": rpmLimit, "concurrency_limit": concLimit, "daily_token_limit": dailyLimit,
		"price_per_month": price, "currency": currency, "key_count": keyCount,
	})
}

// publicConfig 下发前端需要的公开配置。私钥、密钥一律不返回。
// 网关地址优先取 PUBLIC_BASE_URL，未配置时回退到当前请求的 Host（便于本机/测试环境）。
func (a *API) publicConfig(w http.ResponseWriter, r *http.Request) {
	baseURL := strings.TrimSpace(a.cfg.PublicBaseURL)
	if baseURL == "" {
		scheme := "https"
		if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "http"
		}
		baseURL = scheme + "://" + r.Host
	}
	writeJSON(w, 200, map[string]any{
		"api_base_url": strings.TrimRight(baseURL, "/"),
		"chat_path":    "/v1/chat/completions",
	})
}

// keyInfo 返回当前用户的 API Key 前缀（不返回明文/哈希），便于用户核对已开通的 Key。
func (a *API) keyInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var tenantID string
	if err := a.pg.QueryRow(ctx, `SELECT id::text FROM tenants WHERE owner_user_id=$1`,
		userIDFrom(r)).Scan(&tenantID); err != nil {
		writeJSON(w, 404, map[string]string{"error": "tenant not found"})
		return
	}
	rows, err := a.pg.Query(ctx, `
		SELECT key_prefix, status, created_at FROM api_keys
		WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 20`, tenantID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	type keyView struct {
		Prefix    string `json:"key_prefix"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
	}
	keys := []keyView{}
	for rows.Next() {
		var prefix, status, created string
		if err := rows.Scan(&prefix, &status, &created); err != nil {
			continue
		}
		keys = append(keys, keyView{Prefix: prefix, Status: status, CreatedAt: created})
	}
	writeJSON(w, 200, map[string]any{"keys": keys})
}
