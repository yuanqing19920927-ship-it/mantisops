import { useEffect, useState } from 'react'
import { useServerStore } from '../../stores/serverStore'
import { useWebSocket } from '../../hooks/useWebSocket'
import { ServerCard } from '../../components/ServerCard'
import { StatusBadge } from '../../components/StatusBadge'
import { Link } from 'react-router-dom'

export default function Servers() {
  const { servers, metrics, fetchDashboard } = useServerStore()
  const [view, setView] = useState<'card' | 'table'>('card')
  useWebSocket()

  useEffect(() => { fetchDashboard() }, [fetchDashboard])

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>
          服务器列表 ({servers.length})
        </h1>
        <div className="flex gap-2">
          <button onClick={() => setView('card')}
            className="px-3 py-1 rounded text-sm"
            style={{ backgroundColor: view === 'card' ? 'var(--accent)' : 'var(--bg-secondary)', color: view === 'card' ? '#fff' : 'var(--text-secondary)' }}>
            卡片
          </button>
          <button onClick={() => setView('table')}
            className="px-3 py-1 rounded text-sm"
            style={{ backgroundColor: view === 'table' ? 'var(--accent)' : 'var(--bg-secondary)', color: view === 'table' ? '#fff' : 'var(--text-secondary)' }}>
            表格
          </button>
        </div>
      </div>

      {view === 'card' ? (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {servers.map((s) => (
            <ServerCard key={s.host_id} server={s} metrics={metrics[s.host_id]} />
          ))}
        </div>
      ) : (
        <div className="rounded-xl overflow-hidden" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
          <table className="w-full text-sm">
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--text-secondary)' }}>
                <th className="text-left p-3">主机名</th>
                <th className="text-left p-3">IP</th>
                <th className="text-left p-3">OS</th>
                <th className="text-center p-3">CPU</th>
                <th className="text-center p-3">内存</th>
                <th className="text-center p-3">状态</th>
              </tr>
            </thead>
            <tbody>
              {servers.map((s) => {
                const m = metrics[s.host_id]
                return (
                  <tr key={s.host_id} style={{ borderBottom: '1px solid var(--border)' }}>
                    <td className="p-3">
                      <Link to={`/servers/${s.host_id}`} style={{ color: 'var(--accent)' }}>
                        {s.display_name || s.hostname}
                      </Link>
                    </td>
                    <td className="p-3" style={{ color: 'var(--text-secondary)' }}>
                      {(() => { try { return JSON.parse(s.ip_addresses)[0] } catch { return '-' } })()}
                    </td>
                    <td className="p-3" style={{ color: 'var(--text-secondary)' }}>{s.os}</td>
                    <td className="text-center p-3" style={{ color: 'var(--text-primary)' }}>
                      {m?.cpu?.usage_percent?.toFixed(1) ?? '-'}%
                    </td>
                    <td className="text-center p-3" style={{ color: 'var(--text-primary)' }}>
                      {m?.memory?.usage_percent?.toFixed(1) ?? '-'}%
                    </td>
                    <td className="text-center p-3"><StatusBadge status={s.status} /></td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
