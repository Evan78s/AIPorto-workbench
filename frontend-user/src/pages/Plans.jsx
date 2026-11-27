import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { userAPI } from '../api'

const CHANNEL_LABELS = { alipay: '支付宝', wechat: '微信支付', manual: '对公转账' }

export default function Plans() {
  const [plans, setPlans] = useState([])
  const [channels, setChannels] = useState([])
  const [selected, setSelected] = useState(null)
  const [channel, setChannel] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState(null)
  const navigate = useNavigate()

  useEffect(() => {
    userAPI.plans().then(setPlans).catch((e) => setError(e.message))
    userAPI.channels().then((d) => {
      setChannels(d.channels || [])
      if ((d.channels || []).length) setChannel(d.channels[0])
    }).catch((e) => setError(e.message))
  }, [])

  async function checkout() {
    if (!selected || !channel) return
    setError(''); setBusy(true)
    try {
      const res = await userAPI.createOrder(selected.id, channel)
      if (res.payment_url) {
        window.location.href = res.payment_url // 支付宝电脑网站支付跳转
        return
      }
      setResult(res) // 微信二维码 / 对公转账信息在本页展示
      navigate('/orders/' + encodeURIComponent(res.order_no), { state: { fresh: res } })
    } catch (err) { setError(err.message) } finally { setBusy(false) }
  }

  return (
    <div>
      <h1>选择套餐</h1>
      <p className="muted">购买后立即开通专属 API Key 与客户控制台，额度按月计算。</p>
      {error && <p className="error" role="alert">{error}</p>}
      {result && result.bank_info && <div className="card notice"><pre>{result.bank_info}</pre></div>}

      <div className="plan-grid">
        {(plans || []).map((p) => (
          <div key={p.id} className={`card plan ${selected?.id === p.id ? 'selected' : ''}`} onClick={() => setSelected(p)}>
            <h2>{p.name}</h2>
            <div className="price">¥{Number(p.price_per_month).toFixed(2)}<small>/月</small></div>
            <ul>
              <li>每月 {Number(p.monthly_token_limit).toLocaleString()} tokens</li>
              <li>{p.rpm_limit} 次/分钟（RPM）</li>
              <li>并发 {p.concurrency_limit} · 单日 {Number(p.daily_token_limit).toLocaleString()} tokens</li>
            </ul>
          </div>
        ))}
        {plans.length === 0 && !error && <p className="muted">暂无可售套餐，请联系运营方配置。</p>}
      </div>

      {selected && (
        <div className="card checkout">
          <h2>结算 · {selected.name}</h2>
          <p>支付方式：</p>
          <div className="channels">
            {channels.map((c) => (
              <label key={c} className={`channel ${channel === c ? 'active' : ''}`}>
                <input type="radio" name="channel" value={c} checked={channel === c} onChange={() => setChannel(c)} />
                {CHANNEL_LABELS[c] || c}
              </label>
            ))}
          </div>
          <button onClick={checkout} disabled={busy || !channel}>
            {busy ? '提交中…' : `确认支付 ¥${Number(selected.price_per_month).toFixed(2)}`}
          </button>
        </div>
      )}
    </div>
  )
}
