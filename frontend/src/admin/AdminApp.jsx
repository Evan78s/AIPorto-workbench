import { useState } from 'react'
import { NavLink, Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { adminAuthAPI, clearAdminSession, loadAdminSession, saveAdminSession } from '../api'
import Tenants from './Tenants'
import Usage from './Usage'
import Ledger from './Ledger'
import Alerts from './Alerts'
import Plans from './Plans'

function AuthBar() {
  const [step, setStep] = useState(loadAdminSession() ? 'done' : 'credentials')
  const [user, setUser] = useState('')
  const [pass, setPass] = useState('')
  const [code, setCode] = useState('')
  const [challenge, setChallenge] = useState('')
  const [error, setError] = useState('')

  async function doLogin() {
    setError('')
    try {
      const res = await adminAuthAPI.login(user, pass)
      setChallenge(res.challenge_token)
      setStep('otp')
    } catch (e) { setError(e.message) }
  }

  async function doVerify() {
    setError('')
    try {
      const res = await adminAuthAPI.verify(challenge, code)
      saveAdminSession(res.token)
      setStep('done')
    } catch (e) { setError(e.message) }
  }

  return (
    <div className="topbar">
      <strong>运营后台</strong>
      <span className="muted">双因素登录：账号密码 + TOTP 动态码</span>
      <span className="spacer" />
      {step === 'credentials' && (
        <>
          <input placeholder="ADMIN_USER" value={user} onChange={(e) => setUser(e.target.value)} />
          <input placeholder="ADMIN_PASSWORD" type="password" value={pass} onChange={(e) => setPass(e.target.value)} />
          <button onClick={doLogin} disabled={!user || !pass}>下一步</button>
        </>
      )}
      {step === 'otp' && (
        <>
          <span className="muted">请输入验证器中的 6 位动态码</span>
          <input placeholder="000000" inputMode="numeric" maxLength={6} value={code} onChange={(e) => setCode(e.target.value)} />
          <button onClick={doVerify} disabled={code.length !== 6}>登录</button>
          <button className="secondary" onClick={() => setStep('credentials')}>上一步</button>
        </>
      )}
      {step === 'done' && (
        <>
          <span className="badge">已通过双因素验证</span>
          <button className="secondary" onClick={() => { clearAdminSession(); setStep('credentials') }}>退出</button>
        </>
      )}
      {error && <span className="badge" role="alert">{error}</span>}
    </div>
  )
}

function Shell() {
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">Aiporto<small>智港 · 内部运营后台</small><em className="slogan">万模入港，统一调度</em></div>
        <nav className="nav">
          <NavLink to="/admin/tenants">租户与 key</NavLink>
          <NavLink to="/admin/usage">用量看板</NavLink>
          <NavLink to="/admin/ledger">账本流水</NavLink>
          <NavLink to="/admin/alerts">异常告警</NavLink>
          <NavLink to="/admin/plans">套餐与价格</NavLink>
        </nav>
        <div className="switch"><NavLink to="/console">切换到客户控制台 →</NavLink></div>
      </aside>
      <main className="main">
        <AuthBar />
        <Outlet />
      </main>
    </div>
  )
}

export default function AdminApp() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<Navigate to="tenants" replace />} />
        <Route path="tenants" element={<Tenants />} />
        <Route path="usage" element={<Usage />} />
        <Route path="ledger" element={<Ledger />} />
        <Route path="alerts" element={<Alerts />} />
        <Route path="plans" element={<Plans />} />
      </Route>
    </Routes>
  )
}
