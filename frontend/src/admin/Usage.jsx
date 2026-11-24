import { useState } from 'react'
import { adminAPI, formatNumber } from '../api'
import { useFetch } from '../components/useFetch'
import LineChart from '../components/LineChart'

export default function Usage() {
  const [group, setGroup] = useState('day')
  const [tid, setTid] = useState('')
  const { data, error } = useFetch(
    () => adminAPI.get(`/admin/usage?group=${group}&tenant_id=${encodeURIComponent(tid)}&days=30`),
    [group, tid],
  )
  return (
    <>
      <h1>用量看板</h1>
      {error && <div className="error">{error}</div>}
      <div className="card">
        <div className="topbar" style={{ margin: 0 }}>
          <span>维度</span>
          <select value={group} onChange={(e) => setGroup(e.target.value)}>
            <option value="day">按天</option>
            <option value="tenant">按租户</option>
            <option value="model">按模型</option>
          </select>
          <span>租户 id（可留空）</span>
          <input placeholder="留空=全部租户" value={tid} onChange={(e) => setTid(e.target.value)} />
        </div>
        {group === 'day' && <LineChart points={data || []} title="每日 token" />}
      </div>
      <div className="card table-wrap">
        <table>
          <thead><tr><th>{group === 'tenant' ? '租户 id' : group === 'model' ? '模型' : '日期'}</th><th>请求数</th><th>token</th><th>费用</th></tr></thead>
          <tbody>
            {(data || []).map((r) => (
              <tr key={r.label}><td>{r.label}</td><td>{formatNumber(r.requests)}</td><td>{formatNumber(r.tokens)}</td><td>{formatNumber(r.cost)}</td></tr>
            ))}
            {data && data.length === 0 && <tr><td colSpan="4" className="empty">暂无用量数据</td></tr>}
          </tbody>
        </table>
      </div>
      <p className="muted">费用来自 model_pricing 表；未填写价格的模型费用显示为 0。</p>
    </>
  )
}
