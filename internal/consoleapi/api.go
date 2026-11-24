package consoleapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"aiporto/internal/config"
	"aiporto/internal/util"
)

// 下面这些 SQL 是客户控制台访问数据的唯一入口。
// 每一条都必须带 tenant_id = $1，单元测试会逐条校验，防止漏加租户过滤。
const (
	consoleOverviewSQL = `SELECT
		COALESCE((SELECT SUM(prompt_tokens + completion_tokens) FROM v_usage_cost WHERE tenant_id = $1 AND created_at >= date_trunc('month', now())), 0)::bigint,
		COALESCE((SELECT SUM(cost) FROM v_usage_cost WHERE tenant_id = $1 AND created_at >= date_trunc('month', now())), 0)::float8,
		COALESCE((SELECT SUM(prompt_tokens + completion_tokens) FROM v_usage_cost WHERE tenant_id = $1 AND created_at >= date_trunc('day', now())), 0)::bigint,
		COALESCE(b.balance, 0)::float8
		FROM v_tenant_balance b WHERE b.tenant_id = $1`

	consoleByModelSQL = `SELECT model_resolved AS label,
		SUM(prompt_tokens + completion_tokens)::bigint AS tokens,
		SUM(cost)::float8 AS cost, COUNT(*)::bigint AS requests
		FROM v_usage_cost WHERE tenant_id = $1 AND created_at >= $2
		GROUP BY model_resolved ORDER BY tokens DESC`

	consoleDailySQL = `SELECT to_char(created_at, 'YYYY-MM-DD') AS label,
		SUM(prompt_tokens + completion_tokens)::bigint AS tokens,
		SUM(cost)::float8 AS cost, COUNT(*)::bigint AS requests
		FROM v_usage_cost WHERE tenant_id = $1 AND created_at >= $2
		GROUP BY 1 ORDER BY 1`

	consoleKeysSQL = `SELECT id::text, name, key_prefix, status, rpm_limit, concurrency_limit,
		daily_token_limit, COALESCE(last_used_at::text, ''), created_at::text
		FROM api_keys WHERE tenant_id = $1 ORDER BY created_at DESC`

	consoleRechargesSQL = `SELECT reference_id, amount::float8, currency, description, created_at::text
		FROM billing_ledger WHERE tenant_id = $1 AND entry_type IN ('recharge','refund','adjustment')
		ORDER BY created_at DESC`

	consoleUsageExportSQL = `SELECT request_id::text, to_char(created_at, 'YYYY-MM-DD HH24:MI:SS'), model_resolved,
		endpoint, prompt_tokens, completion_tokens, status_code, latency_ms, ttft_ms, cost::float8
		FROM v_usage_cost WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3
		ORDER BY created_at DESC`

	consoleBillExportSQL = `SELECT to_char(created_at, 'YYYY-MM-DD') AS day, model_resolved,
		SUM(prompt_tokens)::bigint, SUM(completion_tokens)::bigint, SUM(cost)::float8
		FROM v_usage_cost WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1, 2 ORDER BY 1, 2`
)

type ctxKey string

const tenantCtxKey ctxKey = "tenant_id"

type API struct {
	pg         *pgxpool.Pool
	pepper     []byte
	adminUser  string
	adminPass  string
	totpSecret string
	lowBalance float64
}

func New(pg *pgxpool.Pool, cfg config.Config) *API {
	return &API{
		pg:         pg,
		pepper:     cfg.KeyPepper,
		adminUser:  cfg.AdminUser,
		adminPass:  cfg.AdminPassword,
		totpSecret: cfg.AdminTOTPSecret,
		lowBalance: cfg.AlertLowBalance,
	}
}

