package server

import (
	"context"
	"net/http"
	"time"

	"aiporto/internal/config"
	"aiporto/internal/consoleapi"
	"aiporto/internal/mailer"
	"aiporto/internal/metrics"
	"aiporto/internal/payment"
	"aiporto/internal/proxy"
	"aiporto/internal/store"
	"aiporto/internal/userapi"
	"github.com/go-chi/chi/v5"
)

// rootPage 是网关根路径的静态落地页。API 网关本身没有首页，浏览器直接打开
// 根域名会看到 Go 默认的 "404 page not found"，容易被误判为反代故障。
// 这里返回一个纯静态说明页：不读取任何配置、不触碰后端、不泄露敏感信息。
const rootPage = `<!doctype html>
<html lang="zh-CN"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Aiporto 智港 · API 网关</title>
<style>
  body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;
    background:#f5f7fa;color:#1f2937}
  .card{max-width:620px;margin:24px;padding:40px;background:#fff;border-radius:14px;
    box-shadow:0 8px 30px rgba(0,0,0,.06)}
  h1{margin:0;font-size:30px;color:#111827}
  .slogan{margin:6px 0 22px;color:#6b7280;font-size:15px;letter-spacing:2px}
  .ok{display:inline-block;margin-bottom:18px;padding:4px 12px;border-radius:999px;
    background:#ecfdf5;color:#047857;font-size:13px}
  p{line-height:1.7;color:#374151}
  code{display:inline-block;padding:2px 8px;background:#f3f4f6;border-radius:6px;
    font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:13px;color:#0f172a}
  .foot{margin-top:22px;padding-top:18px;border-top:1px solid #e5e7eb;color:#9ca3af;font-size:12px}
</style></head><body>
<div class="card">
  <h1>Aiporto <span style="font-weight:400">智港</span></h1>
  <div class="slogan">万模入港，统一调度</div>
  <span class="ok">网关运行中</span>
  <p>这是 <strong>Aiporto Workbench</strong> 的大模型 API 网关端点，仅提供程序接口，没有可供浏览的官网首页。</p>
  <p>客户端请调用 <code>POST /v1/chat/completions</code>，使用签发的客户 Key 鉴权；健康检查见 <code>GET /healthz</code>。</p>
  <p>管理后台与客户控制台请通过你配置的<strong>后台域名</strong>访问（默认 <code>/admin</code> 与 <code>/console</code>），不在本端点提供。</p>
  <div class="foot">看到此页说明反向代理已连通、网关正常工作。请勿在浏览器直接调用 API，也未配置任何密钥。</div>
</div>
</body></html>`

func New(addr string, h *proxy.Handler, st *store.Store, cfg config.Config) *http.Server {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(rootPage))
	})

	r.Get("/metrics", metrics.Handler)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := st.PG.Ping(ctx); err != nil {
			http.Error(w, "postgres unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := st.RDB.Ping(ctx).Err(); err != nil {
			http.Error(w, "redis unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.HandleFunc("/v1/*", h.ServeHTTP)

	// B 方案：用户端与支付。邮件未配置时 mailer 为 nil，注册接口返回 503 提示。
	mail := mailer.New(mailer.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser,
		Pass: cfg.SMTPPass, From: cfg.SMTPFrom, TLS: cfg.SMTPTLS,
	})
	providers := payment.New(cfg)
	userAPI := userapi.New(st.PG, cfg, mail, providers)

	consoleAPI := consoleapi.New(st.PG, cfg)
	// 后台订单管理复用管理员双因素鉴权，作为额外路由注入。
	consoleAPI.Mount(r, userAPI.RegisterAdminRoutes)
	userAPI.Mount(r)

	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// 不设置 WriteTimeout：流式响应可能持续数分钟。
	}
}
