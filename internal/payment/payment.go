// Package payment 封装支付宝电脑网站支付、微信支付 v3 Native 扫码、对公转账三种渠道。
// 对外只暴露统一接口：CreateOrder 返回给前端的支付参数，VerifyNotify 校验回调并返回流水号。
// 签名全部用 Go 标准库 crypto/rsa 实现，不引入第三方 SDK，避免供应链与升级风险。
package payment

import (
	"fmt"
	"strings"

	"aiporto/internal/config"
)

type Channel string

const (
	ChannelAlipay Channel = "alipay"
	ChannelWechat Channel = "wechat"
	ChannelManual Channel = "manual"
)

// CreateResult 是下单结果，前端据此渲染付款入口。
type CreateResult struct {
	Channel     Channel
	PaymentURL  string // 支付宝：完整跳转 URL；对公：空
	QRCode      string // 微信：code_url，前端转成二维码；对公：空
	ExternalID  string // 渠道订单号（支付宝 out_trade_no / 微信 out_trade_no）
	BankInfo    string // 对公转账：收款信息文案
	Amount      string
}

// NotifyResult 是回调验签通过后的结果。
type NotifyResult struct {
	Channel    Channel
	ExternalID string // 渠道订单号，用于匹配订单
	TradeNo    string // 渠道流水号
	Amount     string // 渠道回传金额（元），调用方必须与订单金额比对
	TradeState string // SUCCESS / 已支付 等，调用方判断是否成功
}

// Provider 是所有支付渠道的统一接口。
type Provider interface {
	Channel() Channel
	Enabled() bool
	CreateOrder(orderNo, amountCNY, subject string) (CreateResult, error)
	VerifyNotify(header, body map[string]string) (NotifyResult, error)
}

// New 按配置构造渠道列表。未启用的渠道 Enabled() 返回 false，用户端据此隐藏。
func New(cfg config.Config) map[Channel]Provider {
	providers := map[Channel]Provider{
		ChannelManual: &manualProvider{
			bankInfo: strings.TrimSpace(cfg.PublicBaseURL),
		},
	}
	if cfg.AlipayEnabled {
		providers[ChannelAlipay] = newAlipay(cfg)
	}
	if cfg.WechatEnabled {
		providers[ChannelWechat] = newWechat(cfg)
	}
	return providers
}

// AvailableChannels 返回当前可用的渠道（对公永远可用）。
func AvailableChannels(ps map[Channel]Provider) []Channel {
	out := []Channel{}
	for _, c := range []Channel{ChannelAlipay, ChannelWechat, ChannelManual} {
		if p, ok := ps[c]; ok && p.Enabled() {
			out = append(out, c)
		}
	}
	return out
}

func Get(ps map[Channel]Provider, c Channel) (Provider, error) {
	p, ok := ps[c]
	if !ok || !p.Enabled() {
		return nil, fmt.Errorf("payment channel %s is not available", c)
	}
	return p, nil
}
