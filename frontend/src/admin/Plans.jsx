import { useState } from 'react'
import { adminAPI, formatNumber } from '../api'
import { useFetch } from '../components/useFetch'

const emptyPrice = { model: '', input_price_per_1m: '', output_price_per_1m: '', currency: 'CNY' }
const emptyPlan = {
  code: '', name: '', monthly_token_limit: 1000000, rpm_limit: 60,
  concurrency_limit: 4, daily_token_limit: 100000, price_per_month: '', currency: 'CNY',
}

export default function Plans() {
  const plans = useFetch(() => adminAPI.get('/admin/plans'), [])
  const [price, setPrice] = useState(emptyPrice)
  const [plan, setPlan] = useState(emptyPlan)
  const [msg, setMsg] = useState('')
  const [planMsg, setPlanMsg] = useState('')
  const [savedPlan, setSavedPlan] = useState('')

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
      setMsg('模型价格已保存')
    } catch (err) { setMsg(err.message) }
  }

  async function savePlan(e) {
    e.preventDefault()
    setPlanMsg('')
    try {
      await adminAPI.post('/admin/plans', {
        code: plan.code.trim(),
        name: plan.name.trim(),
        monthly_token_limit: Number(plan.monthly_token_limit),
        rpm_limit: Number(plan.rpm_limit),
        concurrency_limit: Number(plan.concurrency_limit),
        daily_token_limit: Number(plan.daily_token_limit),
        price_per_month: Number(plan.price_per_month),
      })
      setSavedPlan(plan.code.trim())
      setPlan(emptyPlan)
      plans.reload()
    } catch (err) { setPlanMsg(err.message) }
  }

  return (
    <>
      <h1>套餐与价格</h1>
      {plans.error && <div className="error">{plans.error}</div>}
      {msg && <div className={msg === '模型价格已保存' ? 'badge ok' : 'error'}>{msg}</div>}
      <div className="card">
        <h2>新增可售套餐</h2>
        <form onSubmit={savePlan} className="grid cols-4">
          <input placeholder="套餐 code，如 pro-monthly" value={plan.code} onChange={(e) => setPlan({ ...plan, code: e.target.value })} required />
          <input placeholder="套餐名称，如 专业版" value={plan.name} onChange={(e) => setPlan({ ...plan, name: e.target.value })} required />
          <input type="number" placeholder="每月 token" value={plan.monthly_token_limit} onChange={(e) => setPlan({ ...plan, monthly_token_limit: e.target.value })} required />
          <input type="number" placeholder="RPM" value={plan.rpm_limit} onChange={(e) => setPlan({ ...plan, rpm_limit: e.target.value })} required />
          <input type="number" placeholder="并发数" value={plan.concurrency_limit} onChange={(e) => setPlan({ ...plan, concurrency_limit: e.target.value })} required />
          <input type="number" placeholder="每日 token" value={plan.daily_token_limit} onChange={(e) => setPlan({ ...plan, daily_token_limit: e.target.value })} required />
          <input type="number" step="0.01" placeholder="月费（元）" value={plan.price_per_month} onChange={(e) => setPlan({ ...plan, price_per_month: e.target.value })} required />
          <button>新增套餐</button>
        </form>
        <p className="muted">月费必须大于 0 才会显示在用户端购买页。同一 code 重复添加会更新已有套餐。</p>
        {planMsg && <p className="error">{planMsg}</p>}
        {savedPlan && <p className="badge ok">套餐「{savedPlan}」已上架，用户端刷新后可见</p>}
      </div>
      <div className="card table-wrap">
        <h2>套餐列表</h2>
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
