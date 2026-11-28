package routing

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"aiporto/internal/util"
)

var ErrNoRoute = errors.New("no route for model")

type Provider struct {
	ID       string
	Code     string
	BaseURL  string
	Protocol string
}

type Candidate struct {
	Provider Provider
	Model    string
	KeyID    string
	PlainKey []byte
	KeyHint  string
}

type upstreamKey struct {
	id     string
	enc    []byte
	hint   string
	weight int
}

type route struct {
	provider Provider
	model    string
	keys     []upstreamKey
}

type cacheEntry struct {
	at     time.Time
	routes map[string][]route
}

type Router struct {
	pg        *pgxpool.Pool
	masterKey []byte
	ttl       time.Duration

	mu       sync.Mutex
	cache    map[string]cacheEntry
	counters sync.Map // string -> *atomic.Uint64
}

func New(pg *pgxpool.Pool, masterKey []byte, ttl time.Duration) *Router {
	return &Router{
		pg:        pg,
		masterKey: masterKey,
		ttl:       ttl,
		cache:     make(map[string]cacheEntry),
	}
}

// Resolve 返回按优先级与加权轮询排好序的候选列表；调用方最多使用两个候选（最多重试一次）。
func (r *Router) Resolve(ctx context.Context, tenantID, model string) ([]Candidate, error) {
	routes, err := r.routesFor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	selected := routes[model]
	if len(selected) == 0 {
		return nil, ErrNoRoute
	}

	out := make([]Candidate, 0, 4)
	for _, rt := range selected {
		if len(rt.keys) == 0 {
			continue
		}
		counterKey := tenantID + "|" + model + "|" + rt.provider.ID
		v, _ := r.counters.LoadOrStore(counterKey, &atomic.Uint64{})
		tick := v.(*atomic.Uint64).Add(1) - 1
		start := pickByWeight(rt.keys, tick)

		for i := 0; i < len(rt.keys); i++ {
			k := rt.keys[(start+i)%len(rt.keys)]
			plain, err := util.DecryptKey(r.masterKey, k.enc)
			if err != nil {
				continue
			}
			out = append(out, Candidate{
				Provider: rt.provider,
				Model:    rt.model,
				KeyID:    k.id,
				PlainKey: plain,
				KeyHint:  k.hint,
			})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no decryptable upstream key")
	}
	return out, nil
}

func pickByWeight(keys []upstreamKey, tick uint64) int {
	total := 0
	for _, k := range keys {
		total += k.weight
	}
	if total <= 0 {
		return 0
	}
	n := int(tick % uint64(total))
	for i, k := range keys {
		if n < k.weight {
			return i
		}
		n -= k.weight
	}
	return 0
}

func (r *Router) routesFor(ctx context.Context, tenantID string) (map[string][]route, error) {
	if e, ok := r.cache[tenantID]; ok && time.Since(e.at) < r.ttl {
		return e.routes, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[tenantID]; ok && time.Since(e.at) < r.ttl {
		return e.routes, nil
	}

	rows, err := r.pg.Query(ctx, `
		SELECT r.model_alias, r.priority,
		       p.id::text, p.code, p.base_url, p.protocol, r.model,
		       k.id::text, k.encrypted_key, k.key_hint, k.weight
		FROM routing_rules r
		JOIN upstream_providers p ON p.id = r.provider_id AND p.status = 'active'
		JOIN upstream_keys k ON k.provider_id = p.id AND k.status = 'active'
		WHERE r.enabled = true AND (r.tenant_id IS NULL OR r.tenant_id = $1)
		ORDER BY (r.tenant_id IS NULL) ASC, r.priority ASC, r.model_alias ASC, k.priority ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]route)
	for rows.Next() {
		var alias, providerID, code, baseURL, protocol, model, keyID, hint string
		var priority, weight int
		var enc []byte
		if err := rows.Scan(&alias, &priority, &providerID, &code, &baseURL, &protocol, &model, &keyID, &enc, &hint, &weight); err != nil {
			return nil, err
		}
		idx := -1
		for i := range result[alias] {
			if result[alias][i].provider.ID == providerID {
				idx = i
				break
			}
		}
		if idx < 0 {
			result[alias] = append(result[alias], route{
				provider: Provider{ID: providerID, Code: code, BaseURL: baseURL, Protocol: protocol},
				model:    model,
			})
			idx = len(result[alias]) - 1
		}
		result[alias][idx].keys = append(result[alias][idx].keys, upstreamKey{
			id:     keyID,
			enc:    append([]byte(nil), enc...),
			hint:   hint,
			weight: weight,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	r.cache[tenantID] = cacheEntry{at: time.Now(), routes: result}
	return result, nil
}