func (a *API) Mount(r chi.Router) {
	r.Route("/api/admin", func(r chi.Router) {
		// 审计在最外层：鉴权失败的写尝试同样留痕。
		r.Use(a.auditWrite)
		// 登录两步本身不需要会话，但会被审计。
		r.Post("/auth/login", a.adminLogin)
		r.Post("/auth/verify", a.adminVerify)
		r.Group(func(r chi.Router) {
			r.Use(a.adminAuth)
			r.Get("/tenants", a.listTenants)
			r.Post("/tenants", a.createTenant)
			r.Post("/tenants/{id}/keys", a.issueKey)
			r.Get("/tenants/{id}/keys", a.adminListKeys)
			r.Put("/tenants/{id}/quota", a.updateQuota)
			r.Get("/plans", a.listPlans)
			r.Post("/plans", a.createPlan)
			r.Get("/usage", a.adminUsage)
			r.Get("/ledger", a.adminLedger)
			r.Post("/recharge", a.recharge)
			r.Get("/alerts", a.alerts)
			r.Put("/pricing", a.upsertPricing)
		})
	})
	r.Route("/api/console", func(r chi.Router) {
		r.Use(a.auditWrite)
		r.Use(a.consoleAuth)
		r.Get("/overview", a.consoleOverview)
		r.Get("/usage/by-model", a.consoleByModel)
		r.Get("/usage/daily", a.consoleDaily)
		r.Get("/keys", a.consoleKeys)
		r.Post("/keys/{id}/disable", a.consoleDisableKey)
		r.Post("/keys/rotate", a.consoleRotateKey)
		r.Get("/recharges", a.consoleRecharges)
		r.Get("/export/usage.csv", a.exportUsage)
		r.Get("/export/bill.csv", a.exportBill)
		r.Get("/snippets", a.snippets)
	})
}

func (a *API) consoleAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if !strings.HasPrefix(token, "cw-") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid console token"})
			return
		}
		hash := util.HashKey(a.pepper, "console:"+token)
		var tenantID string
		err := a.pg.QueryRow(r.Context(),
			`SELECT tenant_id::text FROM console_tokens WHERE token_hash = $1 AND status = 'active'`, hash).Scan(&tenantID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid console token"})
			return
		}
		_, _ = a.pg.Exec(r.Context(), `UPDATE console_tokens SET last_used_at = now() WHERE token_hash = $1`, hash)
		ctx := context.WithValue(r.Context(), tenantCtxKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func tenantIDFrom(r *http.Request) string {
	v, _ := r.Context().Value(tenantCtxKey).(string)
	return v
}

type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (a *API) insertAPIKey(ctx context.Context, q querier, tenantID string, rpm, conc int, daily int64) (string, error) {
	plain := "sk-wb-" + randomHex(16)
	_, err := q.Exec(ctx, `
		INSERT INTO api_keys(id, tenant_id, name, key_prefix, key_hash, rpm_limit, concurrency_limit, daily_token_limit)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8)
	`, util.NewID(), tenantID, "issued-by-admin", plain[:12], util.HashKey(a.pepper, plain), rpm, conc, daily)
	return plain, err
}

func (a *API) insertConsoleToken(ctx context.Context, q querier, tenantID, name string) (string, error) {
	plain := "cw-" + randomHex(24)
	_, err := q.Exec(ctx, `
		INSERT INTO console_tokens(id, tenant_id, token_hash, name) VALUES($1, $2, $3, $4)
	`, util.NewID(), tenantID, util.HashKey(a.pepper, "console:"+plain), name)
	return plain, err
}

// ---------------- 运营后台 ----------------

type tenantRow struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Plan        string  `json:"plan"`
	Balance     float64 `json:"balance"`
	MonthTokens int64   `json:"month_tokens"`
	MonthCost   float64 `json:"month_cost"`
	ActiveKeys  int64   `json:"active_keys"`
}

func (a *API) listTenants(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), `
		SELECT t.id::text, t.name, t.status, COALESCE(p.name, '未设置'),
		       COALESCE(b.balance,0)::float8, COALESCE(b.month_tokens,0)::bigint,
		       COALESCE(b.month_cost,0)::float8,
		       (SELECT COUNT(*)::bigint FROM api_keys k WHERE k.tenant_id=t.id AND k.status='active')
		FROM tenants t
		LEFT JOIN subscriptions s ON s.tenant_id=t.id
		LEFT JOIN plans p ON p.id=s.plan_id
		LEFT JOIN v_tenant_balance b ON b.tenant_id=t.id
		WHERE t.status <> 'archived' ORDER BY t.created_at DESC`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []tenantRow{}
	for rows.Next() {
		var t tenantRow
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.Plan, &t.Balance, &t.MonthTokens, &t.MonthCost, &t.ActiveKeys); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, t)
	}
	writeJSON(w, 200, out)
}

