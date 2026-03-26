import { useEffect } from 'react'
import { useServerStore } from '../../stores/serverStore'
import { useWebSocket } from '../../hooks/useWebSocket'
import { ServerCard } from '../../components/ServerCard'

export default function Dashboard() {
  const { servers, metrics, loading, fetchDashboard } = useServerStore()
  useWebSocket()

  useEffect(() => {
    fetchDashboard()
  }, [fetchDashboard])

  const onlineCount = servers.filter((s) => s.status === 'online').length

  if (loading && servers.length === 0) {
    return <div style={{ color: 'var(--text-secondary)' }}>加载中...</div>
  }

  return (
    <div>
      {/* 顶部状态栏 */}
      <div
        className="rounded-xl p-4 mb-6 flex flex-wrap gap-6"
        style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}
      >
        <div>
          <div className="text-xs" style={{ color: 'var(--text-secondary)' }}>服务器</div>
          <div className="text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>
            {onlineCount}/{servers.length}
            <span className="text-xs ml-1" style={{ color: 'var(--text-secondary)' }}>在线</span>
          </div>
        </div>
        <div>
          <div className="text-xs" style={{ color: 'var(--text-secondary)' }}>容器</div>
          <div className="text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>
            {Object.values(metrics).reduce(
              (sum, m) => sum + (m.containers?.filter((c) => c.state === 'running').length ?? 0),
              0
            )}
            <span className="text-xs ml-1" style={{ color: 'var(--text-secondary)' }}>运行中</span>
          </div>
        </div>
      </div>

      {/* 服务器卡片网格 */}
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        {servers.map((server) => (
          <ServerCard key={server.host_id} server={server} metrics={metrics[server.host_id]} />
        ))}
      </div>

      {servers.length === 0 && (
        <div className="text-center py-12" style={{ color: 'var(--text-secondary)' }}>
          暂无服务器数据，请部署 Agent 后等待上报
        </div>
      )}
    </div>
  )
}
