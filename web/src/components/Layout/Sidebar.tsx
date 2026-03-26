import { NavLink, useLocation } from 'react-router-dom'
import { useEffect } from 'react'

const links = [
  { to: '/', label: '仪表盘', icon: 'dashboard' },
  { to: '/servers', label: '服务器', icon: 'dns' },
  { to: '/databases', label: '数据库', icon: 'database' },
  { to: '/probes', label: '端口监控', icon: 'sensors' },
  { to: '/assets', label: '本地业务', icon: 'inventory_2' },
  { to: '/alerts', label: '告警中心', icon: 'notifications_active' },
  { to: '/billing', label: '资源到期', icon: 'event_upcoming' },
  { to: '/settings', label: '系统信息', icon: 'settings' },
]

export function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const location = useLocation()

  useEffect(() => { onClose() }, [location.pathname])

  return (
    <>
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm md:hidden"
          onClick={onClose}
        />
      )}

      <aside
        className={`fixed left-0 top-0 h-full w-64 z-40 bg-surface-container-lowest/95 backdrop-blur-lg flex flex-col pt-20 pb-6 shadow-2xl transition-transform duration-300 ${
          open ? 'translate-x-0' : '-translate-x-full'
        } md:translate-x-0`}
      >
        <nav className="flex-1 px-3 space-y-1">
          {links.map((link) => (
            <NavLink
              key={link.to}
              to={link.to}
              end={link.to === '/'}
              className={({ isActive }) =>
                `flex items-center gap-3 px-4 py-3 rounded-lg font-body text-sm font-medium transition-colors duration-300 ${
                  isActive
                    ? 'bg-primary/20 text-primary shadow-[0_0_12px_rgba(96,165,250,0.25)]'
                    : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'
                }`
              }
            >
              <span className="material-symbols-outlined text-xl">{link.icon}</span>
              <span>{link.label}</span>
            </NavLink>
          ))}
        </nav>
      </aside>
    </>
  )
}
