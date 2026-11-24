import { useState } from 'react'
import { consoleAPI, formatNumber } from '../api'
import { useFetch } from '../components/useFetch'

export default function Keys() {
  const { data, error, reload } = useFetch(() => consoleAPI.get('/console/keys'), [])
  const [secret, setSecret] = useState('')
  const [msg, setMsg] = useState('')

  async function disable(id) {
    setMsg('')
    try { await consoleAPI.post(`/console/keys/${id}/disable`, {}); reload() }
    catch (err) { setMsg(err.message) }
  }
  async function rotate() {
    setMsg('')
    try {
      const r = await consoleAPI.post('/console/keys/rotate', {})
      setSecret(r.api_key_plaintext)
      reload()
    } catch (err) { setMsg(err.message) }
  }

  return (
    <>
      <h1>API key</h1>
      {error && <div className="error">{error}</div>}
      {msg && <div className="error">{msg}</div>}
      {secret && (
        <div className="card">
          <h2>新 key（只显示这一次）</h2>
          <div className="secret">{secret}</div>
          <p className="muted">旧 key 已立即停用。请把新 key 更新到你的服务配置里。</p>
          <button className="secondary" onClick={() => setSecret('')}>我已保存</button>
        </div>
      )}
      <div className="card">
        <button onClick={rotate}>重建 key（停用全部旧 key）</button>
        <span className="muted" style={{ marginLeft: 12 }}>数据库中只保存哈希，明文无法找回</span>
      </div>
      <div className="card table-wrap">
        <table>
          <thead><tr><th>前缀</th><th>状态</th><th>每分钟</th><th>并发</th><th>日 token</th><th>最后使用</th><th>操作</th></tr></thead>
          <tbody>
            {(data || []).map((k) => (
              <tr key={k.id}>
                <td>{k.key_prefix}</td>
                <td><span className={`badge ${k.status === 'active' ? 'active' : 'revoked'}`}>{k.status}</span></td>
                <td>{k.rpm_limit}</td><td>{k.concurrency_limit}</td><td>{formatNumber(k.daily_token_limit)}</td>
                <td className="muted">{k.last_used_at || '从未'}</td>
                <td><button className="danger" disabled={k.status !== 'active'} onClick={() => disable(k.id)}>停用</button></td>
              </tr>
            ))}
            {data && data.length === 0 && <tr><td colSpan="7" className="empty">暂无 key，请联系运营方签发</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
