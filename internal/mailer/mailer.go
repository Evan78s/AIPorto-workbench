// Package mailer 用 Go 标准库 net/smtp 发送验证码邮件，无第三方依赖。
// 发信失败返回错误，调用方决定是否重试；验证码本身只以哈希形式落库。
package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

type Config struct {
	Host string
	Port int
	User string
	Pass string
	From string
	TLS  string // none / starttls / ssl
}

// Mailer 为 nil 时表示未配置 SMTP，注册功能应拒绝并提示运营方。
func New(c Config) *Mailer {
	if c.Host == "" {
		return nil
	}
	if c.Port == 0 {
		c.Port = 587
	}
	if c.TLS == "" {
		c.TLS = "starttls"
	}
	return &Mailer{cfg: c}
}

type Mailer struct {
	cfg Config
}

// SendVerifyCode 发送注册/重置密码验证码。code 是 6 位数字，ttl 用于文案提示。
// 重试一次：很多 SMTP 服务商首连接偶发失败，重试可显著提升到达率。
func (m *Mailer) SendVerifyCode(to, code string, ttl time.Duration) error {
	subject := "Aiporto 智港 · 邮箱验证码"
	minutes := int(ttl.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	body := fmt.Sprintf(`<div style="font-family:-apple-system,Segoe UI,PingFang SC,sans-serif;max-width:520px;margin:auto;padding:24px">
<h2 style="color:#111827">Aiporto 智港</h2>
<p style="color:#6b7280;letter-spacing:2px">万模入港，统一调度</p>
<p>你好，</p>
<p>你正在完成 Aiporto 智港的邮箱验证，验证码是：</p>
<div style="margin:18px 0;padding:14px 20px;background:#f3f4f6;border-radius:8px;font-size:26px;letter-spacing:6px;font-family:ui-monospace,monospace">%s</div>
<p style="color:#374151">验证码 <strong>%d 分钟内有效</strong>，请勿转发给他人。若非你本人操作，请忽略本邮件。</p>
<hr style="border:none;border-top:1px solid #e5e7eb;margin:22px 0">
<p style="color:#9ca3af;font-size:12px">本邮件由系统自动发送，请勿直接回复。</p>
</div>`, code, minutes)

	raw := "From: " + m.cfg.From + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mimeEncode(subject) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" + body

	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if err = m.send(to, raw); err == nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("smtp send failed after retry: %w", err)
}

func (m *Mailer) send(to, raw string) error {
	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)

	switch m.cfg.TLS {
	case "ssl":
		tlsCfg := &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, m.cfg.Host)
		if err != nil {
			return err
		}
		defer client.Close()
		return deliver(client, auth, m.cfg.From, to, raw)

	case "starttls":
		client, err := smtp.Dial(addr)
		if err != nil {
			return err
		}
		defer client.Close()
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
		return deliver(client, auth, m.cfg.From, to, raw)

	default: // none：本地调试或内网 relay
		client, err := smtp.Dial(addr)
		if err != nil {
			return err
		}
		defer client.Close()
		return deliver(client, auth, m.cfg.From, to, raw)
	}
}

func deliver(client *smtp.Client, auth smtp.Auth, from, to, raw string) error {
	if ok, _ := client.Extension("AUTH"); ok && auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(extractAddr(from)); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(raw)); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// extractAddr 从 "Aiporto 智港 <no-reply@example.com>" 取出纯邮箱地址。
func extractAddr(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 && strings.HasSuffix(from, ">") {
		return strings.TrimSpace(from[i+1 : len(from)-1])
	}
	return strings.TrimSpace(from)
}

// mimeEncode 对非 ASCII 主题做 UTF-8 Base64 编码，避免主题乱码。
func mimeEncode(s string) string {
	for _, r := range s {
		if r > 127 {
			return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
		}
	}
	return s
}