type createTenantReq struct {
	Name             string `json:"name"`
	PlanCode         string `json:"plan_code"`
	RpmLimit         int    `json:"rpm_limit"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
	DailyTokenLimit  int64  `json:"daily_token_limit"`
	MonthlyTokenLimit int64 `json:"monthly_token_limit"`
}

func (a *API) createTenant(w http.ResponseWriter, r *http.Request) {
	var req createTenantReq
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.RpmLimit <= 0 || req.ConcurrencyLimit <= 0 || req.DailyTokenLimit <= 0 {
		writeJSON(w, 400, map[string]string{"error": "limits must be positive"})
		return
	}

	ctx := r.Context()
	tx, err := a.pg.Begin(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)

	tenantID := util.NewID()
	if _, err := tx.Exec(ctx, `INSERT INTO tenants(id, name) VALUES($1,$2)`, tenantID, req.Name); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	planCode := strings.TrimSpace(req.PlanCode)
	if planCode == "" {
		planCode = "plan-" + randomHex(6)
		monthly := req.MonthlyTokenLimit
		if monthly <= 0 {
			monthly = 1000000
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO plans(id, code, name, monthly_token_limit, rpm_limit, concurrency_limit, daily_token_limit)
			VALUES($1,$2,$3,$4,$5,$6,$7)
		`, util.NewID(), planCode, req.Name+" 默认套餐", monthly, req.RpmLimit, req.ConcurrencyLimit, req.DailyTokenLimit); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}
	var planID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM plans WHERE code=$1`, planCode).Scan(&planID); err != nil {
		writeJSON(w, 400, map[string]string{"error": "plan code not found"})
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO subscriptions(id, tenant_id, plan_id) VALUES($1,$2,$3)`, util.NewID(), tenantID, planID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	apiKey, err := a.insertAPIKey(ctx, tx, tenantID, req.RpmLimit, req.ConcurrencyLimit, req.DailyTokenLimit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	consoleToken, err := a.insertConsoleToken(ctx, tx, tenantID, "默认控制台令牌")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{
		"tenant_id":            tenantID,
		"api_key_plaintext":    apiKey,
		"console_token_plaintext": consoleToken,
		"note":                 "两项明文只返回这一次，请立即保存",
	})
}

func (a *API) issueKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")
	var req struct {
		RpmLimit         int   `json:"rpm_limit"`
		ConcurrencyLimit int   `json:"concurrency_limit"`
		DailyTokenLimit  int64 `json:"daily_token_limit"`
	}
	if err := decodeJSON(r, &req); err != nil || req.RpmLimit <= 0 || req.ConcurrencyLimit <= 0 || req.DailyTokenLimit <= 0 {
		writeJSON(w, 400, map[string]string{"error": "positive rpm_limit, concurrency_limit, daily_token_limit required"})
		return
	}
	plain, err := a.insertAPIKey(r.Context(), a.pg, tenantID, req.RpmLimit, req.ConcurrencyLimit, req.DailyTokenLimit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"api_key_plaintext": plain, "note": "只返回这一次"})
}

func (a *API) adminListKeys(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), `
		SELECT id::text, name, key_prefix, status, rpm_limit, concurrency_limit,
		       daily_token_limit, COALESCE(last_used_at::text,''), created_at::text
		FROM api_keys WHERE tenant_id=$1 ORDER BY created_at DESC`, chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	writeJSON(w, 200, scanKeys(rows))
}

type quotaReq struct {
	PlanID           string `json:"plan_id"`
	MonthlyTokenLimit int64 `json:"monthly_token_limit"`
	RpmLimit         int    `json:"rpm_limit"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
	DailyTokenLimit  int64  `json:"daily_token_limit"`
}

func (a *API) updateQuota(w http.ResponseWriter, r *http.Request) {
	var req quotaReq
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	tenantID := chi.URLParam(r, "id")
	ctx := r.Context()
	tx, err := a.pg.Begin(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)
	override, _ := json.Marshal(map[string]any{
		"monthly_token_limit": req.MonthlyTokenLimit,
		"rpm_limit":           req.RpmLimit,
		"concurrency_limit":   req.ConcurrencyLimit,
		"daily_token_limit":   req.DailyTokenLimit,
	})
	_, err = tx.Exec(ctx, `
		UPDATE subscriptions SET plan_id = COALESCE(NULLIF($2,''), plan_id), quota_override=$3::jsonb, updated_at=now()
		WHERE tenant_id=$1`, tenantID, req.PlanID, string(override))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if req.RpmLimit > 0 && req.ConcurrencyLimit > 0 && req.DailyTokenLimit > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE api_keys SET rpm_limit=$2, concurrency_limit=$3, daily_token_limit=$4
			WHERE tenant_id=$1 AND status='active'`, tenantID, req.RpmLimit, req.ConcurrencyLimit, req.DailyTokenLimit); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (a *API) listPlans(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), `
		SELECT id::text, code, name, monthly_token_limit, rpm_limit, concurrency_limit,
		       daily_token_limit, price_per_month::float8, currency FROM plans ORDER BY created_at`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, code, name, currency string
		var monthly int64
		var rpm, conc int
		var daily int64
		var price float64
		if err := rows.Scan(&id, &code, &name, &monthly, &rpm, &conc, &daily, &price, &currency); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, map[string]any{"id": id, "code": code, "name": name,
			"monthly_token_limit": monthly, "rpm_limit": rpm, "concurrency_limit": conc,
			"daily_token_limit": daily, "price_per_month": price, "currency": currency})
	}
	writeJSON(w, 200, out)
}

