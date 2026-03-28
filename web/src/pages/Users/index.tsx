import { useEffect, useState, useCallback } from 'react'
import { useAuthStore } from '../../stores/authStore'
import { getUsers, createUser, updateUser, deleteUser, resetPassword, type UserInfo } from '../../api/users'

const ROLE_LABELS: Record<string, string> = { admin: '管理员', operator: '运维员', viewer: '观察者' }
const ROLE_COLORS: Record<string, { bg: string; text: string }> = {
  admin: { bg: 'rgba(44,160,122,0.1)', text: '#2ca07a' },
  operator: { bg: 'rgba(64,133,219,0.1)', text: '#4085db' },
  viewer: { bg: 'rgba(135,138,153,0.1)', text: '#878a99' },
}

export default function Users() {
  const { role: myRole, username: myUsername } = useAuthStore()
  const [users, setUsers] = useState<UserInfo[]>([])
  const [showCreate, setShowCreate] = useState(false)
  const [editTarget, setEditTarget] = useState<UserInfo | null>(null)
  const [resetTarget, setResetTarget] = useState<UserInfo | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<UserInfo | null>(null)

  // Create form
  const [cUsername, setCUsername] = useState('')
  const [cPassword, setCPassword] = useState('')
  const [cDisplayName, setCDisplayName] = useState('')
  const [cRole, setCRole] = useState('viewer')

  // Edit form
  const [eDisplayName, setEDisplayName] = useState('')
  const [eRole, setERole] = useState('')
  const [eEnabled, setEEnabled] = useState(true)

  // Reset pwd form
  const [rPassword, setRPassword] = useState('')

  const [saving, setSaving] = useState(false)

  const fetchUsers = useCallback(() => {
    getUsers().then(setUsers).catch(console.error)
  }, [])

  useEffect(() => { fetchUsers() }, [fetchUsers])

  if (myRole !== 'admin') {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-[#878a99]">无权限访问此页面</p>
      </div>
    )
  }

  const adminCount = users.filter(u => u.role === 'admin' && u.enabled).length

  const handleCreate = async () => {
    setSaving(true)
    try {
      await createUser({ username: cUsername, password: cPassword, display_name: cDisplayName, role: cRole })
      setShowCreate(false)
      setCUsername(''); setCPassword(''); setCDisplayName(''); setCRole('viewer')
      fetchUsers()
    } catch (err) {
      alert((err as Error).message || '创建失败')
    } finally { setSaving(false) }
  }

  const handleEdit = async () => {
    if (!editTarget) return
    setSaving(true)
    try {
      await updateUser(editTarget.id, { display_name: eDisplayName, role: eRole, enabled: eEnabled })
      setEditTarget(null)
      fetchUsers()
    } catch (err) {
      alert((err as Error).message || '更新失败')
    } finally { setSaving(false) }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteUser(deleteTarget.id)
      setDeleteTarget(null)
      fetchUsers()
    } catch (err) {
      alert((err as Error).message || '删除失败')
    }
  }

  const handleReset = async () => {
    if (!resetTarget) return
    setSaving(true)
    try {
      await resetPassword(resetTarget.id, rPassword)
      setResetTarget(null)
      setRPassword('')
      fetchUsers()
    } catch (err) {
      alert((err as Error).message || '重置失败')
    } finally { setSaving(false) }
  }

  const openEdit = (u: UserInfo) => {
    setEDisplayName(u.display_name)
    setERole(u.role)
    setEEnabled(u.enabled)
    setEditTarget(u)
  }

  return (
    <div className="flex flex-col gap-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-[22px] font-semibold text-[#495057]">用户管理</h1>
          <p className="text-sm text-[#878a99] mt-1">Users</p>
        </div>
        <button onClick={() => setShowCreate(true)} className="flex items-center gap-1.5 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors">
          <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>person_add</span>
          添加用户
        </button>
      </div>

      {/* User table */}
      <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-[#f8f9fa]">
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">用户名</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">显示名</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">角色</th>
                <th className="text-center text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">状态</th>
                <th className="text-right text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">操作</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u, idx) => {
                const isSelf = u.username === myUsername
                const isLastAdmin = u.role === 'admin' && adminCount <= 1
                const rc = ROLE_COLORS[u.role] || ROLE_COLORS.viewer
                return (
                  <tr key={u.id} className={`hover:bg-[#f8f9fa] transition-colors ${idx < users.length - 1 ? 'border-b border-[#f2f4f7]' : ''}`}>
                    <td className="px-5 py-3.5">
                      <span className="text-[#495057] font-medium">{u.username}</span>
                      {u.must_change_pwd && <span className="ml-2 text-[10px] px-1.5 py-0.5 rounded bg-[#f7b84b]/10 text-[#f7b84b]">待改密</span>}
                    </td>
                    <td className="px-5 py-3.5 text-[#878a99]">{u.display_name || '-'}</td>
                    <td className="px-5 py-3.5">
                      <span className="text-[11px] px-2 py-0.5 rounded font-medium" style={{ backgroundColor: rc.bg, color: rc.text }}>
                        {ROLE_LABELS[u.role] || u.role}
                      </span>
                    </td>
                    <td className="px-5 py-3.5 text-center">
                      <span className={`inline-block w-2 h-2 rounded-full ${u.enabled ? 'bg-[#0ab39c]' : 'bg-[#878a99]'}`} />
                      <span className="ml-1.5 text-[12px] text-[#878a99]">{u.enabled ? '启用' : '禁用'}</span>
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex items-center justify-end gap-2">
                        {!isSelf && (
                          <>
                            <button onClick={() => openEdit(u)} className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded transition-colors">
                              编辑
                            </button>
                            <button onClick={() => { setResetTarget(u); setRPassword('') }} className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#f7b84b] hover:text-[#f7b84b] rounded transition-colors">
                              重置密码
                            </button>
                            {!(isLastAdmin) && (
                              <button onClick={() => setDeleteTarget(u)} className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#f06548] hover:text-[#f06548] rounded transition-colors">
                                删除
                              </button>
                            )}
                          </>
                        )}
                        {isSelf && <span className="text-[11px] text-[#adb5bd]">当前用户</span>}
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
          {users.length === 0 && (
            <div className="py-12 text-center">
              <span className="material-symbols-outlined text-3xl text-[#ced4da] mb-2 block">group_off</span>
              <p className="text-[#878a99] text-sm">暂无用户</p>
            </div>
          )}
        </div>
      </div>

      {/* Create dialog */}
      {showCreate && (
        <Dialog title="添加用户" onClose={() => setShowCreate(false)}>
          <div className="space-y-3">
            <Input label="用户名" value={cUsername} onChange={setCUsername} placeholder="英文用户名" />
            <Input label="初始密码" value={cPassword} onChange={setCPassword} placeholder="用户首次登录需修改" type="password" />
            <Input label="显示名" value={cDisplayName} onChange={setCDisplayName} placeholder="如：张三" />
            <div>
              <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">角色</label>
              <select value={cRole} onChange={e => setCRole(e.target.value)} className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] bg-white">
                <option value="viewer">观察者</option>
                <option value="operator">运维员</option>
                <option value="admin">管理员</option>
              </select>
            </div>
            <button onClick={handleCreate} disabled={saving || !cUsername || !cPassword} className="w-full mt-2 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors disabled:opacity-50">
              {saving ? '创建中...' : '创建'}
            </button>
          </div>
        </Dialog>
      )}

      {/* Edit dialog */}
      {editTarget && (
        <Dialog title={`编辑用户 — ${editTarget.username}`} onClose={() => setEditTarget(null)}>
          <div className="space-y-3">
            <Input label="显示名" value={eDisplayName} onChange={setEDisplayName} />
            <div>
              <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">角色</label>
              <select value={eRole} onChange={e => setERole(e.target.value)} className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] bg-white">
                <option value="viewer">观察者</option>
                <option value="operator">运维员</option>
                <option value="admin">管理员</option>
              </select>
            </div>
            <div className="flex items-center gap-2">
              <label className="text-[12px] font-medium text-[#878a99]">启用</label>
              <input type="checkbox" checked={eEnabled} onChange={e => setEEnabled(e.target.checked)} className="accent-[#2ca07a]" />
            </div>
            <button onClick={handleEdit} disabled={saving} className="w-full mt-2 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors disabled:opacity-50">
              {saving ? '保存中...' : '保存'}
            </button>
          </div>
        </Dialog>
      )}

      {/* Reset password dialog */}
      {resetTarget && (
        <Dialog title={`重置密码 — ${resetTarget.username}`} onClose={() => setResetTarget(null)}>
          <div className="space-y-3">
            <p className="text-[12px] text-[#878a99]">设置新初始密码，用户下次登录需修改。</p>
            <Input label="新初始密码" value={rPassword} onChange={setRPassword} type="password" placeholder="至少6位" />
            <button onClick={handleReset} disabled={saving || !rPassword} className="w-full mt-2 px-4 py-2 text-[13px] bg-[#f7b84b] hover:bg-[#e5a936] text-white rounded transition-colors disabled:opacity-50">
              {saving ? '重置中...' : '确认重置'}
            </button>
          </div>
        </Dialog>
      )}

      {/* Delete confirm dialog */}
      {deleteTarget && (
        <Dialog title={`删除用户 — ${deleteTarget.username}`} onClose={() => setDeleteTarget(null)}>
          <p className="text-sm text-[#495057] mb-4">确定要删除用户 <strong>{deleteTarget.username}</strong> 吗？此操作不可恢复。</p>
          <div className="flex justify-end gap-2">
            <button onClick={() => setDeleteTarget(null)} className="text-xs px-4 py-2 border border-[#ced4da] text-[#878a99] rounded-lg hover:bg-[#f8f9fa] transition-colors">取消</button>
            <button onClick={handleDelete} className="text-xs px-4 py-2 bg-[#f06548] text-white rounded-lg hover:bg-[#d9534f] transition-colors">确认删除</button>
          </div>
        </Dialog>
      )}
    </div>
  )
}

// --- Simple reusable components ---

function Dialog({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div className="bg-white rounded-xl shadow-xl w-[480px] max-w-[90vw] overflow-hidden" onClick={e => e.stopPropagation()}>
        <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center justify-between">
          <h3 className="text-sm font-semibold text-[#495057]">{title}</h3>
          <button onClick={onClose} className="text-[#878a99] hover:text-[#495057]">
            <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>close</span>
          </button>
        </div>
        <div className="px-5 py-4">{children}</div>
      </div>
    </div>
  )
}

function Input({ label, value, onChange, placeholder, type = 'text' }: {
  label: string; value: string; onChange: (v: string) => void; placeholder?: string; type?: string
}) {
  return (
    <div>
      <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">{label}</label>
      <input
        type={type}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
      />
    </div>
  )
}
