import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { useState, useRef, useEffect, useMemo } from 'react'
import { Sidebar } from './Sidebar'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useAuthStore } from '../../stores/authStore'
import { useServerStore } from '../../stores/serverStore'
import { useNasStore } from '../../stores/nasStore'
import { useSettingsStore } from '../../stores/settingsStore'
import { getDatabases, type RDSInfo } from '../../api/client'
import { NotificationBell } from '../NotificationBell'
// import { ChatButton } from '../AIChat/ChatButton'

interface SearchResult {
  icon: string
  label: string
  sub: string
  href: string
}

export function MainLayout() {
  useWebSocket()
  const fetchSettings = useSettingsStore((s) => s.fetchSettings)
  const checkAuth = useAuthStore((s) => s.checkAuth)
  useEffect(() => { fetchSettings() }, [fetchSettings])
  useEffect(() => { checkAuth() }, [checkAuth])
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchFocused, setSearchFocused] = useState(false)
  const { username, logout } = useAuthStore()
  const { servers } = useServerStore()
  const { devices: nasDevices } = useNasStore()
  const [databases, setDatabases] = useState<RDSInfo[]>([])
  const navigate = useNavigate()
  const location = useLocation()
  const menuRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLDivElement>(null)

  // 判断当前页面搜索类型
  const searchMode = useMemo(() => {
    const p = location.pathname
    if (p === '/servers' || p.startsWith('/servers/')) return 'server'
    if (p === '/nas' || p.startsWith('/nas/')) return 'nas'
    if (p === '/databases' || p.startsWith('/databases/')) return 'database'
    return null
  }, [location.pathname])

  // 切换页面时清空搜索
  useEffect(() => { setSearchQuery(''); setSearchFocused(false) }, [location.pathname])

  // 懒加载数据库列表（仅在数据库页面）
  useEffect(() => {
    if (searchMode === 'database' && databases.length === 0) {
      getDatabases().then(setDatabases).catch(() => {})
    }
  }, [searchMode])

  // 点击外部关闭下拉菜单
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setUserMenuOpen(false)
      }
      if (searchRef.current && !searchRef.current.contains(e.target as Node)) {
        setSearchFocused(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  // 搜索结果（按页面类型匹配不同数据源）
  const searchResults = useMemo<SearchResult[]>(() => {
    const q = searchQuery.trim().toLowerCase()
    if (!q || !searchMode) return []
    const results: SearchResult[] = []

    if (searchMode === 'server') {
      for (const s of servers) {
        const name = (s.display_name || s.hostname).toLowerCase()
        let ip = ''
        try { ip = JSON.parse(s.ip_addresses || '[]')[0] || '' } catch { /* */ }
        if (name.includes(q) || ip.includes(q) || s.host_id.toLowerCase().includes(q)) {
          results.push({ icon: 'dns', label: s.display_name || s.hostname, sub: ip || s.host_id, href: `/servers/${s.host_id}` })
        }
        if (results.length >= 8) break
      }
    } else if (searchMode === 'nas') {
      for (const d of nasDevices) {
        const name = d.name.toLowerCase()
        const host = d.host.toLowerCase()
        if (name.includes(q) || host.includes(q)) {
          results.push({ icon: 'hard_drive', label: d.name, sub: `${d.host}:${d.port}`, href: `/nas/${d.id}` })
        }
        if (results.length >= 8) break
      }
    } else if (searchMode === 'database') {
      for (const db of databases) {
        const name = db.name.toLowerCase()
        const hostId = db.host_id.toLowerCase()
        if (name.includes(q) || hostId.includes(q) || db.engine.toLowerCase().includes(q)) {
          results.push({ icon: 'database', label: db.name, sub: `${db.engine} · ${db.host_id}`, href: `/databases/${db.host_id}` })
        }
        if (results.length >= 8) break
      }
    }

    return results
  }, [searchQuery, searchMode, servers, nasDevices, databases])

  const handleLogout = () => {
    setUserMenuOpen(false)
    logout()
    navigate('/login', { replace: true })
  }

  const handleSearchSelect = (href: string) => {
    setSearchQuery('')
    setSearchFocused(false)
    navigate(href)
  }

  return (
    <div className="min-h-screen bg-[#f3f3f9]">
      {/* Top Navigation Bar */}
      <header className="fixed top-0 left-0 md:left-[250px] right-0 z-30 bg-white h-[70px] shadow-sm flex items-center justify-between px-6">
        {/* Left: mobile menu toggle + search */}
        <div className="flex items-center gap-4">
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="md:hidden p-2 rounded-lg hover:bg-gray-100 transition-colors text-gray-500"
          >
            <span className="material-symbols-outlined text-xl">
              {sidebarOpen ? 'close' : 'menu'}
            </span>
          </button>

          {/* Search — 仅在服务器/NAS/数据库页面显示 */}
          {searchMode && <div ref={searchRef} className="relative hidden md:block">
            <span className="material-symbols-outlined text-gray-400 text-base absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none">
              search
            </span>
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onFocus={() => setSearchFocused(true)}
              placeholder={searchMode === 'server' ? '搜索服务器...' : searchMode === 'nas' ? '搜索 NAS 设备...' : '搜索数据库...'}
              className="bg-[#f3f3f9] text-sm text-gray-600 placeholder-gray-400 rounded-full pl-9 pr-4 py-2 w-60 outline-none focus:ring-2 focus:ring-[#2ca07a]/30 transition"
            />
            {/* Search results dropdown */}
            {searchFocused && searchQuery.trim() && (
              <div className="absolute top-full left-0 mt-2 w-80 bg-white rounded-[10px] shadow-lg border border-gray-100 overflow-hidden z-50">
                {searchResults.length > 0 ? (
                  <div className="max-h-80 overflow-y-auto">
                    {searchResults.map((r, i) => (
                      <button
                        key={`${r.href}-${i}`}
                        onClick={() => handleSearchSelect(r.href)}
                        className="w-full flex items-center gap-3 px-4 py-3 hover:bg-[#f8f9fa] transition-colors text-left"
                      >
                        <div className="w-8 h-8 rounded-full bg-[#2ca07a]/10 flex items-center justify-center flex-shrink-0">
                          <span className="material-symbols-outlined text-[#2ca07a] text-base">{r.icon}</span>
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="text-sm font-medium text-[#495057] truncate">{r.label}</div>
                          <div className="text-[11px] text-[#878a99] font-mono truncate">{r.sub}</div>
                        </div>
                        <span className="material-symbols-outlined text-gray-300 text-base flex-shrink-0">chevron_right</span>
                      </button>
                    ))}
                  </div>
                ) : (
                  <div className="px-4 py-6 text-center text-sm text-[#878a99]">
                    未找到匹配结果
                  </div>
                )}
              </div>
            )}
          </div>}
        </div>

        {/* Right: actions */}
        <div className="flex items-center gap-1 text-gray-500">
          {/* Sync button */}
          <button className="p-2 rounded-lg hover:bg-gray-100 hover:text-[#2ca07a] transition-all active:scale-95 duration-200">
            <span className="material-symbols-outlined text-xl">sync</span>
          </button>

          {/* Notification bell */}
          <NotificationBell />

          {/* Divider */}
          <div className="w-px h-6 bg-gray-200 mx-1" />

          {/* User avatar dropdown */}
          <div ref={menuRef} className="relative">
            <button
              onClick={() => setUserMenuOpen(!userMenuOpen)}
              className="flex items-center gap-2 pl-1 pr-2 py-1 rounded-lg hover:bg-gray-100 transition-all active:scale-95 duration-200"
            >
              <img
                src={`https://ui-avatars.com/api/?name=${encodeURIComponent(username ?? 'User')}&size=32&background=2ca07a&color=fff&rounded=true`}
                alt={username ?? 'User'}
                className="w-8 h-8 rounded-full object-cover"
              />
              <span className="hidden md:inline text-sm font-medium text-gray-700">{username}</span>
              <span className="material-symbols-outlined text-base text-gray-400">expand_more</span>
            </button>

            {userMenuOpen && (
              <div className="absolute right-0 top-full mt-2 w-48 bg-white rounded-xl shadow-lg border border-gray-100 overflow-hidden z-50">
                <div className="px-4 py-3 border-b border-gray-100">
                  <p className="text-sm font-medium text-gray-800">{username}</p>
                  <p className="text-[11px] text-gray-400">管理员</p>
                </div>
                <button
                  onClick={handleLogout}
                  className="w-full flex items-center gap-2 px-4 py-3 text-sm text-gray-500 hover:bg-gray-50 hover:text-red-500 transition-colors"
                >
                  <span className="material-symbols-outlined text-base">logout</span>
                  退出登录
                </button>
              </div>
            )}
          </div>
        </div>
      </header>

      <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />

      {/* Main content */}
      <main className="ml-0 md:ml-[250px] pt-[94px] px-6 pb-12 min-h-screen bg-[#f3f3f9]">
        <Outlet />
      </main>

      {/* <ChatButton /> */}
    </div>
  )
}
