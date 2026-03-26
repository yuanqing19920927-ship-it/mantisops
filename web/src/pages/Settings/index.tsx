import { useServerStore } from '../../stores/serverStore'
import { useThemeStore } from '../../stores/themeStore'
import { useEffect } from 'react'
import { StatusBadge } from '../../components/StatusBadge'

function timeSince(unix: number): string {
  const diff = Math.floor(Date.now() / 1000) - unix
  if (diff < 60) return `${diff}秒前`
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`
  return `${Math.floor(diff / 86400)}天前`
}

export default function Settings() {
  const { servers, fetchDashboard } = useServerStore()
  const { theme, toggle } = useThemeStore()

  useEffect(() => { fetchDashboard() }, [fetchDashboard])

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6" style={{ color: 'var(--text-primary)' }}>设置</h1>

      {/* 主题 */}
      <div className="rounded-xl p-5 mb-6" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
        <h2 className="text-lg font-semibold mb-3" style={{ color: 'var(--text-primary)' }}>外观</h2>
        <div className="flex items-center gap-4">
          <span style={{ color: 'var(--text-secondary)' }}>主题</span>
          <button onClick={toggle}
            className="px-4 py-2 rounded-lg text-sm"
            style={{ backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border)' }}>
            当前: {theme === 'dark' ? '深色' : '浅色'} — 点击切换
          </button>
        </div>
      </div>

      {/* Agent 列表 */}
      <div className="rounded-xl p-5" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
        <h2 className="text-lg font-semibold mb-3" style={{ color: 'var(--text-primary)' }}>已注册 Agent ({servers.length})</h2>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--text-secondary)' }}>
                <th className="text-left p-3">主机名</th>
                <th className="text-left p-3">Host ID</th>
                <th className="text-left p-3">Agent 版本</th>
                <th className="text-left p-3">最后心跳</th>
                <th className="text-center p-3">状态</th>
              </tr>
            </thead>
            <tbody>
              {servers.map((s) => (
                <tr key={s.host_id} style={{ borderBottom: '1px solid var(--border)' }}>
                  <td className="p-3" style={{ color: 'var(--text-primary)' }}>{s.hostname}</td>
                  <td className="p-3 text-xs font-mono" style={{ color: 'var(--text-secondary)' }}>{s.host_id}</td>
                  <td className="p-3" style={{ color: 'var(--text-secondary)' }}>{s.agent_version || '-'}</td>
                  <td className="p-3" style={{ color: 'var(--text-secondary)' }}>{s.last_seen ? timeSince(s.last_seen) : '-'}</td>
                  <td className="text-center p-3"><StatusBadge status={s.status} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
