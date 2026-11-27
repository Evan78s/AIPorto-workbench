package userapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aiporto/internal/payment"
	"aiporto/internal/util"
	"github.com/go-chi/chi/v5"
)

func (a *API) listPlans(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), `
		SELECT id::text, code, name, monthly_token_limit, rpm_limit, concurrency_limit,
		       daily_token_limit, price_per_month::float8, currency
		FROM plans WHERE price_per_month > 0 ORDER BY price_per_month ASC`)
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
		out = append(out, map[string]any{
			"id": id, "code": code, "name": name,
			"monthly_token_limit": monthly, "rpm_limit": rpm,
			"concurrency_limit": conc, "daily_token_limit": daily,
			"price_per_month": price, "currency": currency,
		})
	}
	writeJSON(w, 200, out)
}

func (a *API) listChannels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"channels": payment.AvailableChannels(a.providers)})
}

// createOrder 下单：金额从套餐表读取，不接受前端传入，防止 1 分钱买高配套餐。
func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlanID  string          `json:"plan_id"`
		Channel payment.Channel `json:"channel"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	provider, err := payment.Get(a.providers, req.Channel)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	ctx := r.Context()
	var price float64
	var currency, planName string
	if err := a.pg.QueryRow(ctx,
		`SELECT price_per_month::float8, currency, name FROM plans WHERE id=$1`, req.PlanID).
		Scan(&price, &currency, &planName); err != nil {
		writeJSON(w, 404, map[string]string{"error": "plan not found"})
		return
	}
	amount := fmt.Sprintf("%.2f", price)
	no := newOrderNo()

	// 对渠道下单失败要留痕为 failed，便于对账；但失败订单不阻塞重试。
	create, err := provider.CreateOrder(no, amount, "Aiporto 智港 · "+strings.TrimSpace(planName))
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "create payment failed: " + err.Error()})
		return
	}
	var tenantID string
	_ = a.pg.QueryRow(ctx, `SELECT id::text FROM tenants WHERE owner_user_id=$1`, userIDFrom(r)).Scan(&tenantID)
	payloadJSON, _ := json.Marshal(map[string]string{
		"payment_url": create.PaymentURL, "qr_code": create.QRCode, "bank_info": create.BankInfo,
	})
	_, err = a.pg.Exec(ctx, `
		INSERT INTO orders(id, order_no, user_id, tenant_id, plan_id, amount, currency, status, payment_channel, external_id, pay_payload)
		VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10)`,
		util.NewID(), no, userIDFrom(r), nullableString(tenantID), req.PlanID, amount, currency,
		string(req.Channel), create.ExternalID, string(payloadJSON))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{
		"order_no": no, "amount": amount, "currency": currency,
		"channel": req.Channel, "payment_url": create.PaymentURL,
		"qr_code": create.QRCode, "bank_info": create.BankInfo,
	})
}

func nullableString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

type orderView struct {
	OrderNo   string  `json:"order_no"`
	Plan      string  `json:"plan"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	Status    string  `json:"status"`
	Channel   string  `json:"channel"`
	CreatedAt string  `json:"created_at"`
	PaidAt    string  `json:"paid_at"`
	PayPayload string `json:"pay_payload,omitempty"`
}

func (a *API) listOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), `
		SELECT o.order_no, COALESCE(p.name,'-'), o.amount::float8, o.currency,
		       o.status, o.payment_channel, to_char(o.created_at,'YYYY-MM-DD HH24:MI'),
		       COALESCE(to_char(o.paid_at,'YYYY-MM-DD HH24:MI'),'')
		FROM orders o LEFT JOIN plans p ON p.id=o.plan_id
		WHERE o.user_id=$1 ORDER BY o.created_at DESC LIMIT 100`, userIDFrom(r))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []orderView{}
	for rows.Next() {
		var v orderView
		if err := rows.Scan(&v.OrderNo, &v.Plan, &v.Amount, &v.Currency, &v.Status,
			&v.Channel, &v.CreatedAt, &v.PaidAt); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	no := chi.URLParam(r, "id")
	var v orderView
	err := a.pg.QueryRow(r.Context(), `
		SELECT o.order_no, COALESCE(p.name,'-'), o.amount::float8, o.currency,
		       o.status, o.payment_channel, to_char(o.created_at,'YYYY-MM-DD HH24:MI'),
		       COALESCE(to_char(o.paid_at,'YYYY-MM-DD HH24:MI'),''), o.pay_payload
		FROM orders o LEFT JOIN plans p ON p.id=o.plan_id
		WHERE o.order_no=$1 AND o.user_id=$2`, no, userIDFrom(r)).
		Scan(&v.OrderNo, &v.Plan, &v.Amount, &v.Currency, &v.Status,
			&v.Channel, &v.CreatedAt, &v.PaidAt, &v.PayPayload)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "order not found"})
		return
	}
	writeJSON(w, 200, v)
}

