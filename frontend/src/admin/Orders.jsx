import { useState } from 'react'
import { adminAPI } from '../api'
import { useFetch } from '../components/useFetch'

const CHANNEL_LABELS = { alipay: '支付宝', wechat: '微信支付', manual: '对公转账' }
const STATUS_LABELS = { pending: '待支付', paid: '已支付', cancelled: '已取消', refunded: '已退款' }

export default function Orders() {
  const orders = useFetch(() => adminAPI.get('/admin/user-orders'), [])
  const [msg, setMsg] = useState('')
  const [okMsg, setOkMsg] = useState('')

  async function confirm(no) {
    if (!window.confirm(`确认已收到订单 ${no} 的付款？确认后会立即开通套餐并签发 API Key。`)) return
    setMsg(''); setOkMsg('')
    try {
      await adminAPI.post(`/admin/user-orders/${encodeURIComponent(no)}/confirm`)
      orders.reload()
      setOkMsg(`订单 ${no} 已确认开通`)
    } catch (err) { setMsg(err.message || String(err)) }
  }

  async function reissue(no) {
    if (!window.confirm(`为订单 ${no} 补发新的 API Key 和控制台令牌？\n\n用户此前的密钥若遗失，补发后会生成一对全新的凭据供用户重新领取。旧密钥仍然有效，可在「租户与 key」页停用它。`)) return
    setMsg(''); setOkMsg('')
    try {
      await adminAPI.post(`/admin/user-orders/${encodeURIComponent(no)}/reissue`)
      setOkMsg(`订单 ${no} 已补发密钥，通知用户重新登录订单详情页领取`)
    } catch (err) { setMsg(err.message || String(err)) }
  }

  return (
    <>
      <h1>用户订单</h1>
      <p className="muted">用户自助购买的订单都在这里。支付宝/微信支付成功后自动开通；对公转账订单需要你手工确认到账后才会开通。</p>
      {msg && <div className="error">{msg}</div>}
      {okMsg && <div className="badge active">{okMsg}</div>}

      <div className="card table-wrap">
        <h2>订单列表（最近 500 条）</h2>
        <table>
          <thead><tr><th>订单号</th><th>用户邮箱</th><th>套餐</th><th>金额</th><th>支付方式</th><th>状态</th><th>下单时间</th><th>支付时间</th><th>操作</th></tr></thead>
          <tbody>
            {(orders.data || []).map((o) => (
              <tr key={o.order_no}>
                <td><code>{o.order_no}</code></td>
                <td>{o.email}</td>
                <td>{o.plan}</td>
                <td>{o.amount} {o.currency}</td>
                <td>{CHANNEL_LABELS[o.channel] || o.channel}</td>
                <td><span className={'badge ' + (o.status === 'paid' ? 'active' : '')}>{STATUS_LABELS[o.status] || o.status}</span></td>
                <td>{o.created_at}</td>
                <td>{o.paid_at || '-'}</td>
                <td>
                  {o.channel === 'manual' && o.status === 'pending' && (
                    <button onClick={() => confirm(o.order_no)}>确认到账并开通</button>
                  )}
                  {o.status === 'paid' && (
                    <button className="secondary" onClick={() => reissue(o.order_no)}>补发密钥</button>
                  )}
                </td>
              </tr>
            ))}
            {orders.data && orders.data.length === 0 && <tr><td colSpan="9" className="empty">还没有用户订单</td></tr>}
          </tbody>
        </table>
      </div>

      <div className="card">
        <h2>怎么开通订单？</h2>
        <ul className="muted">
          <li><strong>支付宝 / 微信支付：</strong>支付回调验签通过后系统自动开通租户、订阅、API Key 和控制台令牌，无需人工操作。若订单一直「待支付」，检查回调地址与商户平台配置。</li>
          <li><strong>对公转账：</strong>用户下单后订单为「待支付」，你确认款项到账后点本页的「确认到账并开通」，系统立即开通并把明文凭据交给用户领取。</li>
          <li><strong>用户反馈"领取密钥"报错（已领取/未生成）：</strong>明文只显示一次，领取过一次后无法再次查看。若用户遗失，点该订单的「补发密钥」生成一对全新的 API Key 和控制台令牌，用户重新登录订单详情页即可领取。旧密钥仍有效，可在「租户与 key」页停用以避免混淆。</li>
          <li><strong>手工给已有客户充值/改额度：</strong>到「账本流水」页操作；「租户与 key」页可调整额度。</li>
        </ul>
      </div>
    </>
  )
}
