import { useState } from 'react'
import { adminAPI } from '../api'
import { useFetch } from '../components/useFetch'

export default function System() {
  const info = useFetch(() => adminAPI.get('/admin/system/version'), null)
  const [copied, setCopied] = useState('')

  async function copy(text, key) {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(key)
      setTimeout(() => setCopied(''), 2000)
    } catch (_) {
      setCopied('')
    }
  }

  const command = info.data?.update_command || 'cd /www/docker/aiporto && bash scripts/update.sh'

  return (
    <>
      <h1>系统更新</h1>
      <p className="muted">在此查看当前版本与升级方式。更新操作在服务器终端执行，本页不执行任何系统命令。</p>

      <div className="card">
        <h2>当前版本</h2>
        <div className="stat" style={{ maxWidth: 320 }}>
          <div className="label">运行中代码版本（git commit）</div>
          <div className="value" style={{ fontSize: 20, fontFamily: 'ui-monospace, monospace' }}>
            {info.data ? info.data.version : (info.error ? '读取失败' : '加载中…')}
          </div>
        </div>
        {info.error && <p className="error">{info.error}</p>}
        <p className="muted">这个哈希对应你服务器上的代码版本。升级成功后会变化；若没变化说明没有新版本或升级未生效。</p>
      </div>

      <div className="card">
        <h2>一条命令完成升级</h2>
        <p>在服务器 SSH 终端粘贴下面这条命令并回车，脚本会自动完成：<strong>备份 → 拉新代码 → 补跑数据库迁移 → 重建容器 → 健康检查 → 失败自动回滚</strong>。</p>
        <div className="secret" style={{ display: 'flex', alignItems: 'center', gap: 12, justifyContent: 'space-between' }}>
          <code>{command}</code>
          <button className="secondary" onClick={() => copy(command, 'cmd')}>{copied === 'cmd' ? '已复制' : '复制命令'}</button>
        </div>
        <div className="notice" style={{ marginTop: 14 }}>
          <strong>为什么要敲命令而不是点按钮？</strong> 网页点按钮更新，需要把服务器的 Docker 最高权限交给运行在公网的服务，风险极高。这条命令只需几秒，且全程可追溯、可回滚，是更安全的选择。
        </div>
      </div>

      <div className="card">
        <h2>升级前必读</h2>
        <ol style={{ lineHeight: 1.9, paddingLeft: 22, margin: 0 }}>
          <li>升级会先<strong>自动备份数据库</strong>并校验完整性；备份失败会直接中止，不会动你的数据。</li>
          <li>升级过程中网关会短暂重建（约几十秒），用户在途请求可能中断，建议避开高峰期。</li>
          <li>若本地改过代码（未提交），脚本会拒绝升级并提示，避免覆盖你的修改。</li>
          <li>升级后如果新版本有问题，用下面这条命令回滚到上一版本（脚本成功时也会在日志里打印它）。</li>
        </ol>
        <p className="muted" style={{ marginTop: 14 }}>升级日志保存在 <code>backups/update.log</code>，出问题先看它。</p>
      </div>

      <div className="card">
        <h2>手动回滚</h2>
        <p>把下面命令里的 <code>旧版本号</code> 换成升级前记录的 commit（升级日志里有），在服务器执行：</p>
        <div className="secret" style={{ display: 'flex', alignItems: 'center', gap: 12, justifyContent: 'space-between' }}>
          <code>cd /www/docker/aiporto &amp;&amp; git checkout 旧版本号 &amp;&amp; docker compose -f docker-compose.prod.yaml up -d --build --force-recreate gateway admin-ui user-ui</code>
          <button className="secondary" onClick={() => copy('cd /www/docker/aiporto && git checkout 旧版本号 && docker compose -f docker-compose.prod.yaml up -d --build --force-recreate gateway admin-ui user-ui', 'rollback')}>
            {copied === 'rollback' ? '已复制' : '复制'}
          </button>
        </div>
        <p className="muted">注意：回滚代码不会撤销数据库结构变更。若新版本改了表结构导致异常，需用 <code>scripts/restore.sh</code> 从升级前的备份恢复数据库。</p>
      </div>
    </>
  )
}
