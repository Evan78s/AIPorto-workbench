package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"aiporto/internal/config"
	"aiporto/internal/consoleapi"
	"aiporto/internal/metrics"
	"aiporto/internal/proxy"
	"aiporto/internal/store"
)

func New(addr string, h *proxy.Handler, st *store.Store, cfg config.Config) *http.Server {
	r := chi.NewRouter()

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

	consoleapi.New(st.PG, cfg).Mount(r)

	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// 不设置 WriteTimeout：流式响应可能持续数分钟。
	}
}
