const SESSION_KEY = 'aiporto_user_session'

export function saveSession(token) {
  sessionStorage.setItem(SESSION_KEY, token.trim())
}

export function loadSession() {
  return sessionStorage.getItem(SESSION_KEY) || ''
}

export function clearSession() {
  sessionStorage.removeItem(SESSION_KEY)
}

async function call(path, { method = 'GET', body, auth = false } = {}) {
  const headers = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (auth) {
    const token = loadSession()
    if (!token) throw new Error('请先登录')
    headers.Authorization = 'Bearer ' + token
  }
  let res
  try {
    res = await fetch('/api' + path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch (e) {
    throw new Error('无法连接服务器，请检查网络或稍后重试')
  }
  let data = {}
  try { data = await res.json() } catch { /* 空响应体 */ }
  if (!res.ok) {
    if (res.status === 401 && auth) {
      clearSession()
      window.dispatchEvent(new Event('user-session-expired'))
    }
    throw new Error(data.error || `请求失败（${res.status}）`)
  }
  return data
}

export const userAPI = {
  register: (email, password) => call('/user/register', { method: 'POST', body: { email, password } }),
  verify: (email, code, password) => call('/user/verify', { method: 'POST', body: { email, code, password } }),
  login: (email, password) => call('/user/login', { method: 'POST', body: { email, password } }),
  logout: () => call('/user/logout', { method: 'POST', auth: true }),
  me: () => call('/user/me', { auth: true }),
  plans: () => call('/user/plans'),
  channels: () => call('/user/pay/channels'),
  createOrder: (planId, channel) => call('/user/orders', { method: 'POST', auth: true, body: { plan_id: planId, channel } }),
  orders: () => call('/user/orders', { auth: true }),
  order: (no) => call('/user/orders/' + encodeURIComponent(no), { auth: true }),
  cancelOrder: (no) => call('/user/orders/' + encodeURIComponent(no) + '/cancel', { method: 'POST', auth: true }),
  takeCredentials: (no) => call('/user/orders/' + encodeURIComponent(no) + '/credentials', { method: 'POST', auth: true }),
}
