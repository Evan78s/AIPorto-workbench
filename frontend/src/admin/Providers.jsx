import { useState } from 'react'
import { adminAPI } from '../api'
import { useFetch } from '../components/useFetch'

const emptyProvider = { code: '', base_url: '', protocol: 'openai_compatible' }
const emptyKey = { provider_id: '', plain_key: '', name: '', tenant_id: '', priority: 100, weight: 1 }

export default function Providers() {
  const providers = useFetch(() => adminAPI.get('/admin/providers'), [])
  const tenants = useFetch(() => adminAPI.get('/admin/tenants'), [])
  const [provider, setProvider] = useState(emptyProvider)
  const [keyForm, setKeyForm] = useState(emptyKey)
  const [keysFor, setKeysFor] = useState('')
  const keys = useFetch(
    () => (keysFor ? adminAPI.get(`/admin/providers/${keysFor}/keys`) : Promise.resolve([])),
    [keysFor]
  )
  const [msg, setMsg] = useState('')
  const [okMsg, setOkMsg] = useState('')

  function notify(err) {
    setOkMsg('')
    setMsg(err.message || String(err))
  }
  function success(text) {
    setMsg('')
    setOkMsg(text)
    setTimeout(() => setOkMsg(''), 4000)
  }

  async function addProvider(e) {
    e.preventDefault()
    try {
      await adminAPI.post('/admin/providers', provider)
      setProvider(emptyProvider)
      providers.reload()
      success('厂商已添加')
    } catch (err) { notify(err) }
  }

  async function addKey(e) {
    e.preventDefault()
    try {
      const res = await adminAPI.post(`/admin/providers/${keyForm.provider_id}/keys`, {
        plain_key: keyForm.plain_key,
        name: keyForm.name,
        tenant_id: keyForm.tenant_id,
        priority: Number(keyForm.priority),
        weight: Number(keyForm.weight),
      })
      setKeyForm({ ...emptyKey, provider_id: keyForm.provider_id })
      if (keysFor === keyForm.provider_id) keys.reload()
      providers.reload()
      success(`密钥已加密入库，掩码 ${res.key_hint}（明文不会再显示）`)
    } catch (err) { notify(err) }
  }

  async function removeProvider(id, code) {
    if (!window.confirm(`确认停用厂商 ${code}？其所有密钥和路由会一并停用。`)) return
    try {
      await adminAPI.delete(`/admin/providers/${id}`)
      if (keysFor === id) setKeysFor('')
      providers.reload()
      success('厂商已停用')
    } catch (err) { notify(err) }
  }

  async function removeKey(id) {
    if (!window.confirm('确认停用这把厂商密钥？')) return
    try {
      await adminAPI.delete(`/admin/provider-keys/${id}`)
      keys.reload()
      providers.reload()
      success('密钥已停用')
    } catch (err) { notify(err) }
  }

  async function toggleProvider(id, current) {
    const next = current === 'active' ? 'archived' : 'active'
    const p = (providers.data || []).find((x) => x.id === id)
    if (!p) return
    try {
      await adminAPI.put(`/admin/providers/${id}`, { base_url: p.base_url, protocol: p.protocol, status: next })
      providers.reload()
      success(next === 'active' ? '厂商已启用' : '厂商已停用')
    } catch (err) { notify(err) }
  }

  return (
    <>
      <h1>厂商管理</h1>
      <p className="muted">配置上游模型厂商及其 API key。密钥经 AES-256-GCM 加密入库，页面只显示掩码，无法再次查看明文。</p>
      {msg && <div className="error">{msg}</div>}
      {okMsg && <div className="badge active">{okMsg}</div>}

      <div className="card table-wrap">
        <h2>厂商列表</h2>
        <table>
          <thead><tr><th>代号</th><th>API 地址</th><th>协议</th><th>密钥数</th><th>状态</th><th>操作</th></tr></thead>
          <tbody>
            {(providers.data || []).map((p) => (
              <tr key={p.id}>
                <td>{p.code}</td>
                <td>{p.base_url}</td>
                <td>{p.protocol}</td>
                <td>{p.key_count}</td>
                <td>{p.status}</td>
                <td>
                  <button className="secondary" onClick={() => setKeysFor(keysFor === p.id ? '' : p.id)}>密钥</button>{' '}
                  <button className="secondary" onClick={() => toggleProvider(p.id, p.status)}>{p.status === 'active' ? '停用' : '启用'}</button>{' '}
                  <button className="secondary" onClick={() => removeProvider(p.id, p.code)}>删除</button>
                </td>
              </tr>
            ))}
            {providers.data && providers.data.length === 0 && <tr><td colSpan="6" className="empty">暂无厂商，先在下方添加</td></tr>}
          </tbody>
        </table>
      </div>

      {keysFor && (
        <div className="card table-wrap">
          <h2>密钥 · {(providers.data || []).find((p) => p.id === keysFor)?.code || ''}</h2>
          {keys.error && <div className="error">{keys.error}</div>}
          <table>
            <thead><tr><th>名称</th><th>掩码</th><th>优先级</th><th>权重</th><th>归属租户</th><th>状态</th><th>操作</th></tr></thead>
            <tbody>
              {(keys.data || []).map((k) => (
                <tr key={k.id}>
                  <td>{k.name}</td><td><code>{k.key_hint}</code></td>
                  <td>{k.priority}</td><td>{k.weight}</td>
                  <td>{k.tenant_id || '全局共享'}</td><td>{k.status}</td>
                  <td><button className="secondary" onClick={() => removeKey(k.id)}>停用</button></td>
                </tr>
              ))}
              {keys.data && keys.data.length === 0 && <tr><td colSpan="7" className="empty">该厂商暂无密钥</td></tr>}
            </tbody>
          </table>
        </div>
      )}

      <div className="card">
        <h2>新增厂商</h2>
        <form onSubmit={addProvider} className="grid cols-3">
          <input placeholder="厂商代号，如 deepseek" value={provider.code} onChange={(e) => setProvider({ ...provider, code: e.target.value })} required />
          <input placeholder="API 根地址，如 https://api.deepseek.com" value={provider.base_url} onChange={(e) => setProvider({ ...provider, base_url: e.target.value })} required />
          <input placeholder="协议" value={provider.protocol} onChange={(e) => setProvider({ ...provider, protocol: e.target.value })} />
          <button>添加厂商</button>
        </form>
      </div>

      <div className="card">
        <h2>添加厂商密钥</h2>
        <form onSubmit={addKey} className="grid cols-3">
          <select value={keyForm.provider_id} onChange={(e) => setKeyForm({ ...keyForm, provider_id: e.target.value })} required>
            <option value="">选择厂商</option>
            {(providers.data || []).filter((p) => p.status === 'active').map((p) => (
              <option key={p.id} value={p.id}>{p.code}</option>
            ))}
          </select>
          <input type="password" placeholder="厂商 API key 明文（提交后不可再查看）" value={keyForm.plain_key} onChange={(e) => setKeyForm({ ...keyForm, plain_key: e.target.value })} required />
          <input placeholder="密钥名称（可选）" value={keyForm.name} onChange={(e) => setKeyForm({ ...keyForm, name: e.target.value })} />
          <select value={keyForm.tenant_id} onChange={(e) => setKeyForm({ ...keyForm, tenant_id: e.target.value })}>
            <option value="">全局共享（所有租户可用）</option>
            {(tenants.data || []).map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </select>
          <input type="number" placeholder="优先级" value={keyForm.priority} onChange={(e) => setKeyForm({ ...keyForm, priority: e.target.value })} />
          <input type="number" placeholder="权重" value={keyForm.weight} onChange={(e) => setKeyForm({ ...keyForm, weight: e.target.value })} />
          <button>加密保存密钥</button>
        </form>
        <p className="muted">留空"归属租户"即全局共享。密钥只在提交这一刻传入，之后页面只显示掩码，数据库存加密后的密文。</p>
      </div>
    </>
  )
}
