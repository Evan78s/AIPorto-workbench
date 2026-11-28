import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { userAPI } from '../api'

const num = (n) => Number(n || 0).toLocaleString()

export default function MyPlan() {
  const [plan, setPlan] = useState(null)
  const [keys, setKeys] = useState([])
  const [error, setError] = useState('')
  const [rotatePw, setRotatePw] = useState('')
  const [rotateMsg, setRotateMsg] = useState('')
  const [rotating, setRotating] = useState(false)
  const [newKey, setNewKey] = useState('')

  useEffect(() => {
    userAPI.myPlan().then(setPlan).catch((e) => setError(e.message))
    userAPI.keys().then((d) => setKeys(d.keys || [])).catch(() => {})
  }, [])

  if (error) return <div className="card"><p className="error">{error}</p></div>
  if (!plan) return <div className="card"><p className="muted">加载中…</p></div>

  async function rotate(e) {
    e.preventDefault()
    setRotateMsg(''); setRotating(true)
    try {
      const res = await userAPI.rotateKey(rotatePw)
      setNewKey(res.api_key_plaintext || '')
      setRotatePw('')
      setRotateMsg('✓ ' + (res.note || '已生成新 Key'))
      const d = await userAPI.keys().catch(() => ({ keys: [] }))
      setKeys(d.keys || [])
    } catch (err) { setRotateMsg(err.message) } finally { setRotating(false) }
  }

  async function copyKey() {
    if (!newKey) return
    try { await navigator.clipboard.writeText(newKey); setRotateMsg('✓ 已复制到剪贴板') }
    catch { setRotateMsg('复制失败，请手动选中复制') }
  }

  const limit = Number(plan.monthly_token_limit || 0)
  const used = Number(plan.month_tokens || 0)
  const pct = limit > 0 ? Math.min(100, Math.round(used / limit * 100)) : 0

  return (
    <div>
      <h1>我的套餐</h1>
      <p className="muted">查看当前套餐额度、用量与已开通的 API Key。</p>

      <div className="card">
        <div className="plan-head">
          <h2>{plan.plan}</h2>
          <span className={`badge ${plan.status === 'active' ? 'ok' : ''}`}>
            {plan.status === 'active' ? '生效中' : plan.status === 'none' ? '未购买' : plan.status}
          </span>
        </div>
        {plan.status === 'none' ? (
          <p className="muted">你还没有购买套餐。<Link to="/plans">去选择套餐 →</Link></p>
        ) : (
          <>
            <div className="usage-block">
              <div className="usage-row">
                <span>本月用量</span>
                <strong>{num(used)} / {num(limit)} tokens</strong>
              </div>
              <div className="progress"><div className="progress-fill" style={{ width: pct + '%' }} /></div>
              <div className="usage-row muted">
                <span>已用 {pct}%</span>
                <span>剩余 {num(plan.remaining_tokens)} tokens</span>
              </div>
            </div>
            <div className="stat-grid">
              <div className="stat"><div className="stat-label">每分钟请求（RPM）</div><div className="stat-value">{num(plan.rpm_limit)}</div></div>
              <div className="stat"><div className="stat-label">并发数</div><div className="stat-value">{num(plan.concurrency_limit)}</div></div>
              <div className="stat"><div className="stat-label">单日 Token 上限</div><div className="stat-value">{num(plan.daily_token_limit)}</div></div>
              <div className="stat"><div className="stat-label">月费</div><div className="stat-value">{plan.currency} {Number(plan.price_per_month || 0).toFixed(2)}</div></div>
            </div>
          </>
        )}
      </div>

      <div className="card">
        <h2>我的 API Key</h2>
        {keys.length === 0 ? (
          <p className="muted">暂无 API Key。购买套餐并支付成功后系统会自动开通。</p>
        ) : (
          <table className="key-table">
            <thead><tr><th>Key 前缀</th><th>状态</th><th>开通时间</th></tr></thead>
            <tbody>
              {keys.map((k, i) => (
                <tr key={i}>
                  <td><code>{k.key_prefix}…</code></td>
                  <td><span className={`badge ${k.status === 'active' ? 'ok' : ''}`}>{k.status === 'active' ? '正常' : k.status}</span></td>
                  <td className="muted">{k.created_at}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <p className="muted" style={{ marginTop: 12 }}>
          为安全起见，服务端只保存 Key 摘要，无法重新显示旧 Key。遗失后可在下方自行轮换，旧 Key 会立即失效。
        </p>
      </div>

      <div className="card">
        <h2>轮换 API Key</h2>
        <p className="muted">如怀疑 Key 泄露或忘记保存，验证当前登录密码后即可生成新 Key。旧 Key 立即停用，请及时更新你的应用配置。</p>
        <form onSubmit={rotate} className="form">
          <label>当前登录密码</label>
          <input type="password" value={rotatePw} onChange={(e) => setRotatePw(e.target.value)} required autoComplete="current-password" placeholder="输入密码确认是你的操作" />
          <button type="submit" disabled={rotating} className="danger" style={{ alignSelf: 'flex-start' }}>
            {rotating ? '正在轮换…' : '轮换并生成新 Key'}
          </button>
        </form>
        {newKey && (
          <div className="new-key-box" style={{ marginTop: 14 }}>
            <p className="muted" style={{ margin: '0 0 6px' }}>新 Key（仅显示这一次，请立即保存）：</p>
            <code>{newKey}</code>
            <button type="button" className="secondary" onClick={copyKey} style={{ marginTop: 10 }}>复制 Key</button>
          </div>
        )}
        {rotateMsg && <p className={rotateMsg.startsWith('✓') ? 'ok-msg' : 'error'}>{rotateMsg}</p>}
      </div>

      <div className="card">
        <h2>开始接入</h2>
        <p className="muted">拿到 Key 后，按接入指南配置 base URL 即可调用。</p>
        <Link to="/docs" className="btn secondary">查看接入指南 →</Link>
      </div>
    </div>
  )
}
