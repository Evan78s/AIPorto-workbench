import { useFetch } from '../components/useFetch'
import { adminAPI } from '../api'

const LABELS = {
  low_balance: '余额低于阈值',
  usage_spike: '单日用量突增 3 倍',
  upstream_429_surge: '上游 429 激增',
}

export default function Alerts() {
  const { data, error, loading, reload } = useFetch(() => adminAPI.get('/admin/alerts'), [])
  return (
    <>
      <h1>异常告警</h1>
      {error && <div className="error">{error}</div>}
      <div className="card">
        <button className="secondary" onClick={reload} disabled={loading}>{loading ? '刷新中…' : '重新检测'}</button>
        <span className="muted" style={{ marginLeft: 12 }}>实时计算，不依赖后台任务</span>
      </div>
      <div className="card table-wrap">
        <table>
          <thead><tr><th>类型</th><th>租户</th><th>说明</th><th>当前值</th><th>阈值</th><th>检测时间</th></tr></thead>
          <tbody>
            {(data || []).map((a, i) => (
              <tr key={`${a.type}-${a.tenant_id}-${i}`}>
                <td><span className="badge low">{LABELS[a.type] || a.type}</span></td>
                <td>{a.tenant_id}</td>
                <td>{a.message}</td>
                <td>{a.value}</td>
                <td>{a.threshold}</td>
                <td>{a.detected_at}</td>
              </tr>
            ))}
            {data && data.length === 0 && <tr><td colSpan="6" className="empty">当前没有异常</td></tr>}
          </tbody>
        </table>
      </div>
      <p className="muted">余额阈值来自服务端 ALERT_LOW_BALANCE_THRESHOLD；另两类规则固定为 3 倍，且设置了最小样本量以避免噪声。</p>
    </>
  )
}
