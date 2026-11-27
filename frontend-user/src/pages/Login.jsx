import { useState } from 'react'
import { Link } from 'react-router-dom'
import { saveSession, userAPI } from '../api'

export default function Login({ onSuccess }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e) {
    e.preventDefault()
    setError(''); setBusy(true)
    try {
      const res = await userAPI.login(email, password)
      saveSession(res.session_token)
      onSuccess(res.session_token)
    } catch (err) { setError(err.message) } finally { setBusy(false) }
  }

  return (
    <div className="auth-wrap">
      <form onSubmit={submit} className="card auth-card form">
        <h1>登录 Aiporto</h1>
        <p className="muted">使用注册邮箱与密码登录用户中心。</p>
        <label>邮箱</label>
        <input type="email" placeholder="you@example.com" value={email} onChange={(e) => setEmail(e.target.value)} required />
        <label>密码</label>
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        <button disabled={busy}>{busy ? '登录中…' : '登录'}</button>
        <p className="muted">还没有账号？<Link to="/register">免费注册</Link></p>
        {error && <p className="error" role="alert">{error}</p>}
      </form>
    </div>
  )
}
