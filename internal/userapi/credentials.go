package userapi

import (
	"net/http"
	"strings"

	"aiporto/internal/util"
	"github.com/go-chi/chi/v5"
)

// takeCredentials 用户领取一次性明文凭据：解密后立刻清空字段。重复领取返回 409。
func (a *API) takeCredentials(w http.ResponseWriter, r *http.Request) {
	no := chi.URLParam(r, "id")
	ctx := r.Context()

	// 事务内锁定，防止并发重复领取。
	tx, err := a.pg.Begin(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)
	var blob []byte
	var status string
	err = tx.QueryRow(ctx, `
		SELECT pending_credentials, status FROM orders
		WHERE order_no=$1 AND user_id=$2 FOR UPDATE`, no, userIDFrom(r)).Scan(&blob, &status)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "order not found"})
		return
	}
	if status != "paid" {
		writeJSON(w, 409, map[string]string{"error": "order is not paid"})
		return
	}
	if len(blob) == 0 {
		writeJSON(w, 409, map[string]string{"error": "credentials already taken or not generated"})
		return
	}
	plain, err := util.DecryptKey(a.masterKey, blob)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "decrypt credentials failed"})
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE orders SET pending_credentials=NULL, credentials_taken_at=now() WHERE order_no=$1`, no); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	parts := map[string]string{}
	for _, line := range strings.Split(string(plain), "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			parts[kv[0]] = kv[1]
		}
	}
	writeJSON(w, 200, map[string]any{
		"api_key_plaintext": parts["API_KEY"],
		"console_token_plaintext": parts["CONSOLE_TOKEN"],
		"note": "两项明文只显示这一次，请立即保存到安全的地方",
	})
}
