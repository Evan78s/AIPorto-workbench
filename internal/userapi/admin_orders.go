package userapi

import (
	"net/http"
	"strings"

	"aiporto/internal/util"
	"github.com/go-chi/chi/v5"
)

// AdminListOrders 运营后台订单列表。
func (a *API) AdminListOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := a.pg.Query(r.Context(), `
		SELECT o.order_no, COALESCE(u.email,'-'), COALESCE(p.name,'-'), o.amount::float8,
		       o.currency, o.status, o.payment_channel, o.external_id,
		       to_char(o.created_at,'YYYY-MM-DD HH24:MI'),
		       COALESCE(to_char(o.paid_at,'YYYY-MM-DD HH24:MI'),'')
		FROM orders o
		LEFT JOIN users u ON u.id=o.user_id
		LEFT JOIN plans p ON p.id=o.plan_id
		ORDER BY o.created_at DESC LIMIT 500`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var no, email, plan, currency, status, channel, externalID, created, paid string
		var amount float64
		if err := rows.Scan(&no, &email, &plan, &amount, &currency, &status, &channel,
			&externalID, &created, &paid); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		out = append(out, map[string]any{
			"order_no": no, "email": email, "plan": plan, "amount": amount,
			"currency": currency, "status": status, "channel": channel,
			"external_id": externalID, "created_at": created, "paid_at": paid,
		})
	}
	writeJSON(w, 200, out)
}

// AdminReissueCredentials 管理员补发密钥：针对已支付但凭据丢失/未生成的订单，
// 重新签发一对 API Key + 控制台令牌并暂存明文，供用户重新领取。
// 旧 key 不删除（避免破坏用户已有集成），新 key 追加到同一租户。
func (a *API) AdminReissueCredentials(w http.ResponseWriter, r *http.Request) {
	no := chi.URLParam(r, "id")
	ctx := r.Context()
	tx, err := a.pg.Begin(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)
	var userID, tenantID, planID, status string
	if err := tx.QueryRow(ctx, `
		SELECT user_id::text, COALESCE(tenant_id::text,''), plan_id::text, status
		FROM orders WHERE order_no=$1 FOR UPDATE`, no).
		Scan(&userID, &tenantID, &planID, &status); err != nil {
		writeJSON(w, 404, map[string]string{"error": "order not found"})
		return
	}
	if status != "paid" {
		writeJSON(w, 409, map[string]string{"error": "only paid orders can reissue credentials"})
		return
	}
	// 极端情况：订单已支付但从未开通租户，补建一个。
	if tenantID == "" {
		tenantID = util.NewID()
		if _, err := tx.Exec(ctx, `
			INSERT INTO tenants(id, name, owner_user_id) VALUES($1,$2,$3)
			ON CONFLICT (owner_user_id) WHERE owner_user_id IS NOT NULL DO NOTHING`,
			tenantID, "用户-"+userID, userID); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if err := tx.QueryRow(ctx, `SELECT id::text FROM tenants WHERE owner_user_id=$1`, userID).Scan(&tenantID); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}
	apiKey := "sk-wb-" + randomHex(16)
	if _, err := tx.Exec(ctx, `
		INSERT INTO api_keys(id, tenant_id, name, key_prefix, key_hash, rpm_limit, concurrency_limit, daily_token_limit)
		SELECT $1, $2, '补发密钥', $3, $4, p.rpm_limit, p.concurrency_limit, p.daily_token_limit
		FROM plans p WHERE p.id=$5`, util.NewID(), tenantID, apiKey[:12],
		util.HashKey(a.pepper, apiKey), planID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	consoleToken := "cw-" + randomHex(24)
	if _, err := tx.Exec(ctx, `
		INSERT INTO console_tokens(id, tenant_id, token_hash, name)
		VALUES($1,$2,$3,'补发令牌')`, util.NewID(), tenantID,
		util.HashKey(a.pepper, "console:"+consoleToken)); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	credentials, err := util.EncryptKey(a.masterKey, []byte("API_KEY="+apiKey+"\nCONSOLE_TOKEN="+consoleToken))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE orders SET tenant_id=$2, granted_key=true, pending_credentials=$3,
		    credentials_taken_at=NULL, updated_at=now() WHERE order_no=$1`,
		no, tenantID, credentials); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "reissued", "note": "用户现在可以重新领取密钥"})
}

// AdminConfirmManual 对公转账后台确认：把 pending 手动订单置为 paid 并开通。
func (a *API) AdminConfirmManual(w http.ResponseWriter, r *http.Request) {
	no := chi.URLParam(r, "id")
	ctx := r.Context()
	tx, err := a.pg.Begin(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)
	var channel, status, amount string
	if err := tx.QueryRow(ctx, `
		SELECT payment_channel, status, amount::text FROM orders
		WHERE order_no=$1 FOR UPDATE`, no).Scan(&channel, &status, &amount); err != nil {
		writeJSON(w, 404, map[string]string{"error": "order not found"})
		return
	}
	if channel != "manual" {
		writeJSON(w, 400, map[string]string{"error": "only manual orders can be confirmed here"})
		return
	}
	if status != "pending" {
		writeJSON(w, 409, map[string]string{"error": "order is not pending"})
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE orders SET status='paid', paid_at=now(), updated_at=now() WHERE order_no=$1`, no); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO payments(id, order_id, channel, external_id, amount, status, raw_notify)
		SELECT $1, id, 'manual', $2, $3, 'succeeded', '{"source":"admin-confirm"}'::jsonb
		FROM orders WHERE order_no=$2
		ON CONFLICT (channel, external_id) DO NOTHING`, util.NewID(), no, amount)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	// grantOrder 自己开事务；提交后再调用，避免嵌套事务。
	_, _, err = a.grantOrder(ctx, no)
	if err != nil && !strings.Contains(err.Error(), "already") {
		writeJSON(w, 500, map[string]string{"error": "grant failed: " + err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "confirmed"})
}
