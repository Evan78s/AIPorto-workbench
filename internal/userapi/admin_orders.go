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
