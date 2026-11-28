import { useState } from 'react'
import { adminAPI } from '../api'
import { useFetch } from '../components/useFetch'

const emptyRoute = { model_alias: '', provider_id: '', model: '', tenant_id: '', priority: 100 }

export default function Routes() {
  const routes = useFetch(() => adminAPI.get('/admin/routes'), [])
  const providers = useFetch(() => adminAPI.get('/admin/providers'), [])
  const tenants = useFetch(() => adminAPI.get('/admin/tenants'), [])
  const [form, setForm] = useState(emptyRoute)
  const [editing, setEditing] = useState(null)
  const [msg, setMsg] = useState('')
  const [okMsg, setOkMsg] = useState('')
  const [modelsFor, setModelsFor] = useState('')   // 当前拉取模型的厂商 id
  const [modelOptions, setModelOptions] = useState([])
  const [editingModels, setEditingModels] = useState([])

  async function fetchModels(pid, target) {
    if (!pid) return
    try {
      const res = await adminAPI.get(`/admin/providers/${pid}/models`)
      const list = res.models || []
      if (target === 'edit') setEditingModels(list)
      else { setModelOptions(list); setModelsFor(pid) }
      success(`已从厂商拉取 ${list.length} 个模型`)
    } catch (err) { notify(err) }
  }

  function notify(err) { setOkMsg(''); setMsg(err.message || String(err)) }
  function success(text) {
    setMsg(''); setOkMsg(text)
    setTimeout(() => setOkMsg(''), 4000)
  }

  async function addRoute(e) {
    e.preventDefault()
    try {
      await adminAPI.post('/admin/routes', form)
      setForm(emptyRoute)
      routes.reload()
      success('路由已添加')
    } catch (err) { notify(err) }
  }

  async function toggle(r) {
    try {
      await adminAPI.put(`/admin/routes/${r.id}`, {
        provider_id: r.provider_id,
        model: r.model,
        priority: Number(r.priority),
        enabled: !r.enabled,
      })
      routes.reload()
      success(r.enabled ? '路由已停用' : '路由已启用')
    } catch (err) { notify(err) }
  }

  async function remove(id) {
    if (!window.confirm('确认删除这条路由？删除后立即生效。')) return
    try {
      await adminAPI.delete(`/admin/routes/${id}`)
      routes.reload()
      success('路由已删除')
    } catch (err) { notify(err) }
  }

  async function saveEdit(e) {
    e.preventDefault()
    try {
      await adminAPI.put(`/admin/routes/${editing.id}`, {
        provider_id: editing.provider_id,
        model: editing.model,
        priority: Number(editing.priority),
        enabled: editing.enabled,
      })
      setEditing(null)
      routes.reload()
      success('路由已更新')
    } catch (err) { notify(err) }
  }

  return (
    <>
      <h1>模型路由管理</h1>
      <p className="muted">客户用别名调用，网关按路由翻译成厂商真实模型。租户专属路由优先于全局路由，可实现"某个客户单独用别的模型"。</p>
      {msg && <div className="error">{msg}</div>}
      {okMsg && <div className="badge active">{okMsg}</div>}

      <div className="card table-wrap">
        <h2>路由列表</h2>
        <table>
          <thead><tr><th>模型别名</th><th>厂商</th><th>真实模型</th><th>作用域</th><th>优先级</th><th>状态</th><th>操作</th></tr></thead>
          <tbody>
            {(routes.data || []).map((r) => (
              <tr key={r.id}>
                <td><code>{r.model_alias}</code></td>
                <td>{r.provider_code}</td>
                <td>{r.model}</td>
                <td>{r.tenant_id ? '租户专属' : '全局'}</td>
                <td>{r.priority}</td>
                <td>{r.enabled ? '启用' : '停用'}</td>
                <td>
                  <button className="secondary" onClick={() => toggle(r)}>{r.enabled ? '停用' : '启用'}</button>{' '}
                  <button className="secondary" onClick={() => setEditing({ ...r, enabled: r.enabled })}>编辑</button>{' '}
                  <button className="secondary" onClick={() => remove(r.id)}>删除</button>
                </td>
              </tr>
            ))}
            {routes.data && routes.data.length === 0 && <tr><td colSpan="7" className="empty">暂无路由，先在下方添加</td></tr>}
          </tbody>
        </table>
      </div>

      {editing && (
        <div className="card">
          <h2>编辑路由 · <code>{editing.model_alias}</code></h2>
          <form onSubmit={saveEdit} className="grid cols-4">
            <select value={editing.provider_id} onChange={(e) => {
              setEditing({ ...editing, provider_id: e.target.value, model: '' })
              setEditingModels([])
            }} required>
              {(providers.data || []).map((p) => <option key={p.id} value={p.id}>{p.code}</option>)}
            </select>
            <button type="button" className="secondary" onClick={() => fetchModels(editing.provider_id, 'edit')}>拉取模型</button>
            <input list="edit-model-list" placeholder="真实模型名（可从拉取结果选）" value={editing.model} onChange={(e) => setEditing({ ...editing, model: e.target.value })} required />
            <datalist id="edit-model-list">
              {editingModels.map((m) => <option key={m} value={m} />)}
            </datalist>
            <input type="number" placeholder="优先级" value={editing.priority} onChange={(e) => setEditing({ ...editing, priority: e.target.value })} />
            <select value={editing.enabled ? '1' : '0'} onChange={(e) => setEditing({ ...editing, enabled: e.target.value === '1' })}>
              <option value="1">启用</option><option value="0">停用</option>
            </select>
            <button>保存</button>
            <button type="button" className="secondary" onClick={() => setEditing(null)}>取消</button>
          </form>
        </div>
      )}

      <div className="card">
        <h2>新增路由</h2>
        <form onSubmit={addRoute} className="grid cols-3">
          <input placeholder="客户用的别名，如 deepseek-chat" value={form.model_alias} onChange={(e) => setForm({ ...form, model_alias: e.target.value })} required />
          <select value={form.provider_id} onChange={(e) => {
            setForm({ ...form, provider_id: e.target.value, model: '' })
            setModelOptions([]); setModelsFor('')
          }} required>
            <option value="">选择厂商</option>
            {(providers.data || []).filter((p) => p.status === 'active').map((p) => <option key={p.id} value={p.id}>{p.code}</option>)}
          </select>
          <button type="button" className="secondary" onClick={() => fetchModels(form.provider_id, 'new')}>拉取模型</button>
          <input list="new-model-list" placeholder="真实模型名（先拉模型再选）" value={form.model} onChange={(e) => setForm({ ...form, model: e.target.value })} required />
          <datalist id="new-model-list">
            {modelsFor === form.provider_id && modelOptions.map((m) => <option key={m} value={m} />)}
          </datalist>
          <select value={form.tenant_id} onChange={(e) => setForm({ ...form, tenant_id: e.target.value })}>
            <option value="">全局（所有租户）</option>
            {(tenants.data || []).map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </select>
          <input type="number" placeholder="优先级" value={form.priority} onChange={(e) => setForm({ ...form, priority: e.target.value })} />
          <button>添加路由</button>
        </form>
        <p className="muted">先选厂商再点「拉取模型」，网关会用该厂商已保存的密钥调用上游 <code>/models</code> 接口，把模型列表填进输入框的候选项，选中后即为真实模型名。部分厂商不提供该接口，可手工填写。同一别名可配多条路由指向不同厂商，网关按优先级做负载均衡与失败转移；租户专属路由优先于全局路由。</p>
      </div>
    </>
  )
}