func (a *API) createPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code             string  `json:"code"`
		Name             string  `json:"name"`
		MonthlyTokenLimit int64  `json:"monthly_token_limit"`
		RpmLimit         int     `json:"rpm_limit"`
		ConcurrencyLimit int     `json:"concurrency_limit"`
		DailyTokenLimit  int64   `json:"daily_token_limit"`
		PricePerMonth    float64 `json:"price_per_month"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Code == "" || req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "code and name required"})
		return
	}
	_, err := a.pg.Exec(r.Context(), `
		INSERT INTO plans(id, code, name, monthly_token_limit, rpm_limit, concurrency_limit, daily_token_limit, price_per_month)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (code) DO UPDATE SET name=EXCLUDED.name, monthly_token_limit=EXCLUDED.monthly_token_limit,
		rpm_limit=EXCLUDED.rpm_limit, concurrency_limit=EXCLUDED.concurrency_limit,
		daily_token_limit=EXCLUDED.daily_token_limit, price_per_month=EXCLUDED.price_per_month
	`, util.NewID(), req.Code, req.Name, req.MonthlyTokenLimit, req.RpmLimit, req.ConcurrencyLimit, req.DailyTokenLimit, req.PricePerMonth)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"status": "saved"})
}

type aggRow struct {
	Label    string  `json:"label"`
	Tokens   int64   `json:"tokens"`
	Cost     float64 `json:"cost"`
	Requests int64   `json:"requests"`
}

func (a *API) adminUsage(w http.ResponseWriter, r *http.Request) {
	from, to := rangeDates(r)
	group := r.URL.Query().Get("group")
	dimension := "to_char(created_at, 'YYYY-MM-DD')"
	switch group {
	case "tenant":
		dimension = "tenant_id"
	case "model":
		dimension = "model_resolved"
	}
	sql := fmt.Sprintf(`SELECT %s AS label, SUM(prompt_tokens+completion_tokens)::bigint,
		SUM(cost)::float8, COUNT(*)::bigint FROM v_usage_cost
		WHERE created_at >= $1 AND created_at < $2`, dimension)
	args := []any{from, to}
	if tid := r.URL.Query().Get("tenant_id"); tid != "" {
		sql += " AND tenant_id = $3"
		args = append(args, tid)
	}
	sql += " GROUP BY label ORDER BY label"
	a.writeAgg(w, r, sql, args...)
}

func (a *API) adminLedger(w http.ResponseWriter, r *http.Request) {
	sql := `SELECT reference_id, amount::float8, currency, description, created_at::text
		FROM billing_ledger WHERE 1=1`
	args := []any{}
	if tid := r.URL.Query().Get("tenant_id"); tid != "" {
		sql += " AND tenant_id = $1"
		args = append(args, tid)
	}
	sql += " ORDER BY created_at DESC LIMIT 500"
	rows, err := a.pg.Query(r.Context(), sql, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ref, currency, desc, created string
		var amount float64
		if err := rows.Scan(&ref, &amount, &currency, &desc, &created); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, map[string]any{"reference_id": ref, "amount": amount, "currency": currency,
			"description": desc, "created_at": created})
	}
	writeJSON(w, 200, out)
}

func (a *API) recharge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID    string  `json:"tenant_id"`
		Amount      float64 `json:"amount"`
		Note        string  `json:"note"`
		ReferenceID string  `json:"reference_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.TenantID == "" || req.Amount <= 0 {
		writeJSON(w, 400, map[string]string{"error": "tenant_id and positive amount required"})
		return
	}
	ref := strings.TrimSpace(req.ReferenceID)
	if ref == "" {
		ref = "R-" + util.NewID()
	}
	_, err := a.pg.Exec(r.Context(), `
		INSERT INTO billing_ledger(tenant_id, entry_type, amount, reference_id, description)
		VALUES($1,'recharge',$2,$3,$4)
	`, req.TenantID, req.Amount, ref, req.Note)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeJSON(w, 409, map[string]string{"error": "reference_id already exists"})
			return
		}
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"reference_id": ref, "status": "recharged"})
}

