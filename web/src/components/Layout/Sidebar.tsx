import { NavLink, useLocation } from 'react-router-dom'
import { useEffect, useMemo } from 'react'
import { useSettingsStore } from '../../stores/settingsStore'
import { useAuthStore } from '../../stores/authStore'

const baseLinks = [
  { to: '/', label: '仪表盘', icon: 'dashboard' },
  { to: '/servers', label: '服务器', icon: 'dns' },
  { to: '/nas', label: 'NAS 存储', icon: 'hard_drive' },
  { to: '/databases', label: '数据库', icon: 'database' },
  { to: '/containers', label: '容器管理', icon: 'deployed_code' },
  { to: '/probes', label: '端口监控', icon: 'sensors' },
  { to: '/assets', label: '本地业务', icon: 'inventory_2' },
  { to: '/alerts', label: '告警中心', icon: 'notifications_active' },
  { to: '/logs', label: '日志中心', icon: 'article' },
  { to: '/billing', label: '资源到期', icon: 'event_upcoming' },
  { to: '/settings', label: '系统信息', icon: 'settings' },
]

const adminLinks = [
  { to: '/users', label: '��户管理', icon: 'group' },
]

export function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const location = useLocation()
  const { platformName, logoUrl } = useSettingsStore()
  const role = useAuthStore((s) => s.role)
  const links = useMemo(() => [
    ...baseLinks,
    ...(role === 'admin' ? adminLinks : []),
  ], [role])

  useEffect(() => { onClose() }, [location.pathname])

  return (
    <>
      {/* Mobile overlay */}
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/50 md:hidden"
          onClick={onClose}
        />
      )}

      <aside
        className={`fixed left-0 top-0 h-full w-[250px] z-40 flex flex-col bg-[#212529] transition-transform duration-300 ${
          open ? 'translate-x-0' : '-translate-x-full'
        } md:translate-x-0`}
      >
        {/* Logo area */}
        <div className="flex items-center gap-2 px-5 h-[70px] border-b border-white/10 shrink-0">
          <img src={logoUrl} alt="Logo" className="w-7 h-7" />
          <span className="text-white text-sm font-semibold tracking-wide">{platformName}</span>
        </div>

        {/* Menu section */}
        <nav className="flex-1 px-4 py-4 overflow-y-auto">
          <p className="text-[#838fb9] text-[10px] font-semibold uppercase tracking-widest px-3 mb-2">
            导航菜单
          </p>
          <ul className="space-y-0.5">
            {links.map((link) => (
              <li key={link.to}>
                <NavLink
                  to={link.to}
                  end={link.to === '/'}
                  className={({ isActive }) =>
                    `flex items-center gap-3 px-3 py-2.5 rounded-md text-sm font-medium transition-colors duration-200 ${
                      isActive
                        ? 'text-[#2ca07a]'
                        : 'text-[#adb5bd] hover:text-white'
                    }`
                  }
                >
                  {({ isActive }) => (
                    <>
                      <span
                        className={`material-symbols-outlined text-xl ${
                          isActive ? 'text-[#2ca07a]' : ''
                        }`}
                      >
                        {link.icon}
                      </span>
                      <span>{link.label}</span>
                    </>
                  )}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>
      </aside>
    </>
  )
}
