package payment

// manualProvider 是对公转账兜底：用户下单后线下打款，运营后台点「确认收款」开通。
// 它不产生可自动核验的回调，VerifyNotify 永远返回错误。
type manualProvider struct {
	bankInfo string
}

func (m *manualProvider) Channel() Channel { return ChannelManual }
func (m *manualProvider) Enabled() bool    { return true }

func (m *manualProvider) CreateOrder(orderNo, amountCNY, subject string) (CreateResult, error) {
	info := "请通过对公转账付款，并在备注中填写订单号 " + orderNo + "。\n"
	if m.bankInfo != "" {
		info += "收款信息请联系运营方获取，或访问 " + m.bankInfo + " 查看。\n"
	}
	info += "付款后等待运营方在后台确认，到账后自动开通。"
	return CreateResult{
		Channel:    ChannelManual,
		ExternalID: orderNo,
		BankInfo:   info,
		Amount:     amountCNY,
	}, nil
}

func (m *manualProvider) VerifyNotify(header, body map[string]string) (NotifyResult, error) {
	return NotifyResult{}, errNoAutoVerify
}
