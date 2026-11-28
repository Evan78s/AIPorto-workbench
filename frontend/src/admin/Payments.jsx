import { adminAPI } from '../api'
import { useFetch } from '../components/useFetch'

export default function Payments() {
  const status = useFetch(() => adminAPI.get('/admin/payment/status'), [])

  return (
    <>
      <h1>支付配置</h1>
      <p className="muted">三种收款方式：微信支付、支付宝、管理员手动开通/充值。密钥属于最高敏感数据，只能写在服务器 .env，不能放在网页表单或数据库里，本页面只显示开关状态，不显示任何私钥。</p>

      <div className="card table-wrap">
        <h2>渠道状态</h2>
        <table>
          <thead><tr><th>渠道</th><th>状态</th><th>商户号 / AppID</th><th>回调地址</th></tr></thead>
          <tbody>
            <tr>
              <td>支付宝（电脑网站支付）</td>
              <td><span className={'badge ' + (status.data?.alipay_enabled ? 'active' : '')}>{status.data?.alipay_enabled ? '已启用' : '未启用'}</span></td>
              <td>{status.data?.alipay_app_id || '-'}</td>
              <td><code>{status.data?.alipay_notify || '-'}</code></td>
            </tr>
            <tr>
              <td>微信支付（v3 Native）</td>
              <td><span className={'badge ' + (status.data?.wechat_enabled ? 'active' : '')}>{status.data?.wechat_enabled ? '已启用' : '未启用'}</span></td>
              <td>{status.data?.wechat_mch_id || '-'}</td>
              <td><code>{status.data?.wechat_notify || '-'}</code></td>
            </tr>
            <tr>
              <td>管理员手动开通 / 对公转账</td>
              <td><span className="badge active">始终可用</span></td>
              <td>-</td>
              <td>无需回调，后台「用户订单」页确认到账</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div className="card">
        <h2>开启支付宝 / 微信支付</h2>
        <ol className="muted">
          <li>按 <code>PAYMENT-SETUP.md</code> 在支付宝开放平台 / 微信商户平台申请应用与证书，拿到私钥和公钥。</li>
          <li>在服务器执行 <code>cd /www/docker/aiporto &amp;&amp; nano .env</code>，填入对应的 <code>ALIPAY_*</code> / <code>WECHAT_*</code> 配置，并把 <code>*_ENABLED</code> 设为 <code>true</code>。</li>
          <li>必须重建容器才会读取新环境变量：<code>docker compose -f docker-compose.prod.yaml up -d --force-recreate gateway user-ui</code>（<code>restart</code> 不 reload .env）。</li>
          <li>回到本页刷新，状态变为「已启用」；用户端购买页自动出现对应支付选项。</li>
        </ol>
        <p className="muted">回调地址必须与商户平台登记一致：<code>{status.data?.public_base_url || 'https://gw.aiporto.net'}/api/user/pay/alipay/notify</code> 与 <code>.../wechat/notify</code>。</p>
      </div>

      <div className="card">
        <h2>管理员手动开通与充值</h2>
        <ul className="muted">
          <li><strong>开通新订单：</strong>用户用「对公转账」下单后，到「用户订单」页点「确认到账并开通」，系统自动签发租户 + API Key。</li>
          <li><strong>给已有客户充值：</strong>「账本流水」页选租户、填金额，写入 billing_ledger，余额立即增加。</li>
          <li><strong>调整额度：</strong>「租户与 key」页改 RPM / 并发 / 日额度；套餐参数在「套餐与价格」页改。</li>
        </ul>
      </div>
    </>
  )
}
