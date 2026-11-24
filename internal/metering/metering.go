package metering

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Record struct {
	RequestID        string
	TenantID         string
	KeyID            string
	ProviderID       string
	UpstreamKeyID    string
	ModelAlias       string
	ModelResolved    string
	Endpoint         string
	Stream           bool
	PromptTokens     int
	CompletionTokens int
	StatusCode       int
	LatencyMS        int
	TTFTMS           int
	CreatedAt        time.Time
}

type Reporter struct {
	pg       *pgxpool.Pool
	ch       chan Record
	batch    int
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
	dropped  atomic.Uint64
}

func New(pg *pgxpool.Pool, batch int, interval time.Duration) *Reporter {
	return &Reporter{
		pg:       pg,
		ch:       make(chan Record, 8192),
		batch:    batch,
		interval: interval,
		stop:     make(chan struct{}),
	}
}

func (r *Reporter) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.loop(ctx)
}

// Enqueue 完全非阻塞；队列满时丢弃并计数，绝不阻塞请求链路。
func (r *Reporter) Enqueue(rec Record) {
	select {
	case r.ch <- rec:
	default:
		r.dropped.Add(1)
	}
}

func (r *Reporter) Dropped() uint64 { return r.dropped.Load() }

func (r *Reporter) Stop() {
	close(r.stop)
	r.wg.Wait()
}

func (r *Reporter) loop(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	buf := make([]Record, 0, r.batch)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		r.flush(context.Background(), buf)
		buf = buf[:0]
	}

	for {
		select {
		case rec := <-r.ch:
			buf = append(buf, rec)
			if len(buf) >= r.batch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		case <-r.stop:
			for {
				select {
				case rec := <-r.ch:
					buf = append(buf, rec)
					if len(buf) >= r.batch {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (r *Reporter) flush(ctx context.Context, rows []Record) {
	cols := []string{
		"request_id", "tenant_id", "api_key_id", "provider_id", "upstream_key_id",
		"model_alias", "model_resolved", "endpoint", "stream",
		"prompt_tokens", "completion_tokens", "status_code",
		"latency_ms", "ttft_ms", "created_at",
	}
	_, err := r.pg.CopyFrom(ctx, pgx.Identifier{"usage_logs"}, cols, pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
		x := rows[i]
		return []any{
			x.RequestID, x.TenantID, x.KeyID, x.ProviderID, x.UpstreamKeyID,
			x.ModelAlias, x.ModelResolved, x.Endpoint, x.Stream,
			x.PromptTokens, x.CompletionTokens, x.StatusCode,
			x.LatencyMS, x.TTFTMS, x.CreatedAt,
		}, nil
	}))
	if err != nil {
		log.Printf("metering flush failed (%d rows): %v", len(rows), err)
	}
}
