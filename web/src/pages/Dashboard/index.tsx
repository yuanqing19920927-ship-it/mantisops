import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useServerStore } from '../../stores/serverStore'
import { useWebSocket } from '../../hooks/useWebSocket'
import { StatusBadge } from '../../components/StatusBadge'
import { ProgressBar } from '../../components/ProgressBar'
import { getProbeStatus } from '../../api/client'
import type { ProbeResult } from '../../types'

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatBytesPS(bps: number): string {
  return formatBytes(bps) + '/s'
}

export default function Dashboard() {
  const { servers, metrics, loading, fetchDashboard } = useServerStore()
  const [probes, setProbes] = useState<ProbeResult[]>([])
  useWebSocket()

  useEffect(() => {
    fetchDashboard()
    getProbeStatus().then(setProbes).catch(() => {})
    const timer = setInterval(() => {
      getProbeStatus().then(setProbes).catch(() => {})
    }, 15000)
    return () => clearInterval(timer)
  }, [fetchDashboard])

  const onlineCount = servers.filter((s) => s.status === 'online').length
  const totalContainers = Object.values(metrics).reduce(
    (sum, m) => sum + (m.containers?.filter((c) => c.state === 'running').length ?? 0), 0
  )
  const probesUp = probes.filter((p) => p.status === 'up').length

  if (loading && servers.length === 0) {
    return <div style={{ color: 'var(--text-secondary)' }}>加载中...</div>
  }

  // 资源 Top 排行数据
  const serverMetrics = servers.map((s) => {
    const m = metrics[s.host_id]
    return {
      ...s,
      cpuPercent: m?.cpu?.usage_percent ?? 0,
      memPercent: m?.memory?.usage_percent ?? 0,
      diskPercent: m?.disks?.[0]?.usage_percent ?? 0,
      netRx: m?.networks?.reduce((sum, n) => sum + n.rx_bytes_per_sec, 0) ?? 0,
      netTx: m?.networks?.reduce((sum, n) => sum + n.tx_bytes_per_sec, 0) ?? 0,
    }
  })

  return (
    <div>
      {/* 全局统计卡片 */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        {[
          { label: '服务器', value: `${onlineCount}/${servers.length}`, sub: '在线', color: onlineCount === servers.length ? 'var(--success)' : 'var(--warning)' },
          { label: '容器', value: `${totalContainers}`, sub: '运行中', color: 'var(--accent)' },
          { label: '端口探测', value: `${probesUp}/${probes.length}`, sub: '正常', color: probesUp === probes.length ? 'var(--success)' : 'var(--danger)' },
          { label: '平均 CPU', value: `${serverMetrics.length > 0 ? (serverMetrics.reduce((s, m) => s + m.cpuPercent, 0) / serverMetrics.length).toFixed(1) : '0'}%`, sub: '使用率', color: 'var(--accent)' },
        ].map((stat) => (
          <div key={stat.label} className="rounded-xl p-4" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
            <div className="text-xs mb-1" style={{ color: 'var(--text-secondary)' }}>{stat.label}</div>
            <div className="text-2xl font-bold" style={{ color: stat.color }}>{stat.value}</div>
            <div className="text-xs" style={{ color: 'var(--text-secondary)' }}>{stat.sub}</div>
          </div>
        ))}
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        {/* 左列：服务器状态紧凑列表 */}
        <div className="rounded-xl p-5" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base font-semibold" style={{ color: 'var(--text-primary)' }}>服务器状态</h2>
            <Link to="/servers" className="text-xs" style={{ color: 'var(--accent)' }}>查看全部 &rarr;</Link>
          </div>
          <div className="space-y-3">
            {serverMetrics.map((s) => (
              <Link key={s.host_id} to={`/servers/${s.host_id}`}
                className="flex items-center gap-4 p-3 rounded-lg transition-colors"
                style={{ backgroundColor: 'var(--bg-secondary)' }}>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <StatusBadge status={s.status} />
                    <span className="text-sm font-medium truncate" style={{ color: 'var(--text-primary)' }}>
                      {s.display_name || s.hostname}
                    </span>
                    <span className="text-xs" style={{ color: 'var(--text-secondary)' }}>
                      {(() => { try { return JSON.parse(s.ip_addresses)[0] } catch { return '' } })()}
                    </span>
                  </div>
                  <div className="grid grid-cols-3 gap-2">
                    <ProgressBar percent={s.cpuPercent} label="CPU" />
                    <ProgressBar percent={s.memPercent} label="MEM" />
                    <ProgressBar percent={s.diskPercent} label="DISK" />
                  </div>
                </div>
                <div className="text-right text-xs whitespace-nowrap" style={{ color: 'var(--text-secondary)' }}>
                  <div>&uarr;{formatBytesPS(s.netTx)}</div>
                  <div>&darr;{formatBytesPS(s.netRx)}</div>
                </div>
              </Link>
            ))}
          </div>
        </div>

        {/* 右列：端口监控 + 资源 Top */}
        <div className="space-y-6">
          {/* 端口监控 */}
          <div className="rounded-xl p-5" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-base font-semibold" style={{ color: 'var(--text-primary)' }}>端口监控</h2>
              <Link to="/probes" className="text-xs" style={{ color: 'var(--accent)' }}>管理 &rarr;</Link>
            </div>
            {probes.length > 0 ? (
              <div className="space-y-2">
                {probes.map((p) => (
                  <div key={p.rule_id} className="flex items-center justify-between py-1.5">
                    <div className="flex items-center gap-2">
                      <StatusBadge status={p.status === 'up' ? 'up' : 'down'} />
                      <span className="text-sm" style={{ color: 'var(--text-primary)' }}>{p.name}</span>
                      <span className="text-xs" style={{ color: 'var(--text-secondary)' }}>{p.host}:{p.port}</span>
                    </div>
                    <span className="text-xs" style={{ color: p.status === 'up' ? 'var(--success)' : 'var(--danger)' }}>
                      {p.status === 'up' ? `${p.latency_ms.toFixed(1)}ms` : 'DOWN'}
                    </span>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-sm py-4 text-center" style={{ color: 'var(--text-secondary)' }}>
                暂无探测规则，<Link to="/probes" style={{ color: 'var(--accent)' }}>去添加</Link>
              </div>
            )}
          </div>

          {/* 资源 Top 排行 */}
          <div className="rounded-xl p-5" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
            <h2 className="text-base font-semibold mb-4" style={{ color: 'var(--text-primary)' }}>资源使用排行</h2>
            <div className="space-y-3">
              <div>
                <div className="text-xs mb-2" style={{ color: 'var(--text-secondary)' }}>CPU 使用率</div>
                {[...serverMetrics].sort((a, b) => b.cpuPercent - a.cpuPercent).map((s) => (
                  <div key={s.host_id} className="flex items-center gap-2 mb-1.5">
                    <span className="text-xs w-20 truncate" style={{ color: 'var(--text-secondary)' }}>{s.hostname}</span>
                    <div className="flex-1"><ProgressBar percent={s.cpuPercent} showValue={false} /></div>
                    <span className="text-xs w-12 text-right" style={{ color: 'var(--text-primary)' }}>{s.cpuPercent.toFixed(1)}%</span>
                  </div>
                ))}
              </div>
              <div>
                <div className="text-xs mb-2" style={{ color: 'var(--text-secondary)' }}>内存使用率</div>
                {[...serverMetrics].sort((a, b) => b.memPercent - a.memPercent).map((s) => (
                  <div key={s.host_id} className="flex items-center gap-2 mb-1.5">
                    <span className="text-xs w-20 truncate" style={{ color: 'var(--text-secondary)' }}>{s.hostname}</span>
                    <div className="flex-1"><ProgressBar percent={s.memPercent} showValue={false} /></div>
                    <span className="text-xs w-12 text-right" style={{ color: 'var(--text-primary)' }}>{s.memPercent.toFixed(1)}%</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>

      {servers.length === 0 && (
        <div className="text-center py-12" style={{ color: 'var(--text-secondary)' }}>
          暂无服务器数据，请部署 Agent 后等待上报
        </div>
      )}
    </div>
  )
}
