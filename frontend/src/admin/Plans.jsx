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
  const pricing = useFetch(() => adminAPI.get('/admin/pricing'), [])
  const [price, setPrice] = useState(emptyPrice)
  const [plan, setPlan] = useState(emptyPlan)
  const [msg, setMsg] = useState('')
  const [planMsg, setPlanMsg] = useState('')
  const [savedPlan, setSavedPlan] = useState('')
  const [editingPlan, setEditingPlan] = useState(null)
  const [editingPrice, setEditingPrice] = useState(null)

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
      setEditingPrice(null)
      pricing.reload()
      setMsg('模型价格已保存')
    } catch (err) { setMsg(err.message) }
  }

  async function savePlanEdit(e) {
    e.preventDefault()
    setPlanMsg('')
    try {
      await adminAPI.put(`/admin/plans/${editingPlan.id}`, {
        name: editingPlan.name,
        monthly_token_limit: Number(editingPlan.monthly_token_limit),
        rpm_limit: Number(editingPlan.rpm_limit),
        concurrency_limit: Number(editingPlan.concurrency_limit),
        daily_token_limit: Number(editingPlan.daily_token_limit),
        price_per_month: Number(editingPlan.price_per_month),
      })
      setEditingPlan(null)
      plans.reload()
    } catch (err) { setPlanMsg(err.message) }
  }

  async function removePlan(id, code) {
    if (!window.confirm(`确认删除套餐 ${code}？已被订阅或订单引用的套餐不能删除。`)) return
    setPlanMsg('')
    try {
      await adminAPI.delete(`/admin/plans/${id}`)
      plans.reload()
    } catch (err) { setPlanMsg(err.message) }
  }

  async function removePrice(id, model) {
    if (!window.confirm(`确认删除模型 ${model} 的价格配置？删除后该模型费用按 0 计算。`)) return
    try {
      await adminAPI.delete(`/admin/pricing/${id}`)
      pricing.reload()
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
        <p className="muted">下面每个字段的含义都写在标题下方。照着示例填即可，填完点「新增套餐」。</p>
        <form onSubmit={savePlan} className="plan-form">
          <label className="field">
            <span className="field-label">套餐 code <em>必填</em></span>
            <span className="field-hint">唯一编号，只能用英文小写、数字和连字符</span>
            <input placeholder="例如：pro-monthly" value={plan.code} onChange={(e) => setPlan({ ...plan, code: e.target.value })} required />
          </label>
          <label className="field">
            <span className="field-label">套餐名称 <em>必填</em></span>
            <span className="field-hint">用户在购买页看到的名字</span>
            <input placeholder="例如：专业版" value={plan.name} onChange={(e) => setPlan({ ...plan, name: e.target.value })} required />
          </label>
          <label className="field">
            <span className="field-label">每月 token 额度 <em>必填</em></span>
            <span className="field-hint">一个月内累计可调用的最大 token 数，用完当月停用</span>
            <input type="number" min="1" value={plan.monthly_token_limit} onChange={(e) => setPlan({ ...plan, monthly_token_limit: e.target.value })} required />
            <span className="field-example">示例 1000000 = 100 万 token</span>
          </label>
          <label className="field">
            <span className="field-label">RPM（每分钟请求数）<em>必填</em></span>
            <span className="field-hint">同一用户每分钟最多发多少次请求，超了会返回 429</span>
            <input type="number" min="1" value={plan.rpm_limit} onChange={(e) => setPlan({ ...plan, rpm_limit: e.target.value })} required />
            <span className="field-example">示例 60 = 每秒 1 次</span>
          </label>
          <label className="field">
            <span className="field-label">并发数 <em>必填</em></span>
            <span className="field-hint">同一用户同时进行的请求上限，流式对话会占用</span>
            <input type="number" min="1" value={plan.concurrency_limit} onChange={(e) => setPlan({ ...plan, concurrency_limit: e.target.value })} required />
            <span className="field-example">示例 4 = 最多同时 4 路</span>
          </label>
          <label className="field">
            <span className="field-label">每日 token 额度 <em>必填</em></span>
            <span className="field-hint">单日最大 token 数，防止一天把月额度耗光</span>
            <input type="number" min="1" value={plan.daily_token_limit} onChange={(e) => setPlan({ ...plan, daily_token_limit: e.target.value })} required />
            <span className="field-example">示例 100000 = 每天 10 万 token</span>
          </label>
          <label className="field">
            <span className="field-label">月费（元）<em>必填</em></span>
            <span className="field-hint">用户每月支付的价格，必须大于 0 才会显示在购买页</span>
            <input type="number" step="0.01" min="0.01" placeholder="例如：99" value={plan.price_per_month} onChange={(e) => setPlan({ ...plan, price_per_month: e.target.value })} required />
            <span className="field-example">示例 99 = 每月 99 元</span>
          </label>
          <div className="field field-action">
            <button type="submit">新增套餐</button>
          </div>
        </form>
        <div className="notice" style={{ marginTop: 14, marginBottom: 0 }}>
          <strong>重要：</strong>月费必须大于 0，否则套餐不会出现在用户端购买页。同一个 code 重复提交会更新已有套餐，不会重复创建。
        </div>
        {planMsg && <p className="error">{planMsg}</p>}
        {savedPlan && <p className="badge ok">套餐「{savedPlan}」已上架，用户端刷新后可见</p>}
      </div>
      <div className="card table-wrap">
        <h2>套餐列表</h2>
        <table>
          <thead><tr><th>code</th><th>名称</th><th>月 token</th><th>RPM</th><th>并发</th><th>日 token</th><th>月费</th><th>操作</th></tr></thead>
          <tbody>
            {(plans.data || []).map((p) => (
              <tr key={p.id}>
                <td><code>{p.code}</code></td><td>{p.name}</td><td>{formatNumber(p.monthly_token_limit)}</td>
                <td>{p.rpm_limit}</td><td>{p.concurrency_limit}</td><td>{formatNumber(p.daily_token_limit)}</td>
                <td>{p.price_per_month} {p.currency}</td>
                <td>
                  <button className="secondary" onClick={() => setEditingPlan({ ...p })}>编辑</button>{' '}
                  <button className="secondary" onClick={() => removePlan(p.id, p.code)}>删除</button>
                </td>
              </tr>
            ))}
            {plans.data && plans.data.length === 0 && <tr><td colSpan="8" className="empty">暂无套餐</td></tr>}
          </tbody>
        </table>
      </div>

      {editingPlan && (
        <div className="card">
          <h2>编辑套餐 · <code>{editingPlan.code}</code></h2>
          <form onSubmit={savePlanEdit} className="plan-form">
            <label className="field"><span className="field-label">套餐名称</span>
              <input value={editingPlan.name} onChange={(e) => setEditingPlan({ ...editingPlan, name: e.target.value })} required /></label>
            <label className="field"><span className="field-label">每月 token 额度</span>
              <input type="number" min="1" value={editingPlan.monthly_token_limit} onChange={(e) => setEditingPlan({ ...editingPlan, monthly_token_limit: e.target.value })} required /></label>
            <label className="field"><span className="field-label">RPM</span>
              <input type="number" min="1" value={editingPlan.rpm_limit} onChange={(e) => setEditingPlan({ ...editingPlan, rpm_limit: e.target.value })} required /></label>
            <label className="field"><span className="field-label">并发数</span>
              <input type="number" min="1" value={editingPlan.concurrency_limit} onChange={(e) => setEditingPlan({ ...editingPlan, concurrency_limit: e.target.value })} required /></label>
            <label className="field"><span className="field-label">每日 token 额度</span>
              <input type="number" min="1" value={editingPlan.daily_token_limit} onChange={(e) => setEditingPlan({ ...editingPlan, daily_token_limit: e.target.value })} required /></label>
            <label className="field"><span className="field-label">月费（元）</span>
              <input type="number" step="0.01" min="0" value={editingPlan.price_per_month} onChange={(e) => setEditingPlan({ ...editingPlan, price_per_month: e.target.value })} required />
              <span className="field-hint">填 0 会从用户购买页下架（下架比删除更安全）</span></label>
            <div className="field field-action">
              <button type="submit">保存</button>
              <button type="button" className="secondary" onClick={() => setEditingPlan(null)}>取消</button>
            </div>
          </form>
        </div>
      )}
      <div className="card">
        <h2>模型价格（每百万 token）</h2>
        <p className="muted">这里的价格用来计算「用量看板」的成本和租户余额扣减：成本 = 输入 token / 100万 × 输入单价 + 输出 token / 100万 × 输出单价。模型名必须是路由里填的<strong>真实模型名</strong>（usage_logs.model_resolved），不是用户调用的别名；没配价格的模型费用按 0 算。</p>
        <form onSubmit={savePrice} className="grid cols-4">
          <input placeholder="真实模型名，如 deepseek-chat" value={price.model} onChange={(e) => setPrice({ ...price, model: e.target.value })} readOnly={!!editingPrice} required />
          <input type="number" step="0.000001" placeholder="输入单价" value={price.input_price_per_1m} onChange={(e) => setPrice({ ...price, input_price_per_1m: e.target.value })} required />
          <input type="number" step="0.000001" placeholder="输出单价" value={price.output_price_per_1m} onChange={(e) => setPrice({ ...price, output_price_per_1m: e.target.value })} required />
          <input placeholder="币种" value={price.currency} onChange={(e) => setPrice({ ...price, currency: e.target.value })} />
          <button>{editingPrice ? '更新价格' : '保存价格'}</button>
          {editingPrice && <button type="button" className="secondary" onClick={() => { setEditingPrice(null); setPrice(emptyPrice) }}>取消编辑</button>}
        </form>
      </div>

      <div className="card table-wrap">
        <h2>已配置价格</h2>
        <table>
          <thead><tr><th>真实模型名</th><th>输入单价/百万 token</th><th>输出单价/百万 token</th><th>币种</th><th>操作</th></tr></thead>
          <tbody>
            {(pricing.data || []).map((p) => (
              <tr key={p.id}>
                <td><code>{p.model}</code></td>
                <td>{p.input_price_per_1m}</td><td>{p.output_price_per_1m}</td><td>{p.currency}</td>
                <td>
                  <button className="secondary" onClick={() => {
                    setEditingPrice(p.id)
                    setPrice({ model: p.model, input_price_per_1m: String(p.input_price_per_1m), output_price_per_1m: String(p.output_price_per_1m), currency: p.currency })
                  }}>编辑</button>{' '}
                  <button className="secondary" onClick={() => removePrice(p.id, p.model)}>删除</button>
                </td>
              </tr>
            ))}
            {pricing.data && pricing.data.length === 0 && <tr><td colSpan="5" className="empty">还没有配置模型价格</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
