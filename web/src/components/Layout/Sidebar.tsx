import { NavLink } from 'react-router-dom'
import { ThemeToggle } from '../ThemeToggle'

const links = [
  { to: '/', label: '仪表盘' },
  { to: '/servers', label: '服务器' },
  { to: '/probes', label: '端口监控' },
  { to: '/assets', label: '资产信息' },
  { to: '/settings', label: '设置' },
]

export function Sidebar() {
  return (
    <aside
      className="w-56 h-screen flex flex-col"
      style={{
        borderRight: '1px solid var(--border)',
        backgroundColor: 'var(--bg-card)',
      }}
    >
      <div className="p-4 text-xl font-bold" style={{ color: 'var(--accent)' }}>
        OpsBoard
      </div>
      <nav className="flex-1 px-2">
        {links.map((link) => (
          <NavLink
            key={link.to}
            to={link.to}
            end={link.to === '/'}
            className="flex items-center gap-3 px-3 py-2 rounded-lg mb-1 text-sm transition-colors"
            style={({ isActive }) => ({
              backgroundColor: isActive ? 'var(--accent)' : 'transparent',
              color: isActive ? '#ffffff' : 'var(--text-secondary)',
            })}
          >
            <span>{link.label}</span>
          </NavLink>
        ))}
      </nav>
      <div className="p-4" style={{ borderTop: '1px solid var(--border)' }}>
        <ThemeToggle />
      </div>
    </aside>
  )
}
