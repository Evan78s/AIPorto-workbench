import { useEffect, useState } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import { userAPI } from '../api'
import { QRCodeSVG } from 'qrcode.react'

const STATUS = { pending: '待支付', paid: '已支付', cancelled: '已取消', failed: '失败', refunded: '已退款' }

export default function OrderDetail() {
  const { no } = useParams()
  const location = useLocation()
  const fresh = location.state?.fresh
  const [order, setOrder] = useState(fresh || null)
  const [error, setError] = useState('')
  const [credentials, setCredentials] = useState(null)

  function refresh() {
    userAPI.order(no).then(setOrder).catch((e) => setError(e.message))
  }
  useEffect(() => { if (!fresh) refresh() }, [no])

  async function take() {
    setError('')
    try {
      const res = await userAPI.takeCredentials(no)
      setCredentials(res)
      refresh()
    } catch (err) { setError(err.message) }
  }

  async function cancel() {
    if (!window.confirm('确认取消这笔订单？')) return
    try {
      await userAPI.cancelOrder(no)
      refresh()
    } catch (err) { setError(err.message) }
  }

  if (!order) return <div className="card">{error || '加载中…'}</div>

  // pay_payload 是后端存的下单响应 JSON，刷新页面后仍可重看二维码/转账信息。
  let pay = {}
  if (!fresh && order.pay_payload) {
    try { pay = JSON.parse(order.pay_payload) } catch { pay = {} }
  } else if (fresh) {
    pay = fresh
  }
  const qrCode = order.qr_code || pay.qr_code
  const bankInfo = order.bank_info || pay.bank_info

  return (
    <div>
      <h1>订单详情</h1>
      <p><Link to="/orders">← 返回订单列表</Link></p>
      {error && <p className="error" role="alert">{error}</p>}

      <div className="card">
        <div className="kv"><span>订单号</span><code>{order.order_no}</code></div>
        <div className="kv"><span>套餐</span><strong>{order.plan}</strong></div>
        <div className="kv"><span>金额</span><strong>¥{Number(order.amount).toFixed(2)} {order.currency}</strong></div>
        <div className="kv"><span>状态</span><span className={`badge ${order.status === 'paid' ? 'ok' : ''}`}>{STATUS[order.status] || order.status}</span></div>
        <div className="kv"><span>支付方式</span><span>{order.channel}</span></div>
        <div className="kv"><span>下单时间</span><span>{order.created_at}</span></div>
        {order.paid_at && <div className="kv"><span>支付时间</span><span>{order.paid_at}</span></div>}
      </div>

      {order.status === 'pending' && order.channel === 'wechat' && qrCode && (
        <div className="card">
          <h2>微信扫码支付</h2>
          <div className="qr"><QRCodeSVG value={qrCode} size={220} /></div>
          <p className="muted">请用微信扫描二维码完成支付。支付完成后点击下方「刷新状态」。</p>
          <button className="secondary" onClick={refresh}>刷新状态</button>
        </div>
      )}

      {order.status === 'pending' && order.channel === 'manual' && (
        <div className="card notice">
          <h2>对公转账</h2>
          <pre>{bankInfo || '请联系运营方获取收款账户，并在转账备注中填写订单号。'}</pre>
          <p className="muted">到账后由运营方在后台确认，订单状态会自动更新。</p>
          <button className="secondary" onClick={refresh}>刷新状态</button>
        </div>
      )}

      {order.status === 'pending' && order.channel === 'manual' && (
        <div className="card"><button className="secondary danger" onClick={cancel}>取消订单</button></div>
      )}

      {order.status === 'paid' && (
        <div className="card">
          <h2>你的 API Key 与控制台令牌</h2>
          {credentials && (
            <div className="credentials">
              <div className="kv"><span>API Key</span><code>{credentials.api_key_plaintext}</code></div>
              <div className="kv"><span>控制台令牌</span><code>{credentials.console_token_plaintext}</code></div>
              <p className="muted">{credentials.note}</p>
            </div>
          )}
          {!credentials && order.credentials_status === 'available' && (
            <>
              <p className="muted">点击下方按钮领取。明文只显示这一次，请立即保存到安全的地方。</p>
              <button onClick={take}>领取密钥</button>
            </>
          )}
          {!credentials && order.credentials_status === 'taken' && (
            <p className="muted">密钥已被领取过。若遗失，请联系运营方补发；补发后会生成新的 Key，旧的仍然有效。</p>
          )}
          {!credentials && order.credentials_status === 'missing' && (
            <p className="error">凭据尚未生成，请联系运营方在后台为此订单补发密钥。</p>
          )}
          {!credentials && (order.credentials_status === undefined || order.credentials_status === '') && (
            <>
              <p className="muted">点击下方按钮领取。明文只显示这一次，请立即保存到安全的地方。</p>
              <button onClick={take}>领取密钥</button>
            </>
          )}
        </div>
      )}
    </div>
  )
}
