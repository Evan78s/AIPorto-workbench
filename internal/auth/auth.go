package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aiporto/internal/store"
	"aiporto/internal/util"
)

var ErrUnauthorized = errors.New("unauthorized")

const keyPrefix = "sk-wb-"

type Identity struct {
	KeyID            string `json:"key_id"`
	TenantID         string `json:"tenant_id"`
	RpmLimit         int    `json:"rpm"`
	ConcurrencyLimit int    `json:"conc"`
	DailyTokenLimit  int64  `json:"daily"`
}

type Authenticator struct {
	store  *store.Store
	pepper []byte
	ttl    time.Duration
}

func New(s *store.Store, pepper []byte, ttl time.Duration) *Authenticator {
	return &Authenticator{store: s, pepper: pepper, ttl: ttl}
}

// Authenticate 只接受 sk-wb- 前缀的 key，并且只比对 HMAC 摘要，不接触明文存储。
func (a *Authenticator) Authenticate(ctx context.Context, header string) (Identity, error) {
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if !strings.HasPrefix(token, keyPrefix) || len(token) <= len(keyPrefix)+3 {
		return Identity{}, ErrUnauthorized
	}

	hash := util.HashKey(a.pepper, token)
	cacheKey := "gw:auth:" + hash
	if raw, err := a.store.RDB.Get(ctx, cacheKey).Bytes(); err == nil {
		var cached Identity
		if json.Unmarshal(raw, &cached) == nil && cached.KeyID != "" {
			return cached, nil
		}
	}

	var id Identity
	var expiresAt *time.Time
	err := a.store.PG.QueryRow(ctx, `
		SELECT id::text, tenant_id::text, rpm_limit, concurrency_limit, daily_token_limit, expires_at
		FROM api_keys
		WHERE key_hash = $1 AND status = 'active'
	`, hash).Scan(&id.KeyID, &id.TenantID, &id.RpmLimit, &id.ConcurrencyLimit, &id.DailyTokenLimit, &expiresAt)
	if err != nil {
		// 包括 pgx.ErrNoRows 在内，一律按未授权处理，不泄露 key 是否存在。
		return Identity{}, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return Identity{}, ErrUnauthorized
	}

	if raw, err := json.Marshal(id); err == nil {
		_ = a.store.RDB.Set(ctx, cacheKey, raw, a.ttl).Err()
	}
	return id, nil
}
