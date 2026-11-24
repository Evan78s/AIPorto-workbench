import { useState } from 'react'
import { adminAPI, formatNumber } from '../api'
import { useFetch } from '../components/useFetch'

const emptyPrice = { model: '', input_price_per_1m: '', output_price_per_1m: '', currency: 'CNY' }

export default function Plans() {
  const plans = useFetch(() => adminAPI.get('/admin/plans'), [])
  const [price, setPrice] = useState(emptyPrice)
  const [msg, setMsg] = useState('')

  async function savePrice(e) {
    e.preventDefault()
    setMsg('')
    try {
      await adminAPI.put('/admin/pricing', {
        model: price.model,
        input_price_per_1m: Number(price.input_price_per_1m),
        output_price_per_1m: Number(price.output_price_per_1m),
        currency: price.currency,
      })
      setPrice(emptyPrice)
    } catch (err) { setMsg(err.message) }
  }

  return (
    <>
      <h1>套餐与价格</h1>
      {plans.error && <div className="error">{plans.error}</div>}
      {msg && <div className="error">{msg}</div>}
      <div className="card table-wrap">
        <h2>套餐</h2>
        <table>
          <thead><tr><th>code</th><th>名称</th><th>月 token</th><th>RPM</th><th>并发</th><th>日 token</th><th>月费</th></tr></thead>
          <tbody>
            {(plans.data || []).map((p) => (
              <tr key={p.id}><td>{p.code}</td><td>{p.name}</td><td>{formatNumber(p.monthly_token_limit)}</td><td>{p.rpm_limit}</td><td>{p.concurrency_limit}</td><td>{formatNumber(p.daily_token_limit)}</td><td>{p.price_per_month} {p.currency}</td></tr>
            ))}
            {plans.data && plans.data.length === 0 && <tr><td colSpan="7" className="empty">暂无套餐</td></tr>}
          </tbody>
        </table>
      </div>
      <div className="card">
        <h2>模型价格（每百万 token）</h2>
        <form onSubmit={savePrice} className="grid cols-4">
          <input placeholder="真实模型名，如 deepseek-chat" value={price.model} onChange={(e) => setPrice({ ...price, model: e.target.value })} required />
          <input type="number" step="0.000001" placeholder="输入单价" value={price.input_price_per_1m} onChange={(e) => setPrice({ ...price, input_price_per_1m: e.target.value })} required />
          <input type="number" step="0.000001" placeholder="输出单价" value={price.output_price_per_1m} onChange={(e) => setPrice({ ...price, output_price_per_1m: e.target.value })} required />
          <input placeholder="币种" value={price.currency} onChange={(e) => setPrice({ ...price, currency: e.target.value })} />
          <button>保存价格</button>
        </form>
        <p className="muted">模型名必须与 usage_logs.model_resolved 一致；未配置的模型费用按 0 计算。</p>
      </div>
    </>
  )
}
