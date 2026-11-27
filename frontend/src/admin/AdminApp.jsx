import { useEffect, useState } from 'react'
import { NavLink, Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { adminAuthAPI, clearAdminSession, loadAdminSession, saveAdminSession } from '../api'
import Tenants from './Tenants'
import Usage from './Usage'
import Ledger from './Ledger'
import Alerts from './Alerts'
import Plans from './Plans'
import Providers from './Providers'
import RouteAdmin from './Routes'

// 未通过双因素验证时，只渲染登录界面，不泄露任何后台页面/导航/数据。
function LoginGate({ onSuccess }) {
  const [step, setStep] = useState('credentials')
  const [user, setUser] = useState('')
  const [pass, setPass] = useState('')
  const [code, setCode] = useState('')
  const [challenge, setChallenge] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function doLogin() {
    setError('')
    setBusy(true)
    try {
      const res = await adminAuthAPI.login(user, pass)
      setChallenge(res.challenge_token)
      setStep('otp')
    } catch (e) { setError(e.message) } finally { setBusy(false) }
  }

  async function doVerify() {
    setError('')
    setBusy(true)
    try {
      const res = await adminAuthAPI.verify(challenge, code)
      saveAdminSession(res.token)
      onSuccess(res.token)
    } catch (e) { setError(e.message) } finally { setBusy(false) }
  }

  return (
    <div className="login-wrap">
      <div className="login-card card">
        <div className="brand">Aiporto<small>智港 · 内部运营后台</small><em className="slogan">万模入港，统一调度</em></div>
        <h1>运营后台登录</h1>
        <p className="muted">双因素验证：账号密码 + 验证器 6 位动态码。全部通过后才可进入后台。</p>

        {step === 'credentials' && (
          <div className="login-form">
            <label>ADMIN_USER</label>
            <input placeholder="管理员账号" value={user} onChange={(e) => setUser(e.target.value)} autoComplete="username" />
            <label>ADMIN_PASSWORD</label>
            <input placeholder="管理员密码" type="password" value={pass} onChange={(e) => setPass(e.target.value)} autoComplete="current-password" onKeyDown={(e) => e.key === 'Enter' && user && pass && doLogin()} />
            <button onClick={doLogin} disabled={busy || !user || !pass}>下一步 · 验证动态码</button>
          </div>
        )}

        {step === 'otp' && (
          <div className="login-form">
            <p className="muted">请输入验证器（Aegis / FreeOTP）中的 6 位动态码</p>
            <input placeholder="000000" inputMode="numeric" maxLength={6} value={code} onChange={(e) => setCode(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && code.length === 6 && doVerify()} />
            <button onClick={doVerify} disabled={busy || code.length !== 6}>登录</button>
            <button className="secondary" onClick={() => { setStep('credentials'); setCode(''); setError('') }}>上一步</button>
          </div>
        )}

        {error && <p className="error" role="alert">{error}</p>}
      </div>
    </div>
  )
}

function Shell({ onLogout }) {
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">Aiporto<small>智港 · 内部运营后台</small><em className="slogan">万模入港，统一调度</em></div>
        <nav className="nav">
          <NavLink to="/admin/tenants">租户与 key</NavLink>
          <NavLink to="/admin/providers">厂商管理</NavLink>
          <NavLink to="/admin/routes">模型路由</NavLink>
          <NavLink to="/admin/usage">用量看板</NavLink>
          <NavLink to="/admin/ledger">账本流水</NavLink>
          <NavLink to="/admin/alerts">异常告警</NavLink>
          <NavLink to="/admin/plans">套餐与价格</NavLink>
        </nav>
        <div className="switch">
          <NavLink to="/console">切换到客户控制台 →</NavLink>
          <button className="secondary logout-btn" onClick={onLogout}>退出登录</button>
        </div>
      </aside>
      <main className="main">
        <div className="topbar">
          <strong>运营后台</strong>
          <span className="badge active">已通过双因素验证</span>
          <span className="spacer" />
        </div>
        <Outlet />
      </main>
    </div>
  )
}

export default function AdminApp() {
  // 会话状态：本地令牌存在且未过期才进入后台；接口返回 401 时强制回到登录页。
  const [token, setToken] = useState(() => loadAdminSession())

  useEffect(() => {
    const onExpired = () => setToken('')
    window.addEventListener('admin-session-expired', onExpired)
    return () => window.removeEventListener('admin-session-expired', onExpired)
  }, [])

  if (!token) {
    return <LoginGate onSuccess={(t) => setToken(t)} />
  }

  return (
    <Routes>
      <Route element={<Shell onLogout={() => { clearAdminSession(); setToken('') }} />}>
        <Route index element={<Navigate to="tenants" replace />} />
        <Route path="tenants" element={<Tenants />} />
        <Route path="providers" element={<Providers />} />
        <Route path="routes" element={<RouteAdmin />} />
        <Route path="usage" element={<Usage />} />
        <Route path="ledger" element={<Ledger />} />
        <Route path="alerts" element={<Alerts />} />
        <Route path="plans" element={<Plans />} />
      </Route>
    </Routes>
  )
}
