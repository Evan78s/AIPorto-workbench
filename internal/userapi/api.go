// Package userapi 实现 B 方案用户端：注册、邮箱验证、登录、套餐浏览、下单、支付回调、
// 支付成功后自动开通租户/API key/控制台令牌/订阅，以及运营后台的订单管理与对公确认。
package userapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"aiporto/internal/config"
	"aiporto/internal/mailer"
	"aiporto/internal/payment"
	"aiporto/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type API struct {
	pg        *pgxpool.Pool
	pepper    []byte // HMAC 用：会话令牌、API key 摘要
	masterKey []byte // AES-GCM 用：暂存明文凭据，与 pepper 密钥分离
	cfg       config.Config
	mailer    *mailer.Mailer
	providers map[payment.Channel]payment.Provider
}

func New(pg *pgxpool.Pool, cfg config.Config, m *mailer.Mailer, providers map[payment.Channel]payment.Provider) *API {
	return &API{pg: pg, pepper: cfg.KeyPepper, masterKey: cfg.MasterKey, cfg: cfg, mailer: m, providers: providers}
}

// Mount 挂在网关根路由下，与 /api/admin、/api/console 平级。
// 运营后台订单接口也在这里注册，但复用 consoleapi 的管理员鉴权无法直接共享，
// 因此单独走 /api/admin/user-orders，且同样受 adminAuth 保护（见 consoleapi.MountAdminExtra）。
func (a *API) Mount(r chi.Router) {
	r.Route("/api/user", func(r chi.Router) {
		// 公开接口：注册、验证、登录、套餐列表、支付回调。
		r.Post("/register", a.register)
		r.Post("/verify", a.verifyEmail)
		r.Post("/login", a.login)
		r.Get("/plans", a.listPlans)
		r.Get("/config", a.publicConfig)
		r.Get("/pay/channels", a.listChannels)
		r.Post("/pay/alipay/notify", a.alipayNotify)
		r.Post("/pay/wechat/notify", a.wechatNotify)

		r.Group(func(r chi.Router) {
			r.Use(a.userAuth)
			r.Post("/logout", a.logout)
			r.Get("/me", a.me)
			r.Patch("/profile", a.updateProfile)
			r.Post("/password", a.changePassword)
			r.Get("/my-plan", a.myPlan)
			r.Get("/keys", a.keyInfo)
			r.Post("/keys/rotate", a.rotateKey)
			r.Post("/orders", a.createOrder)
			r.Get("/orders", a.listOrders)
			r.Get("/orders/{id}", a.getOrder)
			r.Post("/orders/{id}/cancel", a.cancelOrder)
			r.Post("/orders/{id}/credentials", a.takeCredentials)
		})
	})
}

type ctxKey string

const userCtxKey ctxKey = "user_id"

func (a *API) userAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if !strings.HasPrefix(token, userTokenPrefix) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid session"})
			return
		}
		var userID string
		err := a.pg.QueryRow(r.Context(),
			`SELECT user_id::text FROM user_sessions
			 WHERE token_hash=$1 AND expires_at > now() AND revoked_at IS NULL`,
			hashSession(a.pepper, token)).Scan(&userID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session expired or invalid"})
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFrom(r *http.Request) string {
	v, _ := r.Context().Value(userCtxKey).(string)
	return v
}

// RegisterAdminRoutes 由 server 在 consoleapi 的管理员鉴权组内调用。
func (a *API) RegisterAdminRoutes(r chi.Router) {
	r.Get("/user-orders", a.AdminListOrders)
	r.Post("/user-orders/{id}/confirm", a.AdminConfirmManual)
	r.Post("/user-orders/{id}/reissue", a.AdminReissueCredentials)
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// 订单号：时间 + 随机，便于人工识别且天然唯一。
func newOrderNo() string {
	return "AP" + time.Now().UTC().Format("20060102150405") + "-" + strings.ToUpper(util.NewID()[:8])
}
