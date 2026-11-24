import { consoleAPI, formatNumber } from '../api'
import { useFetch } from '../components/useFetch'
import LineChart from '../components/LineChart'

export default function Overview() {
  const overview = useFetch(() => consoleAPI.get('/console/overview'), [])
  const daily = useFetch(() => consoleAPI.get('/console/usage/daily?days=30'), [])
  const o = overview.data || {}

  return (
    <>
      <h1>本月总览</h1>
      {overview.error && <div className="error">{overview.error}</div>}
      <div className="grid cols-4">
        <div className="stat"><div className="label">本月 token</div><div className="value">{formatNumber(o.month_tokens)}</div></div>
        <div className="stat"><div className="label">剩余额度 token</div><div className="value">{formatNumber(o.remaining_tokens)}</div></div>
        <div className="stat"><div className="label">本月费用</div><div className="value">{formatNumber(o.month_cost)}</div></div>
        <div className="stat"><div className="label">账户余额</div><div className="value">{formatNumber(o.balance)}</div></div>
      </div>
      <div className="card" style={{ marginTop: 18 }}>
        <div className="muted">套餐：{o.plan || '—'} · 每月上限 {formatNumber(o.monthly_token_limit)} token · 今日已用 {formatNumber(o.today_tokens)} token</div>
      </div>
      <div className="card"><LineChart points={daily.data || []} title="近 30 天 token" /></div>
    </>
  )
}
