import { useFetch } from '../components/useFetch'
import { consoleAPI } from '../api'

export default function Snippets() {
  const { data, error } = useFetch(() => consoleAPI.get('/console/snippets'), [])
  return (
    <>
      <h1>调用示例</h1>
      {error && <div className="error">{error}</div>}
      <div className="notice">示例中的占位符不会替换成真实 key；请不要把真实 key 提交到代码仓库。</div>
      <div className="card"><h2>curl</h2><pre>{data?.curl || '加载中…'}</pre></div>
      <div className="card"><h2>Python</h2><pre>{data?.python || '加载中…'}</pre></div>
      {data?.note && <p className="muted">{data.note}</p>}
    </>
  )
}