type alert struct {
	Type       string  `json:"type"`
	TenantID   string  `json:"tenant_id"`
	Message    string  `json:"message"`
	Value      float64 `json:"value"`
	Threshold  float64 `json:"threshold"`
	DetectedAt string  `json:"detected_at"`
}

func (a *API) alerts(w http.ResponseWriter, r *http.Request) {
	out := []alert{}
	now := time.Now().Format(time.RFC3339)

	rows, err := a.pg.Query(r.Context(), `SELECT tenant_id::text, balance::float8 FROM v_tenant_balance WHERE balance < $1`, a.lowBalance)
	if err == nil {
		for rows.Next() {
			var tid string
			var bal float64
			if rows.Scan(&tid, &bal) == nil {
				out = append(out, alert{"low_balance", tid,
					fmt.Sprintf("余额 %.2f 低于阈值 %.2f", bal, a.lowBalance), bal, a.lowBalance, now})
			}
		}
		rows.Close()
	}

	rows, err = a.pg.Query(r.Context(), `
		SELECT tenant_id::text, today::float8, avg7::float8 FROM (
			SELECT c.tenant_id,
			       SUM(c.prompt_tokens+c.completion_tokens)::float8 AS today,
			       COALESCE((SELECT AVG(d.tokens) FROM (
			           SELECT SUM(u.prompt_tokens+u.completion_tokens) AS tokens
			           FROM v_usage_cost u
			           WHERE u.created_at >= now() - interval '7 days' AND u.created_at < date_trunc('day', now())
			           GROUP BY date_trunc('day', u.created_at)
			       ) d), 0) AS avg7
			FROM v_usage_cost c WHERE c.created_at >= date_trunc('day', now())
			GROUP BY c.tenant_id
		) s WHERE avg7 > 1000 AND today > 3 * avg7`)
	if err == nil {
		for rows.Next() {
			var tid string
			var today, avg float64
			if rows.Scan(&tid, &today, &avg) == nil {
				out = append(out, alert{"usage_spike", tid,
					fmt.Sprintf("今日 token %.0f，超过近 7 日均值 %.0f 的 3 倍", today, avg), today, avg * 3, now})
			}
		}
		rows.Close()
	}

	rows, err = a.pg.Query(r.Context(), `
		SELECT tenant_id::text,
		       COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now()))::float8 AS today,
		       COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now()) - interval '1 day'
		                          AND created_at < date_trunc('day', now()))::float8 AS yesterday
		FROM usage_logs WHERE created_at >= date_trunc('day', now()) - interval '1 day'
		GROUP BY tenant_id
		HAVING COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now())) > 10
		   AND COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now())) >
		       3 * COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now()) - interval '1 day'
		                              AND created_at < date_trunc('day', now()))`)
	if err == nil {
		for rows.Next() {
			var tid string
			var today, yesterday float64
			if rows.Scan(&tid, &today, &yesterday) == nil {
				out = append(out, alert{"upstream_429_surge", tid,
					fmt.Sprintf("今日 429 次数 %.0f，昨日同期 %.0f", today, yesterday), today, yesterday * 3, now})
			}
		}
		rows.Close()
	}
	writeJSON(w, 200, out)
}

func (a *API) upsertPricing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model      string  `json:"model"`
		Input1M    float64 `json:"input_price_per_1m"`
		Output1M   float64 `json:"output_price_per_1m"`
		Currency   string  `json:"currency"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Model == "" {
		writeJSON(w, 400, map[string]string{"error": "model required"})
		return
	}
	currency := req.Currency
	if currency == "" {
		currency = "CNY"
	}
	_, err := a.pg.Exec(r.Context(), `
		INSERT INTO model_pricing(id, model, input_price_per_1m, output_price_per_1m, currency)
		VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (model) DO UPDATE SET input_price_per_1m=EXCLUDED.input_price_per_1m,
		output_price_per_1m=EXCLUDED.output_price_per_1m, currency=EXCLUDED.currency, updated_at=now()
	`, util.NewID(), req.Model, req.Input1M, req.Output1M, currency)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "saved"})
}

// ---------------- 客户控制台 ----------------

type keyRow struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	KeyPrefix        string `json:"key_prefix"`
	Status           string `json:"status"`
	RpmLimit         int    `json:"rpm_limit"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
	DailyTokenLimit  int64  `json:"daily_token_limit"`
	LastUsedAt       string `json:"last_used_at"`
	CreatedAt        string `json:"created_at"`
}

