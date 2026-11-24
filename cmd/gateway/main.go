package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aiporto/internal/auth"
	"aiporto/internal/config"
	"aiporto/internal/metering"
	"aiporto/internal/proxy"
	"aiporto/internal/quota"
	"aiporto/internal/routing"
	"aiporto/internal/server"
	"aiporto/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, err := store.New(ctx, cfg)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	reporter := metering.New(st.PG, cfg.MeteringBatchSize, cfg.MeteringFlush)
	reporter.Start(ctx)

	authn := auth.New(st, cfg.KeyPepper, cfg.AuthCacheTTL)
	limiter := quota.New(st.RDB)
	router := routing.New(st.PG, cfg.MasterKey, cfg.RouteCacheTTL)

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    true,
		ResponseHeaderTimeout: cfg.UpstreamHeaderTimeout,
		ForceAttemptHTTP2:     true,
	}
	// 不设置 Client.Timeout，否则会切断长 SSE 流；由 context 与响应头超时控制。
	client := &http.Client{Transport: transport}

	handler := proxy.New(authn, limiter, router, reporter, client)
	srv := server.New(cfg.ListenAddr, handler, st, cfg)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Printf("shutting down")
		shutdownCtx, done := context.WithTimeout(context.Background(), 15*time.Second)
		_ = srv.Shutdown(shutdownCtx)
		done()
		cancel()
	}()

	log.Printf("aiporto listening on %s", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
	reporter.Stop()
	log.Printf("stopped; dropped usage records: %d", reporter.Dropped())
}
