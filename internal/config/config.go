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

	// B 方案：用户端。
	// UserPortalURL 是用户站点的公网地址（如 https://user.aiporto.net），用于生成回跳链接。
	// PublicBaseURL 是网关对外的公网地址（如 https://gw.aiporto.net），用于拼支付回调地址。
	UserPortalURL string
	PublicBaseURL string

	// SMTP：验证码邮件。TLSMode 取 none / starttls / ssl。
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string
	SMTPTLS  string

	// 支付宝：私钥与支付宝公钥都是 base64(PEM) 形式，避免 .env 多行值。
	AlipayEnabled      bool
	AlipayAppID        string
	AlipayPrivateKey   []byte
	AlipayPublicKey    []byte
	AlipayNotifyURL    string
	AlipayReturnURL    string

	// 微信支付 v3：商户私钥为 base64(PEM)，APIv3Key 为 32 字节 base64。
	WechatEnabled        bool
	WechatAppID          string
	WechatMchID          string
	WechatAPIV3Key       []byte
	WechatCertSerialNo   string
	WechatPrivateKey     []byte
	WechatPlatformPubKey []byte // 微信支付平台公钥（验回调签名用），base64(PEM)
	WechatNotifyURL      string
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

		UserPortalURL: strings.TrimRight(os.Getenv("USER_PORTAL_URL"), "/"),
		PublicBaseURL: strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),

		SMTPHost: os.Getenv("SMTP_HOST"),
		SMTPPort: envInt("SMTP_PORT", 587),
		SMTPUser: os.Getenv("SMTP_USER"),
		SMTPPass: os.Getenv("SMTP_PASS"),
		SMTPFrom: os.Getenv("SMTP_FROM"),
		SMTPTLS:  envDefault("SMTP_TLS", "starttls"),

		AlipayEnabled:   envBool("ALIPAY_ENABLED", false),
		AlipayAppID:     os.Getenv("ALIPAY_APP_ID"),
		AlipayNotifyURL: os.Getenv("ALIPAY_NOTIFY_URL"),
		AlipayReturnURL: os.Getenv("ALIPAY_RETURN_URL"),

		WechatEnabled:        envBool("WECHAT_ENABLED", false),
		WechatAppID:          os.Getenv("WECHAT_APP_ID"),
		WechatMchID:          os.Getenv("WECHAT_MCH_ID"),
		WechatCertSerialNo:   os.Getenv("WECHAT_CERT_SERIAL_NO"),
		WechatNotifyURL:      os.Getenv("WECHAT_NOTIFY_URL"),
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

	// 用户端：注册登录需要 SMTP；支付渠道按需启用，未启用时走对公转账后台确认。
	if c.SMTPHost != "" {
		if c.SMTPFrom == "" {
			return c, fmt.Errorf("SMTP_FROM is required when SMTP_HOST is set")
		}
		switch c.SMTPTLS {
		case "none", "starttls", "ssl":
		default:
			return c, fmt.Errorf("SMTP_TLS must be one of none/starttls/ssl")
		}
	}
	if c.AlipayEnabled {
		if c.AlipayAppID == "" {
			return c, fmt.Errorf("ALIPAY_APP_ID is required when ALIPAY_ENABLED=true")
		}
		if c.AlipayPrivateKey, err = decodeKeyMaterial("ALIPAY_PRIVATE_KEY_B64"); err != nil {
			return c, err
		}
		if c.AlipayPublicKey, err = decodeKeyMaterial("ALIPAY_PUBLIC_KEY_B64"); err != nil {
			return c, err
		}
	}
	if c.WechatEnabled {
		if c.WechatAppID == "" || c.WechatMchID == "" || c.WechatCertSerialNo == "" {
			return c, fmt.Errorf("WECHAT_APP_ID, WECHAT_MCH_ID, WECHAT_CERT_SERIAL_NO are required when WECHAT_ENABLED=true")
		}
		if c.WechatAPIV3Key, err = decodeKey("WECHAT_API_V3_KEY_B64"); err != nil {
			return c, err
		}
		if c.WechatPrivateKey, err = decodeKeyMaterial("WECHAT_PRIVATE_KEY_B64"); err != nil {
			return c, err
		}
		if c.WechatPlatformPubKey, err = decodeKeyMaterial("WECHAT_PLATFORM_PUBLIC_KEY_B64"); err != nil {
			return c, err
		}
	}
	return c, nil
}

// decodeKeyMaterial 解码任意长度的 base64 密钥材料（PEM 私钥/公钥），不打印内容。
func decodeKeyMaterial(name string) ([]byte, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64: %w", name, err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	return b, nil
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

func envBool(name string, fallback bool) bool {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
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