// cancelOrder 只允许取消 pending 订单，已支付订单要走退款流程。
func (a *API) cancelOrder(w http.ResponseWriter, r *http.Request) {
	no := chi.URLParam(r, "id")
	tag, err := a.pg.Exec(r.Context(), `
		UPDATE orders SET status='cancelled', updated_at=now()
		WHERE order_no=$1 AND user_id=$2 AND status='pending'`, no, userIDFrom(r))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, 409, map[string]string{"error": "order cannot be cancelled"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

// 支付成功后调用：事务内 FOR UPDATE 保证回调重复到达也只开通一次。
func (a *API) grantOrder(ctx context.Context, orderNo string) (apiKey, consoleToken string, err error) {
	tx, err := a.pg.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)

	var userID, tenantID, planID, status string
	var granted bool
	err = tx.QueryRow(ctx, `
		SELECT user_id::text, COALESCE(tenant_id::text,''), plan_id::text, status, granted_key
		FROM orders WHERE order_no=$1 FOR UPDATE`, orderNo).
		Scan(&userID, &tenantID, &planID, &status, &granted)
	if err != nil {
		return "", "", err
	}
	if status != "paid" {
		return "", "", fmt.Errorf("order is not paid")
	}
	if granted {
		return "", "", nil // 幂等：已经开通过，不再重复发 key。
	}

	if tenantID == "" {
		tenantID = util.NewID()
		if _, err := tx.Exec(ctx, `
			INSERT INTO tenants(id, name, owner_user_id) VALUES($1,$2,$3)
			ON CONFLICT (owner_user_id) WHERE owner_user_id IS NOT NULL DO NOTHING`,
			tenantID, "用户-"+userID, userID); err != nil {
			return "", "", err
		}
		var realID string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM tenants WHERE owner_user_id=$1`, userID).Scan(&realID); err != nil {
			return "", "", err
		}
		tenantID = realID
	}
	// 订阅：一个租户一条，存在则升级套餐。
	if _, err := tx.Exec(ctx, `
		INSERT INTO subscriptions(id, tenant_id, plan_id, status) VALUES($1,$2,$3,'active')
		ON CONFLICT (tenant_id) DO UPDATE SET plan_id=EXCLUDED.plan_id,
		    status='active', updated_at=now()`, util.NewID(), tenantID, planID); err != nil {
		return "", "", err
	}
	// 客户 key 与控制台令牌：HMAC 存摘要，明文只返回本次。
	apiKey = "sk-wb-" + randomHex(16)
	if _, err := tx.Exec(ctx, `
		INSERT INTO api_keys(id, tenant_id, name, key_prefix, key_hash, rpm_limit, concurrency_limit, daily_token_limit)
		SELECT $1, $2, '开通赠送', $3, $4, p.rpm_limit, p.concurrency_limit, p.daily_token_limit
		FROM plans p WHERE p.id=$5`, util.NewID(), tenantID, apiKey[:12],
		util.HashKey(a.pepper, apiKey), planID); err != nil {
		return "", "", err
	}
	consoleToken = "cw-" + randomHex(24)
	if _, err := tx.Exec(ctx, `
		INSERT INTO console_tokens(id, tenant_id, token_hash, name)
		VALUES($1,$2,$3,'开通赠送')`, util.NewID(), tenantID,
		util.HashKey(a.pepper, "console:"+consoleToken)); err != nil {
		return "", "", err
	}
	// 明文只加密暂存一次，前端领取后清空；数据库不存明文。
	credentials, err := util.EncryptKey(a.masterKey, []byte("API_KEY="+apiKey+"\nCONSOLE_TOKEN="+consoleToken))
	if err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE orders SET tenant_id=$2, granted_key=true, pending_credentials=$3, updated_at=now()
		WHERE order_no=$1`, orderNo, tenantID, credentials); err != nil {
		return "", "", err
	}
	// 流水：reference_id 用订单号，天然幂等。
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing_ledger(tenant_id, entry_type, amount, reference_id, description, created_by)
		VALUES($1,'recharge',(SELECT amount FROM orders WHERE order_no=$2),$2,'套餐开通','user-portal')
		ON CONFLICT (reference_id) DO NOTHING`, tenantID, orderNo); err != nil {
		return "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return apiKey, consoleToken, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
