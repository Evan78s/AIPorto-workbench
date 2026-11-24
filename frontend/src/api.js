const ADMIN_SESSION_KEY = 'wb_admin_session'
const CONSOLE_KEY = 'wb_console_token'

// 管理后台会话只放 sessionStorage，关闭标签页即失效；不持久化账号密码。
export function saveAdminSession(token) {
  sessionStorage.setItem(ADMIN_SESSION_KEY, token.trim())
}
export function loadAdminSession() {
  return sessionStorage.getItem(ADMIN_SESSION_KEY) || ''
}
export function clearAdminSession() {
  sessionStorage.removeItem(ADMIN_SESSION_KEY)
}
export function saveConsoleToken(token) {
  localStorage.setItem(CONSOLE_KEY, token.trim())
}
export function loadConsoleToken() {
  return localStorage.getItem(CONSOLE_KEY) || ''
}

async function call(path, { admin = false, method = 'GET', body, download = false } = {}) {
  const headers = { 'Content-Type': 'application/json' }
  if (admin) {
    const session = loadAdminSession()
    if (!session) throw new Error('请先完成双因素登录')
    headers.Authorization = 'Bearer ' + session
  } else {
    const token = loadConsoleToken()
    if (!token) throw new Error('请先在页面顶部填写控制台令牌')
    headers.Authorization = 'Bearer ' + token
  }
  const res = await fetch('/api' + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    let message = `请求失败 (${res.status})`
    try {
      const data = await res.json()
      if (data.error) message = data.error
    } catch (_) { /* 保留默认错误信息 */ }
    throw new Error(message)
  }
  if (download) return res
  return res.json()
}

// 登录接口不携带会话；分两步换取 8 小时会话令牌。
async function authCall(path, body) {
  const res = await fetch('/api/admin/auth/' + path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || `登录失败 (${res.status})`)
  return data
}

export const adminAuthAPI = {
  login: (username, password) => authCall('login', { username, password }),
  verify: (challenge_token, code) => authCall('verify', { challenge_token, code }),
}

export const adminAPI = {
  get: (p) => call(p, { admin: true }),
  post: (p, body) => call(p, { admin: true, method: 'POST', body }),
  put: (p, body) => call(p, { admin: true, method: 'PUT', body }),
}

export const consoleAPI = {
  get: (p) => call(p),
  post: (p, body) => call(p, { method: 'POST', body }),
  download: (p) => call(p, { download: true }),
}

export async function saveResponse(res, filename) {
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

export function formatNumber(n) {
  return new Intl.NumberFormat('zh-CN').format(Number(n || 0))
}
