import { useState } from 'react'
import { NavLink, Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { loadConsoleToken, saveConsoleToken } from '../api'
import Overview from './Overview'
import Usage from './Usage'
import Keys from './Keys'
import Billing from './Billing'
import Snippets from './Snippets'

function TokenBar() {
  const [token, setToken] = useState('')
  const configured = Boolean(loadConsoleToken())
  return (
    <div className="topbar">
      <strong>客户控制台</strong>
      <span className="muted">只能看到本租户数据；令牌由运营方签发</span>
      <span className="spacer" />
      <input placeholder="粘贴控制台令牌 cw-..." value={token} onChange={(e) => setToken(e.target.value)} style={{ minWidth: 280 }} />
      <button onClick={() => { saveConsoleToken(token); location.reload() }} disabled={!token}>保存</button>
      <button className="secondary" onClick={() => { localStorage.removeItem('wb_console_token'); location.reload() }}>清除</button>
      <span className="badge">{configured ? '已配置' : '未配置'}</span>
    </div>
  )
}

function Shell() {
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">Aiporto<small>智港 · 客户控制台</small><em className="slogan">万模入港，统一调度</em></div>
        <nav className="nav">
          <NavLink to="/console/overview">总览</NavLink>
          <NavLink to="/console/usage">用量曲线</NavLink>
          <NavLink to="/console/keys">API key</NavLink>
          <NavLink to="/console/billing">充值与账单</NavLink>
          <NavLink to="/console/snippets">调用示例</NavLink>
        </nav>
        <div className="switch"><NavLink to="/admin">切换到运营后台 →</NavLink></div>
      </aside>
      <main className="main">
        <TokenBar />
        <Outlet />
      </main>
    </div>
  )
}

export default function ConsoleApp() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<Navigate to="overview" replace />} />
        <Route path="overview" element={<Overview />} />
        <Route path="usage" element={<Usage />} />
        <Route path="keys" element={<Keys />} />
        <Route path="billing" element={<Billing />} />
        <Route path="snippets" element={<Snippets />} />
      </Route>
    </Routes>
  )
}
