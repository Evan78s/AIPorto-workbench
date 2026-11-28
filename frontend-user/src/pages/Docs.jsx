import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { userAPI } from '../api'

function CodeBlock({ title, code }) {
  const [copied, setCopied] = useState(false)
  function copy() {
    navigator.clipboard?.writeText(code).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    }).catch(() => {})
  }
  return (
    <div className="code-block">
      <div className="code-head">
        <span>{title}</span>
        <button className="copy-btn" onClick={copy}>{copied ? '已复制' : '复制'}</button>
      </div>
      <pre><code>{code}</code></pre>
    </div>
  )
}

export default function Docs() {
  const [config, setConfig] = useState(null)
  const [tab, setTab] = useState('quickstart')

  useEffect(() => {
    userAPI.config().then(setConfig).catch(() => setConfig({ api_base_url: 'https://gw.example.com', chat_path: '/v1/chat/completions' }))
  }, [])

  const base = config?.api_base_url || 'https://gw.example.com'
  const endpoint = base + (config?.chat_path || '/v1/chat/completions')
  const modelsEndpoint = base + '/v1/models'

  const curlCode = `curl ${endpoint} \\
  -H "Authorization: Bearer sk-wb-你的APIKey" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "你的模型别名",
    "messages": [{"role": "user", "content": "你好"}],
    "stream": false
  }'`

  const pythonCode = `from openai import OpenAI

client = OpenAI(
    api_key="sk-wb-你的APIKey",
    base_url="${base}/v1",
)

resp = client.chat.completions.create(
    model="你的模型别名",
    messages=[{"role": "user", "content": "你好"}],
)
print(resp.choices[0].message.content)`

  const jsCode = `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "sk-wb-你的APIKey",
  baseURL: "${base}/v1",
});

const resp = await client.chat.completions.create({
  model: "你的模型别名",
  messages: [{ role: "user", content: "你好" }],
});
console.log(resp.choices[0].message.content);`

  const sseCode = `# 流式输出：把 stream 设为 true
curl -N ${endpoint} \\
  -H "Authorization: Bearer sk-wb-你的APIKey" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"你的模型别名","stream":true,
       "messages":[{"role":"user","content":"写首诗"}]}'`

  return (
    <div>
      <h1>API 接入指南</h1>
      <p className="muted">Aiporto 智港兼容 OpenAI 协议，一次接入即可调用多家大模型。按下面步骤配置即可开始使用。</p>

      <div className="doc-tabs">
        <button className={tab === 'quickstart' ? 'active' : ''} onClick={() => setTab('quickstart')}>快速开始</button>
        <button className={tab === 'clients' ? 'active' : ''} onClick={() => setTab('clients')}>客户端配置</button>
        <button className={tab === 'faq' ? 'active' : ''} onClick={() => setTab('faq')}>常见问题</button>
      </div>

      {tab === 'quickstart' && (
        <>
          <div className="card">
            <h2>① 准备 API Key</h2>
            <ol className="steps">
              <li>注册并登录账号，<Link to="/plans">选择套餐</Link>并完成支付。</li>
              <li>支付成功后，到 <Link to="/orders">我的订单</Link> → 订单详情领取 <strong>API Key</strong> 和 <strong>控制台令牌</strong>（明文只显示一次，请立即保存）。</li>
              <li>API Key 形如 <code>sk-wb-xxxxxxxx</code>，请妥善保管，不要提交到代码仓库或公开渠道。</li>
            </ol>
          </div>

          <div className="card">
            <h2>② 接口地址</h2>
            <table className="kv-table">
              <tbody>
                <tr><th>接口地址（Base URL）</th><td><code>{base}</code></td></tr>
                <tr><th>对话补全</th><td><code>POST {config?.chat_path || '/v1/chat/completions'}</code></td></tr>
                <tr><th>模型列表</th><td><code>GET /v1/models</code></td></tr>
                <tr><th>鉴权方式</th><td><code>Authorization: Bearer sk-wb-你的APIKey</code></td></tr>
                <tr><th>协议</th><td>OpenAI 兼容（支持 <code>stream: true</code> 流式输出）</td></tr>
              </tbody>
            </table>
            <p className="muted" style={{ marginTop: 10 }}>完整对话地址：<code>{endpoint}</code></p>
          </div>

          <div className="card">
            <h2>③ 模型别名</h2>
            <p>请求体里的 <code>model</code> 字段填<strong>模型别名</strong>，不是厂商的真实模型名。网关会根据别名自动路由到对应厂商。可用别名由运营方配置，可调用模型列表接口查看：</p>
            <CodeBlock title="查看可用模型" code={`curl ${modelsEndpoint} \\\n  -H "Authorization: Bearer sk-wb-你的APIKey"`} />
            <p className="muted">若返回 <code>no route for model</code>，说明该别名未配置或厂商密钥未就绪，请联系运营方。</p>
          </div>

          <div className="card">
            <h2>④ 第一个请求</h2>
            <CodeBlock title="curl 示例" code={curlCode} />
          </div>
        </>
      )}

      {tab === 'clients' && (
        <>
          <div className="card">
            <h2>OpenAI 官方 SDK（Python / Node.js）</h2>
            <p className="muted">只需把 <code>base_url</code> 指向本网关、<code>api_key</code> 换成你的 Key，其余代码不用改。</p>
            <CodeBlock title="Python" code={pythonCode} />
            <CodeBlock title="Node.js" code={jsCode} />
          </div>

          <div className="card">
            <h2>OpenClaw / Claude Code 等工具</h2>
            <p>这类工具通常支持自定义 OpenAI 兼容端点。在工具的模型配置里填写：</p>
            <table className="kv-table">
              <tbody>
                <tr><th>API Base URL</th><td><code>{base}/v1</code></td></tr>
                <tr><th>API Key</th><td><code>sk-wb-你的APIKey</code></td></tr>
                <tr><th>模型名（Model）</th><td>填入你有权限的模型别名，例如运营方提供的别名</td></tr>
              </tbody>
            </table>
            <p className="muted" style={{ marginTop: 10 }}>如果工具只接受完整的对话地址，填 <code>{endpoint}</code>。</p>
          </div>

          <div className="card">
            <h2>定制工作台 / 自己的应用</h2>
            <p>任何能发 HTTP 请求的程序都能接入：带上 Bearer Key、指定模型别名、POST JSON 即可。流式场景把 <code>stream</code> 设为 <code>true</code> 并逐行读取 SSE 事件。</p>
            <CodeBlock title="流式请求（SSE）" code={sseCode} />
          </div>
        </>
      )}

      {tab === 'faq' && (
        <div className="card">
          <h2>常见问题</h2>
          <div className="faq">
            <h3>提示 401 / invalid session 怎么办？</h3>
            <p>检查 Key 是否填写正确、是否带了 <code>Bearer </code> 前缀，以及套餐是否过期。Key 明文只显示一次，遗失需联系运营方重置。</p>
            <h3>提示 no route for model？</h3>
            <p>说明 <code>model</code> 填的别名未配置。用模型列表接口查看可用别名，或联系运营方添加路由。</p>
            <h3>提示额度不足 / 429？</h3>
            <p>本月 token 或 RPM/并发额度已用完。到 <Link to="/my-plan">我的套餐</Link> 查看用量，或升级套餐。</p>
            <h3>支持流式输出吗？</h3>
            <p>支持。请求体加 <code>"stream": true</code>，全链路 SSE 不缓冲，首字响应更快。</p>
            <h3>费用怎么计算？</h3>
            <p>按真实模型与输入/输出 token 计费，从套餐余额中扣减。具体单价由运营方在后台配置。</p>
          </div>
        </div>
      )}
    </div>
  )
}
