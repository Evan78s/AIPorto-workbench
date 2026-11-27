package payment

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aiporto/internal/config"
)

const wechatNativeURL = "https://api.mch.weixin.qq.com/v3/pay/transactions/native"

type wechatProvider struct {
	appID        string
	mchID        string
	certSerialNo string
	privateKey   *rsa.PrivateKey
	platformKey  *rsa.PublicKey
	apiV3Key     []byte
	notifyURL    string
	client       *http.Client
}

func newWechat(cfg config.Config) *wechatProvider {
	p := &wechatProvider{
		appID:        cfg.WechatAppID,
		mchID:        cfg.WechatMchID,
		certSerialNo: cfg.WechatCertSerialNo,
		apiV3Key:     cfg.WechatAPIV3Key,
		notifyURL:    cfg.WechatNotifyURL,
		client:       &http.Client{Timeout: 15 * time.Second},
	}
	if cfg.PublicBaseURL != "" && p.notifyURL == "" {
		p.notifyURL = strings.TrimRight(cfg.PublicBaseURL, "/") + "/api/user/pay/wechat/notify"
	}
	p.privateKey, _ = loadPrivateKey(cfg.WechatPrivateKey)
	p.platformKey, _ = loadPublicKey(cfg.WechatPlatformPubKey)
	return p
}

func (p *wechatProvider) Channel() Channel { return ChannelWechat }
func (p *wechatProvider) Enabled() bool {
	return p.appID != "" && p.mchID != "" && p.privateKey != nil && p.platformKey != nil && len(p.apiV3Key) == 32
}

func (p *wechatProvider) CreateOrder(orderNo, amountCNY, subject string) (CreateResult, error) {
	if !p.Enabled() {
		return CreateResult{}, fmt.Errorf("wechat pay not configured")
	}
	// 金额转分：用字符串解析避免浮点误差。
	fen, err := yuanToFen(amountCNY)
	if err != nil {
		return CreateResult{}, err
	}
	body := map[string]any{
		"appid":        p.appID,
		"mchid":        p.mchID,
		"description":  subject,
		"out_trade_no": orderNo,
		"notify_url":   p.notifyURL,
		"amount":       map[string]any{"total": fen, "currency": "CNY"},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return CreateResult{}, err
	}
	auth, err := p.authorization("POST", wechatNativeURL, string(bodyBytes))
	if err != nil {
		return CreateResult{}, err
	}
	req, err := http.NewRequest(http.MethodPost, wechatNativeURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return CreateResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", auth)
	resp, err := p.client.Do(req)
	if err != nil {
		return CreateResult{}, err
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return CreateResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CreateResult{}, fmt.Errorf("wechat create order http %d: %s", resp.StatusCode, string(respBytes))
	}
	var out struct {
		CodeURL string `json:"code_url"`
	}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return CreateResult{}, err
	}
	if out.CodeURL == "" {
		return CreateResult{}, fmt.Errorf("wechat returned empty code_url: %s", string(respBytes))
	}
	return CreateResult{
		Channel:    ChannelWechat,
		QRCode:     out.CodeURL,
		ExternalID: orderNo,
		Amount:     amountCNY,
	}, nil
}

// VerifyNotify 校验微信 v3 回调：先验签，再用 APIv3Key 解密 resource。
// header 传入 Wechatpay-Signature / Wechatpay-Timestamp / Wechatpay-Nonce / Wechatpay-Serial，
// body 传入解密前的完整 JSON（我们把它作为一个键 "raw" 传入）。
func (p *wechatProvider) VerifyNotify(header, body map[string]string) (NotifyResult, error) {
	if !p.Enabled() {
		return NotifyResult{}, fmt.Errorf("wechat pay not configured")
	}
	signature := header["Wechatpay-Signature"]
	timestamp := header["Wechatpay-Timestamp"]
	nonce := header["Wechatpay-Nonce"]
	serial := header["Wechatpay-Serial"]
	rawJSON := body["raw"]
	if signature == "" || timestamp == "" || nonce == "" || rawJSON == "" {
		return NotifyResult{}, fmt.Errorf("missing wechat callback headers")
	}
	if serial != "" && serial != p.certSerialNo && serial != "platform" {
		// 平台证书序列号与商户证书不同，这里不强制相等，仅记录；验签本身保证来源。
		_ = serial
	}
	// 防重放：时间戳与当前时间相差不能超过 5 分钟。
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(ts, 0)) > 5*time.Minute || time.Until(time.Unix(ts, 0)) > 5*time.Minute {
		return NotifyResult{}, fmt.Errorf("wechat callback timestamp out of window")
	}
	message := timestamp + "\n" + nonce + "\n" + rawJSON + "\n"
	h := sha256.Sum256([]byte(message))
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return NotifyResult{}, fmt.Errorf("invalid wechat signature encoding: %w", err)
	}
	if err := rsa.VerifyPKCS1v15(p.platformKey, crypto.SHA256, h[:], sig); err != nil {
		return NotifyResult{}, fmt.Errorf("wechat callback signature mismatch: %w", err)
	}

	var envelope struct {
		Resource struct {
			Algorithm      string `json:"algorithm"`
			Ciphertext     string `json:"ciphertext"`
			AssociatedData string `json:"associated_data"`
			Nonce          string `json:"nonce"`
		} `json:"resource"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &envelope); err != nil {
		return NotifyResult{}, err
	}
	plain, err := decryptAESGCM(p.apiV3Key, envelope.Resource.Nonce,
		envelope.Resource.AssociatedData, envelope.Resource.Ciphertext)
	if err != nil {
		return NotifyResult{}, err
	}
	var detail struct {
		OutTradeNo string `json:"out_trade_no"`
		TradeState string `json:"trade_state"`
		Amount     struct {
			Total int `json:"total"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(plain, &detail); err != nil {
		return NotifyResult{}, err
	}
	if detail.TradeState != "SUCCESS" {
		return NotifyResult{}, fmt.Errorf("wechat trade_state %s is not paid", detail.TradeState)
	}
	return NotifyResult{
		Channel:    ChannelWechat,
		ExternalID: detail.OutTradeNo,
		TradeNo:    detail.OutTradeNo,
		Amount:     strconv.Itoa(detail.Amount.Total),
		TradeState: detail.TradeState,
	}, nil
}

// authorization 生成微信 v3 Authorization 头。
func (p *wechatProvider) authorization(method, urlPath, body string) (string, error) {
	nonce := strings.ToUpper(hex.EncodeToString(randomBytes(16)))
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	message := method + "\n" + urlPath + "\n" + timestamp + "\n" + nonce + "\n" + body + "\n"
	h := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.privateKey, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",signature="%s",timestamp="%s",serial_no="%s"`,
		p.mchID, nonce, base64.StdEncoding.EncodeToString(sig), timestamp, p.certSerialNo,
	), nil
}

func decryptAESGCM(key []byte, nonceStr, associatedData, ciphertext string) ([]byte, error) {
	nonce := []byte(nonceStr)
	ct, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, []byte(associatedData))
}

func yuanToFen(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}
	parts := strings.SplitN(s, ".", 2)
	yuan, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	fen := yuan * 100
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 1 {
			frac += "0"
		}
		if len(frac) > 2 {
			return 0, fmt.Errorf("amount has more than 2 decimals: %q", s)
		}
		d, err := strconv.Atoi(frac)
		if err != nil {
			return 0, err
		}
		fen += d
	}
	return fen, nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
