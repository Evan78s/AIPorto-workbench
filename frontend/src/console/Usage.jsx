import { consoleAPI, formatNumber } from '../api'
import { useFetch } from '../components/useFetch'
import LineChart from '../components/LineChart'

export default function Usage() {
  const daily = useFetch(() => consoleAPI.get('/console/usage/daily?days=30'), [])
  const byModel = useFetch(() => consoleAPI.get('/console/usage/by-model?days=30'), [])
  return (
    <>
      <h1>用量曲线</h1>
      {daily.error && <div className="error">{daily.error}</div>}
      {byModel.error && <div className="error">{byModel.error}</div>}
      <div className="card"><LineChart points={daily.data || []} title="每日 token" /></div>
      <div className="card table-wrap">
        <h2>按模型</h2>
        <table>
          <thead><tr><th>模型</th><th>请求数</th><th>token</th><th>费用</th></tr></thead>
          <tbody>
            {(byModel.data || []).map((r) => (
              <tr key={r.label}><td>{r.label}</td><td>{formatNumber(r.requests)}</td><td>{formatNumber(r.tokens)}</td><td>{formatNumber(r.cost)}</td></tr>
            ))}
            {byModel.data && byModel.data.length === 0 && <tr><td colSpan="4" className="empty">暂无数据</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
