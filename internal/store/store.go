package store

import (
	"context"
	"fmt"
	"aiporto/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Store struct {
	PG  *pgxpool.Pool
	RDB *redis.Client
}

func New(ctx context.Context, cfg config.Config) (*Store, error) {
	pg, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pg.Ping(ctx); err != nil {
		pg.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		pg.Close()
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Store{PG: pg, RDB: rdb}, nil
}

func (s *Store) Close() {
	if s.PG != nil {
		s.PG.Close()
	}
	if s.RDB != nil {
		_ = s.RDB.Close()
	}
}