func scanKeys(rows pgx.Rows) []keyRow {
	out := []keyRow{}
	for rows.Next() {
		var k keyRow
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Status, &k.RpmLimit,
			&k.ConcurrencyLimit, &k.DailyTokenLimit, &k.LastUsedAt, &k.CreatedAt); err != nil {
			return out
		}
		out = append(out, k)
	}
	return out
}

func (a *API) consoleOverview(w http.ResponseWriter, r *http.Request) {
	tid := tenantIDFrom(r)
	var monthTokens, todayTokens int64
	var monthCost, balance float64
	err := a.pg.QueryRow(r.Context(), consoleOverviewSQL, tid).Scan(&monthTokens, &monthCost, &todayTokens, &balance)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var planName string
	var monthlyLimit int64
	_ = a.pg.QueryRow(r.Context(), `
		SELECT COALESCE(p.name,'未设置'), COALESCE(p.monthly_token_limit,0)::bigint
		FROM subscriptions s LEFT JOIN plans p ON p.id=s.plan_id WHERE s.tenant_id=$1`, tid).Scan(&planName, &monthlyLimit)
	remaining := monthlyLimit - monthTokens
	if remaining < 0 {
		remaining = 0
	}
	writeJSON(w, 200, map[string]any{
		"tenant_id": tid, "plan": planName, "month_tokens": monthTokens,
		"month_cost": monthCost, "today_tokens": todayTokens, "balance": balance,
		"monthly_token_limit": monthlyLimit, "remaining_tokens": remaining,
	})
}

func (a *API) consoleByModel(w http.ResponseWriter, r *http.Request) {
	from, _ := rangeDates(r)
	a.writeAgg(w, r, consoleByModelSQL, tenantIDFrom(r), from)
}

func (a *API) consoleDaily(w http.ResponseWriter, r *http.Request) {
	from, _ := rangeDates(r)
	a.writeAgg(w, r, consoleDailySQL, tenantIDFrom(r), from)
}

func (a *API) consoleKeys(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), consoleKeysSQL, tenantIDFrom(r))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	writeJSON(w, 200, scanKeys(rows))
}

func (a *API) consoleDisableKey(w http.ResponseWriter, r *http.Request) {
	tag, err := a.pg.Exec(r.Context(), `
		UPDATE api_keys SET status='revoked', revoked_at=now()
		WHERE id=$1 AND tenant_id=$2 AND status='active'`, chi.URLParam(r, "id"), tenantIDFrom(r))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "key not found or already disabled"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "disabled"})
}

