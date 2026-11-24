import { useState } from 'react'
import { adminAPI, formatNumber } from '../api'
import { useFetch } from '../components/useFetch'

const emptyForm = { name: '', plan_code: '', rpm_limit: 60, concurrency_limit: 4, daily_token_limit: 100000, monthly_token_limit: 1000000 }

export default function Tenants() {
  const { data, error, reload } = useFetch(() => adminAPI.get('/admin/tenants'), [])
  const [form, setForm] = useState(emptyForm)
  const [secret, setSecret] = useState(null)
  const [quotaFor, setQuotaFor] = useState('')
  const [quota, setQuota] = useState({ plan_id: '', monthly_token_limit: 1000000, rpm_limit: 60, concurrency_limit: 4, daily_token_limit: 100000 })
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')

  const field = (k) => (e) => setForm({ ...form, [k]: ['rpm_limit', 'concurrency_limit', 'daily_token_limit', 'monthly_token_limit'].includes(k) ? Number(e.target.value) : e.target.value })

  async function createTenant(e) {
    e.preventDefault()
    setBusy(true); setMsg('')
    try {
      const r = await adminAPI.post('/admin/tenants', form)
      setSecret(r)
      setForm(emptyForm)
      reload()
    } catch (err) { setMsg(err.message) } finally { setBusy(false) }
  }

  async function issueKey(id) {
    setBusy(true); setMsg('')
    try {
      const r = await adminAPI.post(`/admin/tenants/${id}/keys`, {
        rpm_limit: 60, concurrency_limit: 4, daily_token_limit: 100000,
      })
      setSecret(r)
    } catch (err) { setMsg(err.message) } finally { setBusy(false) }
  }

  async function saveQuota(e) {
    e.preventDefault()
    setBusy(true); setMsg('')
    try {
      await adminAPI.put(`/admin/tenants/${quotaFor}/quota`, quota)
      setQuotaFor(''); reload()
    } catch (err) { setMsg(err.message) } finally { setBusy(false) }
  }

  return (
    <>
      <h1>租户与 key</h1>
      {error && <div className="error">{error}</div>}
      {msg && <div className="error">{msg}</div>}

      {secret && (
        <div className="card">
          <h2>请立即保存明文（只显示这一次）</h2>
          {secret.api_key_plaintext && <><div className="muted">客户 API key</div><div className="secret">{secret.api_key_plaintext}</div></>}
          {secret.console_token_plaintext && <><div className="muted" style={{ marginTop: 10 }}>客户控制台令牌</div><div className="secret">{secret.console_token_plaintext}</div></>}
          <p className="muted">把控制台令牌交给对应租户；API key 用于在他们自己的服务里调用网关。</p>
          <button className="secondary" onClick={() => setSecret(null)}>我已保存</button>
        </div>
      )}

      <div className="card">
        <h2>创建租户并签发 key</h2>
        <form onSubmit={createTenant} className="grid cols-4">
          <input placeholder="租户名称" value={form.name} onChange={field('name')} required />
          <input placeholder="套餐 code（留空自动创建）" value={form.plan_code} onChange={field('plan_code')} />
          <input type="number" placeholder="每分钟请求数" value={form.rpm_limit} onChange={field('rpm_limit')} />
          <input type="number" placeholder="并发数" value={form.concurrency_limit} onChange={field('concurrency_limit')} />
          <input type="number" placeholder="单日 token 上限" value={form.daily_token_limit} onChange={field('daily_token_limit')} />
          <input type="number" placeholder="每月 token 上限" value={form.monthly_token_limit} onChange={field('monthly_token_limit')} />
          <button disabled={busy}>创建</button>
        </form>
      </div>

      {quotaFor && (
        <div className="card">
          <h2>设置额度 · {quotaFor}</h2>
          <form onSubmit={saveQuota} className="grid cols-4">
            <input placeholder="套餐 id（可留空）" value={quota.plan_id} onChange={(e) => setQuota({ ...quota, plan_id: e.target.value })} />
            <input type="number" placeholder="每月 token" value={quota.monthly_token_limit} onChange={(e) => setQuota({ ...quota, monthly_token_limit: Number(e.target.value) })} />
            <input type="number" placeholder="每分钟请求数" value={quota.rpm_limit} onChange={(e) => setQuota({ ...quota, rpm_limit: Number(e.target.value) })} />
            <input type="number" placeholder="并发数" value={quota.concurrency_limit} onChange={(e) => setQuota({ ...quota, concurrency_limit: Number(e.target.value) })} />
            <input type="number" placeholder="单日 token" value={quota.daily_token_limit} onChange={(e) => setQuota({ ...quota, daily_token_limit: Number(e.target.value) })} />
            <button disabled={busy}>保存额度</button>
            <button type="button" className="secondary" onClick={() => setQuotaFor('')}>取消</button>
          </form>
        </div>
      )}

      <div className="card table-wrap">
        <h2>租户列表</h2>
        <table>
          <thead><tr><th>租户</th><th>套餐</th><th>余额</th><th>本月 token</th><th>本月费用</th><th>有效 key</th><th>操作</th></tr></thead>
          <tbody>
            {(data || []).map((t) => (
              <tr key={t.id}>
                <td>{t.name}<div className="muted" style={{ fontSize: 11 }}>{t.id}</div></td>
                <td>{t.plan}</td>
                <td>{formatNumber(t.balance)}</td>
                <td>{formatNumber(t.month_tokens)}</td>
                <td>{formatNumber(t.month_cost)}</td>
                <td>{t.active_keys}</td>
                <td>
                  <button className="secondary" onClick={() => issueKey(t.id)} disabled={busy}>签发 key</button>{' '}
                  <button className="secondary" onClick={() => { setQuotaFor(t.id); setQuota({ ...quota, monthly_token_limit: t.monthly_token_limit || 1000000 }) }}>设置额度</button>
                </td>
              </tr>
            ))}
            {data && data.length === 0 && <tr><td colSpan="7" className="empty">还没有租户，请先在上方创建</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
