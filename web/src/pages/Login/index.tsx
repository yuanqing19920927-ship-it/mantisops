import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '../../stores/authStore'
import { useSettingsStore } from '../../stores/settingsStore'

export default function Login() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const login = useAuthStore((s) => s.login)
  const navigate = useNavigate()
  const { platformName, platformSubtitle, logoUrl } = useSettingsStore()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await login(username, password)
      navigate('/', { replace: true })
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-[#111827] to-[#1f2937] flex items-center justify-center p-4">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-[450px] bg-white/95 backdrop-blur-sm rounded-[12px] shadow-2xl p-10"
      >
        {/* Logo */}
        <div className="flex flex-col items-center mb-8">
          <div className="w-14 h-14 rounded-full bg-[#2ca07a]/15 flex items-center justify-center mb-3 overflow-hidden">
            <img src={logoUrl} alt="Logo" className="w-8 h-8 object-contain" />
          </div>
          <h1 className="text-xl font-bold text-[#495057]">{platformName}</h1>
          <p className="text-sm text-[#878a99] mt-1">{platformSubtitle}</p>
        </div>

        {error && (
          <div className="mb-5 px-4 py-3 rounded-[8px] bg-[#f06548]/10 border border-[#f06548]/20 text-[#f06548] text-sm text-center">
            {error}
          </div>
        )}

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-[#495057] mb-1.5">
              用户名
            </label>
            <div className="relative">
              <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-[#878a99] text-[18px]">
                person
              </span>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full border border-[#e9ebec] rounded-[8px] pl-10 pr-4 py-2.5 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
                placeholder="请输入用户名"
                autoFocus
              />
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-[#495057] mb-1.5">
              密码
            </label>
            <div className="relative">
              <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-[#878a99] text-[18px]">
                lock
              </span>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full border border-[#e9ebec] rounded-[8px] pl-10 pr-4 py-2.5 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
                placeholder="请输入密码"
              />
            </div>
          </div>

          <button
            type="submit"
            disabled={loading || !username || !password}
            className="w-full bg-gradient-to-r from-[#2ca07a] to-[#0ab39c] hover:from-[#259b73] hover:to-[#099d88] text-white py-2.5 rounded-[8px] font-semibold text-sm shadow-[0_4px_12px_rgba(44,160,122,0.3)] hover:shadow-[0_4px_16px_rgba(44,160,122,0.4)] active:scale-[0.98] transition-all disabled:opacity-50 disabled:cursor-not-allowed mt-2"
          >
            {loading ? '登录中...' : '登 录'}
          </button>
        </div>
      </form>
    </div>
  )
}
