import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '../../stores/authStore'

export default function ChangePassword() {
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [confirmPwd, setConfirmPwd] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const { changePassword, mustChangePwd } = useAuthStore()
  const navigate = useNavigate()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (newPwd !== confirmPwd) {
      setError('两次输入的新密码不一致')
      return
    }
    if (newPwd.length < 6) {
      setError('新密码长度不能少于6位')
      return
    }
    setLoading(true)
    try {
      await changePassword(oldPwd, newPwd)
      navigate('/', { replace: true })
    } catch (err) {
      setError((err as Error).message || '修改密码失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-[#111827] to-[#1f2937] flex items-center justify-center p-4">
      <form onSubmit={handleSubmit} className="w-full max-w-[450px] bg-white/95 backdrop-blur-sm rounded-[12px] shadow-2xl p-10">
        <div className="flex flex-col items-center mb-8">
          <div className="w-14 h-14 rounded-full bg-[#f7b84b]/15 flex items-center justify-center mb-3">
            <span className="material-symbols-outlined text-[#f7b84b] text-[28px]">lock_reset</span>
          </div>
          <h1 className="text-xl font-bold text-[#495057]">修改密码</h1>
          {mustChangePwd && (
            <p className="text-sm text-[#878a99] mt-2 text-center">
              管理员已为您设置初始密码，请输入初始密码后设置新密码
            </p>
          )}
        </div>

        {error && (
          <div className="mb-5 px-4 py-3 rounded-[8px] bg-[#f06548]/10 border border-[#f06548]/20 text-[#f06548] text-sm text-center">
            {error}
          </div>
        )}

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-[#495057] mb-1.5">
              {mustChangePwd ? '初始密码' : '当前密码'}
            </label>
            <input
              type="password"
              value={oldPwd}
              onChange={(e) => setOldPwd(e.target.value)}
              className="w-full border border-[#e9ebec] rounded-[8px] px-4 py-2.5 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
              placeholder={mustChangePwd ? '请输入管理员设置的初始密码' : '请输入当前密码'}
              autoFocus
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-[#495057] mb-1.5">新密码</label>
            <input
              type="password"
              value={newPwd}
              onChange={(e) => setNewPwd(e.target.value)}
              className="w-full border border-[#e9ebec] rounded-[8px] px-4 py-2.5 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
              placeholder="请输入新密码（至少6位）"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-[#495057] mb-1.5">确认新密码</label>
            <input
              type="password"
              value={confirmPwd}
              onChange={(e) => setConfirmPwd(e.target.value)}
              className="w-full border border-[#e9ebec] rounded-[8px] px-4 py-2.5 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
              placeholder="请再次输入新密码"
            />
          </div>
          <button
            type="submit"
            disabled={loading || !oldPwd || !newPwd || !confirmPwd}
            className="w-full bg-gradient-to-r from-[#2ca07a] to-[#0ab39c] hover:from-[#259b73] hover:to-[#099d88] text-white py-2.5 rounded-[8px] font-semibold text-sm shadow-[0_4px_12px_rgba(44,160,122,0.3)] active:scale-[0.98] transition-all disabled:opacity-50 disabled:cursor-not-allowed mt-2"
          >
            {loading ? '保存中...' : '确认修改'}
          </button>
        </div>
      </form>
    </div>
  )
}
