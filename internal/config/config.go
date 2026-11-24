package config

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr            string
	DatabaseURL           string
	RedisAddr             string
	RedisPassword         string
	RedisDB               int
	KeyPepper             []byte
	MasterKey             []byte
	UpstreamTimeout       time.Duration
	UpstreamHeaderTimeout time.Duration
	MeteringBatchSize     int
	MeteringFlush         time.Duration
	AuthCacheTTL          time.Duration
	RouteCacheTTL         time.Duration
	AdminUser             string
	AdminPassword         string
	AdminTOTPSecret       string
	AlertLowBalance       float64
}

func Load() (Config, error) {
	loadDotEnv("")
	c := Config{
		ListenAddr:            envDefault("GATEWAY_LISTEN_ADDR", ":8080"),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		RedisAddr:             envDefault("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:         os.Getenv("REDIS_PASSWORD"),
		RedisDB:               envInt("REDIS_DB", 0),
		UpstreamTimeout:       envDuration("UPSTREAM_TIMEOUT", 10*time.Minute),
		UpstreamHeaderTimeout: envDuration("UPSTREAM_HEADER_TIMEOUT", 15*time.Second),
		MeteringBatchSize:     envInt("METERING_BATCH_SIZE", 200),
		MeteringFlush:         envDuration("METERING_FLUSH_INTERVAL", time.Second),
		AuthCacheTTL:          envDuration("AUTH_CACHE_TTL", 60*time.Second),
		RouteCacheTTL:         envDuration("ROUTE_CACHE_TTL", 5*time.Second),
		AdminUser:             os.Getenv("ADMIN_USER"),
		AdminPassword:         os.Getenv("ADMIN_PASSWORD"),
		AdminTOTPSecret:       os.Getenv("ADMIN_TOTP_SECRET"),
		AlertLowBalance:       envFloat("ALERT_LOW_BALANCE_THRESHOLD", 100),
	}

	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.MeteringBatchSize < 1 {
		return c, fmt.Errorf("METERING_BATCH_SIZE must be positive")
	}

	var err error
	if c.KeyPepper, err = decodeKey("API_KEY_PEPPER_B64"); err != nil {
		return c, err
	}
	if c.MasterKey, err = decodeKey("UPSTREAM_MASTER_KEY_B64"); err != nil {
		return c, err
	}
	return c, nil
}

// loadDotEnv 读取当前目录下的 .env；已由 shell 导出的变量优先。
// 值不要加引号，也不要在行尾写注释。
func loadDotEnv(path string) {
	if path == "" {
		path = ".env"
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, strings.TrimSpace(parts[1]))
	}
}

func decodeKey(name string) ([]byte, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64: %w", name, err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("%s must decode to exactly 32 bytes, got %d", name, len(b))
	}
	return b, nil
}

func envDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func envInt(name string, fallback int) int {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(name string, fallback float64) float64 {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func envDuration(name string, fallback time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
