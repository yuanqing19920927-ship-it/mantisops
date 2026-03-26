import { Outlet, useNavigate } from 'react-router-dom'
import { useState, useRef, useEffect } from 'react'
import { Sidebar } from './Sidebar'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useAuthStore } from '../../stores/authStore'
import { useThemeStore } from '../../stores/themeStore'
import { NotificationBell } from '../NotificationBell'

export function MainLayout() {
  useWebSocket()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const { username, logout } = useAuthStore()
  const { theme, toggle: toggleTheme } = useThemeStore()
  const navigate = useNavigate()
  const menuRef = useRef<HTMLDivElement>(null)

  // 点击外部关闭下拉菜单
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setUserMenuOpen(false)
      }
    }
    if (userMenuOpen) document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [userMenuOpen])

  const handleLogout = () => {
    setUserMenuOpen(false)
    logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className="min-h-screen bg-surface-dim">
      {/* Top Navigation Bar */}
      <header className="fixed top-0 w-full z-50 bg-surface-container/80 backdrop-blur-xl flex justify-between items-center px-4 md:px-6 h-16 shadow-[0_4px_24px_0_rgba(0,0,0,0.15)]">
        <div className="flex items-center gap-3">
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="md:hidden p-2 rounded-lg hover:bg-surface-container-high transition-colors text-on-surface-variant"
          >
            <span className="material-symbols-outlined text-xl">
              {sidebarOpen ? 'close' : 'menu'}
            </span>
          </button>
          <span className="text-2xl font-bold bg-gradient-to-br from-primary to-primary-container bg-clip-text text-transparent font-headline tracking-tight">
            OpsBoard
          </span>
        </div>
        <div className="flex items-center gap-2 md:gap-4 text-on-surface-variant">
          <button className="p-2 rounded-lg hover:bg-surface-container-high hover:text-primary transition-all active:scale-95 duration-200">
            <span className="material-symbols-outlined text-xl">sync</span>
          </button>
          <NotificationBell />
          <button
            onClick={toggleTheme}
            className="p-2 rounded-lg hover:bg-surface-container-high hover:text-primary transition-all active:scale-95 duration-200"
            title={theme === 'dark' ? '切换浅色' : '切换深色'}
          >
            <span className="material-symbols-outlined text-xl">
              {theme === 'dark' ? 'light_mode' : 'dark_mode'}
            </span>
          </button>

          {/* 用户菜单 */}
          <div ref={menuRef} className="relative">
            <button
              onClick={() => setUserMenuOpen(!userMenuOpen)}
              className="flex items-center gap-2 p-2 rounded-lg hover:bg-surface-container-high transition-all active:scale-95 duration-200"
            >
              <span className="material-symbols-outlined text-xl">account_circle</span>
              <span className="hidden md:inline text-xs font-medium text-on-surface">{username}</span>
            </button>

            {userMenuOpen && (
              <div className="absolute right-0 top-full mt-2 w-48 bg-surface-container rounded-xl shadow-2xl border border-outline-variant/10 overflow-hidden">
                <div className="px-4 py-3 border-b border-outline-variant/10">
                  <p className="text-sm font-medium text-on-surface">{username}</p>
                  <p className="text-[10px] text-on-surface-variant">管理员</p>
                </div>
                <button
                  onClick={handleLogout}
                  className="w-full flex items-center gap-2 px-4 py-3 text-xs text-on-surface-variant hover:bg-surface-container-high hover:text-error transition-colors"
                >
                  <span className="material-symbols-outlined text-sm">logout</span>
                  退出登录
                </button>
              </div>
            )}
          </div>
        </div>
      </header>

      <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />

      <main className="pt-24 px-4 md:px-8 pb-12 md:ml-64 min-h-screen bg-grid-pattern">
        <Outlet />
      </main>
    </div>
  )
}
