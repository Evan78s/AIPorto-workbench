package payment

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"aiporto/internal/config"
)

const alipayGateway = "https://openapi.alipay.com/gateway.do"

type alipayProvider struct {
	appID      string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	notifyURL  string
	returnURL  string
}

func newAlipay(cfg config.Config) *alipayProvider {
	p := &alipayProvider{
		appID:     cfg.AlipayAppID,
		notifyURL: cfg.AlipayNotifyURL,
		returnURL: cfg.AlipayReturnURL,
	}
	if cfg.PublicBaseURL != "" {
		if p.notifyURL == "" {
			p.notifyURL = strings.TrimRight(cfg.PublicBaseURL, "/") + "/api/user/pay/alipay/notify"
		}
		if p.returnURL == "" && cfg.UserPortalURL != "" {
			p.returnURL = strings.TrimRight(cfg.UserPortalURL, "/") + "/orders"
		}
	}
	p.privateKey, _ = loadPrivateKey(cfg.AlipayPrivateKey)
	p.publicKey, _ = loadPublicKey(cfg.AlipayPublicKey)
	return p
}

func (p *alipayProvider) Channel() Channel { return ChannelAlipay }
func (p *alipayProvider) Enabled() bool {
	return p.appID != "" && p.privateKey != nil && p.publicKey != nil
}

func (p *alipayProvider) CreateOrder(orderNo, amountCNY, subject string) (CreateResult, error) {
	if !p.Enabled() {
		return CreateResult{}, fmt.Errorf("alipay not configured")
	}
	bizContent, err := json.Marshal(map[string]string{
		"out_trade_no": orderNo,
		"total_amount": amountCNY,
		"subject":      subject,
		"product_code": "FAST_INSTANT_TRADE_PAY",
	})
	if err != nil {
		return CreateResult{}, err
	}
	params := map[string]string{
		"app_id":     p.appID,
		"method":     "alipay.trade.page.pay",
		"format":     "JSON",
		"charset":    "utf-8",
		"sign_type":  "RSA2",
		"timestamp":  time.Now().Format("2006-01-02 15:04:05"),
		"version":    "1.0",
		"notify_url": p.notifyURL,
		"return_url": p.returnURL,
		"biz_content": string(bizContent),
	}
	sign, err := p.sign(params)
	if err != nil {
		return CreateResult{}, err
	}
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	q.Set("sign", sign)
	return CreateResult{
		Channel:    ChannelAlipay,
		PaymentURL: alipayGateway + "?" + q.Encode(),
		ExternalID: orderNo,
		Amount:     amountCNY,
	}, nil
}

// VerifyNotify 校验支付宝异步通知。body 是通知表单字段（含 sign / sign_type）。
func (p *alipayProvider) VerifyNotify(header, body map[string]string) (NotifyResult, error) {
	if !p.Enabled() {
		return NotifyResult{}, fmt.Errorf("alipay not configured")
	}
	sign := body["sign"]
	if sign == "" {
		return NotifyResult{}, fmt.Errorf("missing sign")
	}
	params := map[string]string{}
	for k, v := range body {
		if k == "sign" || k == "sign_type" {
			continue
		}
		params[k] = v
	}
	if err := p.verify(params, sign); err != nil {
		return NotifyResult{}, err
	}
	tradeStatus := body["trade_status"]
	if tradeStatus != "TRADE_SUCCESS" && tradeStatus != "TRADE_FINISHED" {
		return NotifyResult{}, fmt.Errorf("alipay trade status %s is not paid", tradeStatus)
	}
	return NotifyResult{
		Channel:    ChannelAlipay,
		ExternalID: body["out_trade_no"],
		TradeNo:    body["trade_no"],
		Amount:     body["total_amount"],
		TradeState: tradeStatus,
	}, nil
}

// signAlgorithm 排除 sign 与 sign_type，按 key 字典序拼接 k=v&...。
func signAlgorithm(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" || k == "sign_type" || params[k] == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	return strings.Join(parts, "&")
}

func (p *alipayProvider) sign(params map[string]string) (string, error) {
	text := signAlgorithm(params)
	h := sha256.Sum256([]byte(text))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.privateKey, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func (p *alipayProvider) verify(params map[string]string, sign string) error {
	text := signAlgorithm(params)
	h := sha256.Sum256([]byte(text))
	sig, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return fmt.Errorf("invalid sign encoding: %w", err)
	}
	return rsa.VerifyPKCS1v15(p.publicKey, crypto.SHA256, h[:], sig)
}
