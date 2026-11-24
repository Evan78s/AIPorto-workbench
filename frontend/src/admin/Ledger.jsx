import { useState } from 'react'
import { adminAPI, formatNumber } from '../api'
import { useFetch } from '../components/useFetch'

export default function Ledger() {
  const [tid, setTid] = useState('')
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [msg, setMsg] = useState('')
  const { data, error, reload } = useFetch(
    () => adminAPI.get(`/admin/ledger?tenant_id=${encodeURIComponent(tid)}`),
    [tid],
  )

  async function submit(e) {
    e.preventDefault()
    setMsg('')
    try {
      await adminAPI.post('/admin/recharge', { tenant_id: tid, amount: Number(amount), note })
      setAmount(''); setNote(''); reload()
    } catch (err) { setMsg(err.message) }
  }

  return (
    <>
      <h1>账本流水</h1>
      {error && <div className="error">{error}</div>}
      {msg && <div className="error">{msg}</div>}
      <div className="card">
        <h2>手动充值</h2>
        <form onSubmit={submit} className="grid cols-4">
          <input placeholder="租户 id" value={tid} onChange={(e) => setTid(e.target.value)} required />
          <input type="number" step="0.01" placeholder="金额" value={amount} onChange={(e) => setAmount(e.target.value)} required />
          <input placeholder="备注（如：对公收款流水号）" value={note} onChange={(e) => setNote(e.target.value)} />
          <button>充值</button>
        </form>
        <p className="muted">同一笔收款请复用相同 reference_id（本页留空会自动生成），避免重复入账。</p>
      </div>
      <div className="card table-wrap">
        <h2>流水</h2>
        <table>
          <thead><tr><th>流水号</th><th>金额</th><th>币种</th><th>备注</th><th>时间</th></tr></thead>
          <tbody>
            {(data || []).map((r) => (
              <tr key={r.reference_id}><td>{r.reference_id}</td><td>{formatNumber(r.amount)}</td><td>{r.currency}</td><td>{r.description}</td><td>{r.created_at}</td></tr>
            ))}
            {data && data.length === 0 && <tr><td colSpan="5" className="empty">暂无流水</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
