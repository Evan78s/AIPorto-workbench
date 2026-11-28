import { useEffect, useState } from 'react'
import { userAPI } from '../api'

export default function Profile() {
  const [me, setMe] = useState(null)
  const [nickname, setNickname] = useState('')
  const [oldPw, setOldPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [newPw2, setNewPw2] = useState('')
  const [profileMsg, setProfileMsg] = useState('')
  const [pwMsg, setPwMsg] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    userAPI.me().then((d) => { setMe(d); setNickname(d.nickname || '') }).catch((e) => setProfileMsg(e.message))
  }, [])

  async function saveProfile(e) {
    e.preventDefault()
    setProfileMsg(''); setBusy(true)
    try {
      const res = await userAPI.updateProfile(nickname)
      setMe({ ...me, nickname: res.nickname })
      setProfileMsg('✓ 昵称已保存')
    } catch (err) { setProfileMsg(err.message) } finally { setBusy(false) }
  }

  async function savePassword(e) {
    e.preventDefault()
    setPwMsg('')
    if (newPw !== newPw2) { setPwMsg('两次输入的新密码不一致'); return }
    if (newPw.length < 8) { setPwMsg('新密码至少 8 位'); return }
    setBusy(true)
    try {
      const res = await userAPI.changePassword(oldPw, newPw)
      if (res.relogin_required) {
        setPwMsg('✓ 密码已修改，请用新密码重新登录')
        setTimeout(() => { window.dispatchEvent(new Event('user-session-expired')) }, 1200)
      }
    } catch (err) { setPwMsg(err.message) } finally { setBusy(false) }
  }

  return (
    <div>
      <h1>个人资料</h1>
      <p className="muted">管理你的账号信息与登录密码。</p>

      <div className="card">
        <h2>基本信息</h2>
        <form onSubmit={saveProfile} className="form">
          <label>邮箱（登录账号，不可修改）</label>
          <input value={me?.email || ''} readOnly disabled />
          <label>昵称</label>
          <input value={nickname} onChange={(e) => setNickname(e.target.value)} placeholder="给自己起个名字" maxLength={32} />
          <label>注册时间</label>
          <input value={me?.created_at || ''} readOnly disabled />
          <label>邮箱状态</label>
          <input value={me?.email_verified ? '已验证' : '未验证'} readOnly disabled />
          <button type="submit" disabled={busy} style={{ alignSelf: 'flex-start' }}>保存昵称</button>
          {profileMsg && <p className={profileMsg.startsWith('✓') ? 'ok-msg' : 'error'}>{profileMsg}</p>}
        </form>
      </div>

      <div className="card">
        <h2>修改密码</h2>
        <p className="muted">修改成功后，当前登录状态会失效，需要用新密码重新登录。</p>
        <form onSubmit={savePassword} className="form">
          <label>当前密码</label>
          <input type="password" value={oldPw} onChange={(e) => setOldPw(e.target.value)} required autoComplete="current-password" />
          <label>新密码（8-128 位）</label>
          <input type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} required minLength={8} autoComplete="new-password" />
          <label>确认新密码</label>
          <input type="password" value={newPw2} onChange={(e) => setNewPw2(e.target.value)} required minLength={8} autoComplete="new-password" />
          <button type="submit" disabled={busy} style={{ alignSelf: 'flex-start' }}>修改密码</button>
          {pwMsg && <p className={pwMsg.startsWith('✓') ? 'ok-msg' : 'error'}>{pwMsg}</p>}
        </form>
      </div>
    </div>
  )
}
