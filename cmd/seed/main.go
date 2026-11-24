package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"aiporto/internal/config"
	"aiporto/internal/util"
)

// 一次性初始化演示数据：租户、客户 key（明文只打印一次）、上游 provider、加密后的上游 key、路由规则。
// 上游 key 明文从环境变量 SEED_UPSTREAM_KEY_PLAIN 读取，不写进命令行参数。
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	tenantID := flag.String("tenant-id", util.NewID(), "tenant id")
	tenantName := flag.String("tenant-name", "Demo Tenant", "tenant display name")
	providerCode := flag.String("provider-code", "demo", "upstream provider code")
	baseURL := flag.String("provider-base-url", "https://example.invalid/v1", "upstream base origin, no trailing slash")
	alias := flag.String("model-alias", "demo-model", "model alias sent by clients")
	model := flag.String("model", "demo-model", "real upstream model name")
	rpm := flag.Int("rpm", 60, "requests per minute per key")
	concurrency := flag.Int("concurrency", 4, "max concurrent requests per key")
	dailyTokens := flag.Int64("daily-tokens", 1000000, "daily token cap per key")
	weight := flag.Int("weight", 1, "weight of this upstream key")
	flag.Parse()

	plainUpstream := os.Getenv("SEED_UPSTREAM_KEY_PLAIN")
	if plainUpstream == "" {
		log.Fatal("SEED_UPSTREAM_KEY_PLAIN is required")
	}

	ctx := context.Background()
	pg, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}
	defer pg.Close()

	if _, err := pg.Exec(ctx, `INSERT INTO tenants(id, name) VALUES($1, $2) ON CONFLICT (id) DO NOTHING`, *tenantID, *tenantName); err != nil {
		log.Fatalf("insert tenant: %v", err)
	}
	if _, err := pg.Exec(ctx, `INSERT INTO upstream_providers(id, code, base_url) VALUES($1, $2, $3) ON CONFLICT (code) DO NOTHING`,
		util.NewID(), *providerCode, *baseURL); err != nil {
		log.Fatalf("insert provider: %v", err)
	}

	var providerID string
	if err := pg.QueryRow(ctx, `SELECT id::text FROM upstream_providers WHERE code = $1`, *providerCode).Scan(&providerID); err != nil {
		log.Fatalf("query provider: %v", err)
	}

	customerKey := "sk-wb-" + randomHex(16)
	keyHash := util.HashKey(cfg.KeyPepper, customerKey)
	if _, err := pg.Exec(ctx, `
		INSERT INTO api_keys(id, tenant_id, name, key_prefix, key_hash, rpm_limit, concurrency_limit, daily_token_limit)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8)
	`, util.NewID(), *tenantID, "mvp-key", customerKey[:12], keyHash, *rpm, *concurrency, *dailyTokens); err != nil {
		log.Fatalf("insert api key: %v", err)
	}

	encrypted, err := util.EncryptKey(cfg.MasterKey, []byte(plainUpstream))
	if err != nil {
		log.Fatalf("encrypt upstream key: %v", err)
	}
	if _, err := pg.Exec(ctx, `
		INSERT INTO upstream_keys(id, provider_id, tenant_id, name, encrypted_key, key_hint, priority, weight)
		VALUES($1, $2, NULL, $3, $4, $5, 100, $6)
	`, util.NewID(), providerID, "upstream-1", encrypted, util.KeyHint(plainUpstream), *weight); err != nil {
		log.Fatalf("insert upstream key: %v", err)
	}

	if _, err := pg.Exec(ctx, `DELETE FROM routing_rules WHERE model_alias = $1 AND provider_id = $2 AND tenant_id IS NULL`, *alias, providerID); err != nil {
		log.Fatalf("clear route: %v", err)
	}
	if _, err := pg.Exec(ctx, `
		INSERT INTO routing_rules(id, tenant_id, model_alias, provider_id, model, priority, enabled)
		VALUES($1, NULL, $2, $3, $4, 100, true)
	`, util.NewID(), *alias, providerID, *model); err != nil {
		log.Fatalf("insert route: %v", err)
	}

	fmt.Println("CUSTOMER_KEY_PLAINTEXT=" + customerKey)
	fmt.Println("提示：该明文只显示这一次，请立即保存；数据库中只存哈希。")
	fmt.Printf("tenant_id=%s provider_id=%s alias=%s\n", *tenantID, providerID, *alias)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(b)
}
