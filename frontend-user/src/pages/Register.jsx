import { useState } from 'react'
import { Link } from 'react-router-dom'
import { saveSession, userAPI } from '../api'

export default function Register({ onSuccess }) {
  const [step, setStep] = useState(1)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [credentials, setCredentials] = useState(null)

  async function sendCode(e) {
    e.preventDefault()
    setError(''); setBusy(true)
    try {
      await userAPI.register(email, password)
      setStep(2)
    } catch (err) { setError(err.message) } finally { setBusy(false) }
  }

  async function verify(e) {
    e.preventDefault()
    setError(''); setBusy(true)
    try {
      const res = await userAPI.verify(email, code, password)
      saveSession(res.session_token)
      setCredentials(res)
      setStep(3)
      onSuccess(res.session_token)
    } catch (err) { setError(err.message) } finally { setBusy(false) }
  }

  return (
    <div className="auth-wrap">
      <div className="card auth-card">
        <h1>注册 Aiporto 账号</h1>
        {step === 1 && (
          <form onSubmit={sendCode} className="form">
            <p className="muted">注册后立即获得专属租户，购买套餐即可开启 API 调用。</p>
            <label>邮箱</label>
            <input type="email" placeholder="you@example.com" value={email} onChange={(e) => setEmail(e.target.value)} required />
            <label>密码（至少 8 位）</label>
            <input type="password" placeholder="设置一个密码" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
            <button disabled={busy}>{busy ? '发送中…' : '发送验证码'}</button>
            <p className="muted">已有账号？<Link to="/login">直接登录</Link></p>
          </form>
        )}
        {step === 2 && (
          <form onSubmit={verify} className="form">
            <p className="muted">验证码已发送至 <strong>{email}</strong>，5 分钟内有效。请再次输入密码完成注册。</p>
            <label>邮箱验证码</label>
            <input inputMode="numeric" maxLength={6} placeholder="6 位数字" value={code} onChange={(e) => setCode(e.target.value)} required />
            <label>密码</label>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
            <button disabled={busy || code.length !== 6}>{busy ? '验证中…' : '完成注册'}</button>
            <button type="button" className="secondary" onClick={() => setStep(1)}>返回修改</button>
          </form>
        )}
        {step === 3 && credentials && (
          <div className="form">
            <div className="badge ok">注册成功</div>
            <p>你的专属租户已创建。前往「套餐」购买后即可获得 API Key 与控制台令牌。</p>
            <Link to="/plans" className="btn">查看套餐</Link>
          </div>
        )}
        {error && <p className="error" role="alert">{error}</p>}
      </div>
    </div>
  )
}
