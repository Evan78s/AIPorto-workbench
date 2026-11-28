import { useEffect, useState } from 'react'
import { Link, NavLink, Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { clearSession, loadSession, userAPI } from './api'
import Register from './pages/Register'
import Login from './pages/Login'
import Plans from './pages/Plans'
import Orders from './pages/Orders'
import OrderDetail from './pages/OrderDetail'
import Profile from './pages/Profile'
import MyPlan from './pages/MyPlan'
import Docs from './pages/Docs'

function Shell({ onLogout }) {
  const [me, setMe] = useState(null)
  const [menuOpen, setMenuOpen] = useState(false)

  useEffect(() => {
    userAPI.me().then(setMe).catch(() => {})
  }, [])

  const displayName = me?.nickname || me?.email?.split('@')[0] || '用户'
  const initial = [...displayName][0]?.toUpperCase() || 'U'

  function logout() {
    setMenuOpen(false)
    onLogout()
  }

  return (
    <div className="portal">
      <header className="topbar">
        <Link to="/" className="brand">Aiporto <span>智港</span><em>万模入港，统一调度</em></Link>
        <nav>
          <NavLink to="/plans">套餐</NavLink>
          <NavLink to="/docs">接入指南</NavLink>
          <NavLink to="/my-plan">我的套餐</NavLink>
          <NavLink to="/orders">我的订单</NavLink>
          <div className="user-menu">
            <button className="user-trigger" onClick={() => setMenuOpen(!menuOpen)} aria-label="用户菜单">
              <span className="avatar">{initial}</span>
              <span className="user-name">{displayName}</span>
              <span className="caret">▾</span>
            </button>
            {menuOpen && (
              <>
                <div className="menu-backdrop" onClick={() => setMenuOpen(false)} />
                <div className="user-dropdown">
                  <div className="dropdown-head">
                    <span className="avatar large">{initial}</span>
                    <div>
                      <div className="dropdown-name">{displayName}</div>
                      <div className="dropdown-email">{me?.email || ''}</div>
                      <div className="dropdown-plan">套餐：{me?.plan || '未购买'}</div>
                    </div>
                  </div>
                  <Link to="/profile" onClick={() => setMenuOpen(false)}>个人资料</Link>
                  <Link to="/my-plan" onClick={() => setMenuOpen(false)}>我的套餐</Link>
                  <Link to="/docs" onClick={() => setMenuOpen(false)}>接入指南</Link>
                  <button className="link-btn" onClick={logout}>退出登录</button>
                </div>
              </>
            )}
          </div>
        </nav>
      </header>
      <main className="content"><Outlet /></main>
      <footer className="footer">Aiporto 智港 · 多租户大模型 API 网关</footer>
    </div>
  )
}

function Home() {
  return (
    <div className="hero">
      <h1>Aiporto <span>智港</span></h1>
      <p className="slogan">万模入港，统一调度</p>
      <p className="lead">一个 Key，接入多家大模型。统一管理额度、用量与账单，专注你的业务。</p>
      <div className="hero-actions">
        <Link to="/plans" className="btn">选择套餐</Link>
        <Link to="/register" className="btn secondary">免费注册</Link>
        <Link to="/docs" className="btn secondary">接入指南</Link>
      </div>
      <div className="features">
        <div className="feature"><h3>统一接口</h3><p>OpenAI 兼容协议，切换模型无需改代码。</p></div>
        <div className="feature"><h3>流式优先</h3><p>SSE 全链路不缓冲，首字响应更快。</p></div>
        <div className="feature"><h3>用量透明</h3><p>实时查看按模型、按天的 token 与费用。</p></div>
      </div>
    </div>
  )
}

export default function App() {
  const [token, setToken] = useState(() => loadSession())

  useEffect(() => {
    const expired = () => setToken('')
    window.addEventListener('user-session-expired', expired)
    return () => window.removeEventListener('user-session-expired', expired)
  }, [])

  function logout() {
    userAPI.logout().catch(() => {})
    clearSession()
    setToken('')
  }

  return (
    <Routes>
      <Route path="/" element={<Home />} />
      <Route path="/register" element={token ? <Navigate to="/plans" replace /> : <Register onSuccess={setToken} />} />
      <Route path="/login" element={token ? <Navigate to="/plans" replace /> : <Login onSuccess={setToken} />} />
      <Route path="/docs" element={<Docs />} />
      <Route element={token ? <Shell onLogout={logout} /> : <Navigate to="/login" replace />}>
        <Route path="/plans" element={<Plans />} />
        <Route path="/my-plan" element={<MyPlan />} />
        <Route path="/profile" element={<Profile />} />
        <Route path="/orders" element={<Orders />} />
        <Route path="/orders/:no" element={<OrderDetail />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
