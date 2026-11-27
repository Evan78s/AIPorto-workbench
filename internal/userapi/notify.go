package userapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"aiporto/internal/payment"
	"aiporto/internal/util"
)

// alipayNotify 支付宝异步通知：表单字段，验签后开通。
func (a *API) alipayNotify(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	body := map[string]string{}
	for k, v := range r.PostForm {
		if len(v) > 0 {
			body[k] = v[0]
		}
	}
	p, ok := a.providers[payment.ChannelAlipay]
	if !ok || !p.Enabled() {
		http.Error(w, "alipay disabled", http.StatusBadRequest)
		return
	}
	res, err := p.VerifyNotify(nil, body)
	if err != nil {
		http.Error(w, "verify failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.handlePaid(r.Context(), res); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// 支付宝要求成功响应严格为 success 两个字。
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("success"))
}

// wechatNotify 微信 v3 回调：JSON body，验签头 + 解密 resource。
func (a *API) wechatNotify(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	header := map[string]string{}
	for k, v := range r.Header {
		if len(v) > 0 {
			header[http.CanonicalHeaderKey(k)] = v[0]
		}
	}
	body := map[string]string{"raw": string(raw)}
	p, ok := a.providers[payment.ChannelWechat]
	if !ok || !p.Enabled() {
		http.Error(w, "wechat disabled", http.StatusBadRequest)
		return
	}
	res, err := p.VerifyNotify(header, body)
	if err != nil {
		http.Error(w, "verify failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.handlePaid(r.Context(), res); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"成功"}`))
}

// handlePaid 是两种渠道共用的幂等开通逻辑，只返回错误，响应交给调用方。
func (a *API) handlePaid(ctx context.Context, res payment.NotifyResult) error {
	// 1. 记录原始流水（唯一约束保证重复回调不重复插入）。
	_, _ = a.pg.Exec(ctx, `
		INSERT INTO payments(id, order_id, channel, external_id, amount, status, raw_notify)
		SELECT $1, o.id, $2, $3, $4, 'succeeded', $5::jsonb
		FROM orders o WHERE o.order_no=$3
		ON CONFLICT (channel, external_id) DO NOTHING`,
		util.NewID(), string(res.Channel), res.ExternalID, res.Amount, `{}`)

	// 2. 查订单并比对金额（微信回调是分，统一到元）。
	var orderID, status, amountDB, channel string
	err := a.pg.QueryRow(ctx, `
		SELECT id::text, status, amount::text, payment_channel FROM orders WHERE order_no=$1`,
		res.ExternalID).Scan(&orderID, &status, &amountDB, &channel)
	if err != nil {
		return fmt.Errorf("order not found")
	}
	if channel != string(res.Channel) {
		return fmt.Errorf("channel mismatch")
	}
	amountPaid := res.Amount
	if res.Channel == payment.ChannelWechat {
		if fen, err := strconv.Atoi(amountPaid); err == nil {
			amountPaid = fmt.Sprintf("%.2f", float64(fen)/100)
		}
	}
	if normalizeAmount(amountPaid) != normalizeAmount(amountDB) {
		return fmt.Errorf("amount mismatch")
	}

	// 3. 幂等置为已支付：只有 pending 才会更新成功。重复回调不视为错误。
	if status == "pending" {
		tag, err := a.pg.Exec(ctx, `
			UPDATE orders SET status='paid', paid_at=now(), updated_at=now()
			WHERE id=$1 AND status='pending'`, orderID)
		if err != nil {
			return fmt.Errorf("update order failed: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil // 另一个并发回调已处理，幂等成功。
		}
	}
	// 4. 幂等开通订阅与密钥；明文加密暂存，用户领取一次后清空。
	_, _, err = a.grantOrder(ctx, res.ExternalID)
	if err != nil && !strings.Contains(err.Error(), "already") {
		return fmt.Errorf("grant failed: %w", err)
	}
	return nil
}

func normalizeAmount(s string) string {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return strings.TrimSpace(s)
	}
	return fmt.Sprintf("%.2f", f)
}
