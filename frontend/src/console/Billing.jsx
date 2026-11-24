import { consoleAPI, formatNumber, saveResponse } from '../api'
import { useFetch } from '../components/useFetch'

export default function Billing() {
  const recharges = useFetch(() => consoleAPI.get('/console/recharges'), [])

  async function download(path, name) {
    try {
      const res = await consoleAPI.download(path)
      await saveResponse(res, name)
    } catch (e) { alert(e.message) }
  }

  return (
    <>
      <h1>充值记录与账单</h1>
      {recharges.error && <div className="error">{recharges.error}</div>}
      <div className="card">
        <h2>下载</h2>
        <button onClick={() => download('/console/export/usage.csv?days=30', 'usage.csv')}>下载近 30 天用量 CSV</button>{' '}
        <button className="secondary" onClick={() => download('/console/export/bill.csv?days=30', 'bill.csv')}>下载账单 CSV</button>
        <p className="muted">导出内容只有 token、状态码、耗时与费用，不含 prompt 正文、key 或租户信息。</p>
      </div>
      <div className="card table-wrap">
        <h2>充值记录</h2>
        <table>
          <thead><tr><th>流水号</th><th>金额</th><th>币种</th><th>备注</th><th>时间</th></tr></thead>
          <tbody>
            {(recharges.data || []).map((r) => (
              <tr key={r.reference_id}><td>{r.reference_id}</td><td>{formatNumber(r.amount)}</td><td>{r.currency}</td><td>{r.description}</td><td>{r.created_at}</td></tr>
            ))}
            {recharges.data && recharges.data.length === 0 && <tr><td colSpan="5" className="empty">暂无充值记录</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