func (a *API) consoleRotateKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tid := tenantIDFrom(r)
	tx, err := a.pg.Begin(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)
	var rpm, conc int
	var daily int64
	if err := tx.QueryRow(ctx, `
		SELECT rpm_limit, concurrency_limit, daily_token_limit FROM api_keys
		WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 1`, tid).Scan(&rpm, &conc, &daily); err != nil {
		writeJSON(w, 400, map[string]string{"error": "no existing key to rotate"})
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE api_keys SET status='revoked', revoked_at=now()
		WHERE tenant_id=$1 AND status='active'`, tid); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	plain, err := a.insertAPIKey(ctx, tx, tid, rpm, conc, daily)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"api_key_plaintext": plain, "note": "旧 key 已停用，新明文只显示这一次"})
}

func (a *API) consoleRecharges(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), consoleRechargesSQL, tenantIDFrom(r))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ref, currency, desc, created string
		var amount float64
		if err := rows.Scan(&ref, &amount, &currency, &desc, &created); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, map[string]any{"reference_id": ref, "amount": amount, "currency": currency,
			"description": desc, "created_at": created})
	}
	writeJSON(w, 200, out)
}

// UsageExportRow 是导出的白名单字段：没有 prompt、没有 key、没有租户信息。
type UsageExportRow struct {
	RequestID        string
	CreatedAt        string
	Model            string
	Endpoint         string
	PromptTokens     int
	CompletionTokens int
	StatusCode       int
	LatencyMS        int
	TTFTMS           int
	Cost             float64
}

func RenderUsageCSV(rows []UsageExportRow) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"request_id", "created_at", "model", "endpoint",
		"prompt_tokens", "completion_tokens", "status_code", "latency_ms", "ttft_ms", "cost"})
	for _, r := range rows {
		_ = w.Write([]string{
			r.RequestID, r.CreatedAt, r.Model, r.Endpoint,
			strconv.Itoa(r.PromptTokens), strconv.Itoa(r.CompletionTokens),
			strconv.Itoa(r.StatusCode), strconv.Itoa(r.LatencyMS),
			strconv.Itoa(r.TTFTMS), strconv.FormatFloat(r.Cost, 'f', 8, 64),
		})
	}
	w.Flush()
	return b.Bytes()
}

func (a *API) exportUsage(w http.ResponseWriter, r *http.Request) {
	from, to := rangeDates(r)
	rows, err := a.pg.Query(r.Context(), consoleUsageExportSQL, tenantIDFrom(r), from, to)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []UsageExportRow{}
	for rows.Next() {
		var x UsageExportRow
		if err := rows.Scan(&x.RequestID, &x.CreatedAt, &x.Model, &x.Endpoint,
			&x.PromptTokens, &x.CompletionTokens, &x.StatusCode, &x.LatencyMS, &x.TTFTMS, &x.Cost); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	writeCSV(w, "usage.csv", RenderUsageCSV(out))
}

func (a *API) exportBill(w http.ResponseWriter, r *http.Request) {
	from, to := rangeDates(r)
	rows, err := a.pg.Query(r.Context(), consoleBillExportSQL, tenantIDFrom(r), from, to)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var b bytes.Buffer
	cw := csv.NewWriter(&b)
	_ = cw.Write([]string{"day", "model", "prompt_tokens", "completion_tokens", "cost"})
	for rows.Next() {
		var day, model string
		var p, c int64
		var cost float64
		if err := rows.Scan(&day, &model, &p, &c, &cost); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		_ = cw.Write([]string{day, model, strconv.FormatInt(p, 10), strconv.FormatInt(c, 10),
			strconv.FormatFloat(cost, 'f', 8, 64)})
	}
	cw.Flush()
	writeCSV(w, "bill.csv", b.Bytes())
}

func (a *API) snippets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"curl": "curl -N -X POST http://127.0.0.1:8080/v1/chat/completions \\\n" +
			"  -H \"Authorization: Bearer <YOUR_API_KEY>\" \\\n" +
			"  -H \"Content-Type: application/json\" \\\n" +
			"  -d '{\"model\":\"<YOUR_MODEL_ALIAS>\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"你好\"}]}'",
		"python": "import os, requests\n\nr = requests.post(\n    'http://127.0.0.1:8080/v1/chat/completions',\n    headers={'Authorization': 'Bearer ' + os.environ['WB_API_KEY']},\n    json={'model': '<YOUR_MODEL_ALIAS>', 'stream': True,\n          'messages': [{'role': 'user', 'content': '你好'}]},\n    stream=True,\n)\nfor line in r.iter_lines():\n    if line:\n        print(line.decode())",
		"note": "<YOUR_API_KEY> 与 <YOUR_MODEL_ALIAS> 请替换为你自己的值；不要把真实 key 提交到代码仓库。",
	})
}

// ---------------- 公共工具 ----------------

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeCSV(w http.ResponseWriter, filename string, data []byte) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(200)
	_, _ = w.Write(data)
}

func (a *API) writeAgg(w http.ResponseWriter, r *http.Request, sql string, args ...any) {
	rows, err := a.pg.Query(r.Context(), sql, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []aggRow{}
	for rows.Next() {
		var x aggRow
		if err := rows.Scan(&x.Label, &x.Tokens, &x.Cost, &x.Requests); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
}

func rangeDates(r *http.Request) (time.Time, time.Time) {
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	layout := "2006-01-02"
	if from := r.URL.Query().Get("from"); from != "" {
		if t, err := time.ParseInLocation(layout, from, time.Local); err == nil {
			return t, time.Now()
		}
	}
	return time.Now().AddDate(0, 0, -days), time.Now()
}
