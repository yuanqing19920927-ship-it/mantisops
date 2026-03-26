import { useParams, Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { useServerStore } from '../../stores/serverStore'
import { useWebSocket } from '../../hooks/useWebSocket'
import { getServer } from '../../api/client'
import { MetricsChart } from '../../components/MetricsChart'
import { StatusBadge } from '../../components/StatusBadge'
import type { Server } from '../../types'

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

function timeSince(unix: number): string {
  const diff = Math.floor(Date.now() / 1000) - unix
  if (diff < 60) return `${diff}秒前`
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`
  return `${Math.floor(diff / 86400)}天前`
}

export default function ServerDetail() {
  const { id } = useParams<{ id: string }>()
  const [server, setServer] = useState<Server | null>(null)
  const metrics = useServerStore((s) => id ? s.metrics[id] : undefined)
  useWebSocket()

  useEffect(() => {
    if (id) {
      getServer(id).then(setServer).catch(() => {})
    }
  }, [id])

  if (!server) {
    return <div style={{ color: 'var(--text-secondary)' }}>加载中...</div>
  }

  const cpu = metrics?.cpu
  const mem = metrics?.memory
  const disk = metrics?.disks?.[0]
  const nets = metrics?.networks || []
  const rxTotal = nets.reduce((s, n) => s + n.rx_bytes_per_sec, 0)
  const txTotal = nets.reduce((s, n) => s + n.tx_bytes_per_sec, 0)
  const containers = metrics?.containers || []
  const gpu = metrics?.gpu

  return (
    <div>
      {/* 头部 */}
      <div className="flex items-center gap-4 mb-6">
        <Link to="/" className="text-sm" style={{ color: 'var(--accent)' }}>
          &larr; 返回
        </Link>
        <h1 className="text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>
          {server.display_name || server.hostname}
        </h1>
        <StatusBadge status={server.status} label={server.status === 'online' ? '在线' : '离线'} />
      </div>

      {/* 基本信息 */}
      <div
        className="rounded-xl p-5 mb-6"
        style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}
      >
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
          <div>
            <div style={{ color: 'var(--text-secondary)' }}>操作系统</div>
            <div style={{ color: 'var(--text-primary)' }}>{server.os}</div>
          </div>
          <div>
            <div style={{ color: 'var(--text-secondary)' }}>内核</div>
            <div style={{ color: 'var(--text-primary)' }}>{server.kernel}</div>
          </div>
          <div>
            <div style={{ color: 'var(--text-secondary)' }}>CPU</div>
            <div style={{ color: 'var(--text-primary)' }}>{server.cpu_model || `${server.cpu_cores}核`}</div>
          </div>
          <div>
            <div style={{ color: 'var(--text-secondary)' }}>内存 / 磁盘</div>
            <div style={{ color: 'var(--text-primary)' }}>
              {formatBytes(server.memory_total)} / {formatBytes(server.disk_total)}
            </div>
          </div>
          {server.gpu_model && (
            <div>
              <div style={{ color: 'var(--text-secondary)' }}>GPU</div>
              <div style={{ color: 'var(--text-primary)' }}>{server.gpu_model}</div>
            </div>
          )}
          <div>
            <div style={{ color: 'var(--text-secondary)' }}>IP</div>
            <div style={{ color: 'var(--text-primary)' }}>
              {(() => {
                try { return JSON.parse(server.ip_addresses)[0] } catch { return '-' }
              })()}
            </div>
          </div>
          <div>
            <div style={{ color: 'var(--text-secondary)' }}>最后心跳</div>
            <div style={{ color: 'var(--text-primary)' }}>{server.last_seen ? timeSince(server.last_seen) : '-'}</div>
          </div>
          <div>
            <div style={{ color: 'var(--text-secondary)' }}>Agent</div>
            <div style={{ color: 'var(--text-primary)' }}>{server.agent_version || '-'}</div>
          </div>
        </div>
      </div>

      {/* 实时监控图表 */}
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4 mb-6">
        <MetricsChart value={cpu?.usage_percent ?? 0} label="CPU" />
        <MetricsChart value={mem?.usage_percent ?? 0} label="内存" />
        <MetricsChart value={disk?.usage_percent ?? 0} label="磁盘" />
        <MetricsChart
          value={(rxTotal + txTotal) / 1024 / 1024}
          label="网络"
          unit=" MB/s"
          color="var(--accent)"
        />
      </div>

      {/* 网络详情 */}
      {nets.length > 0 && (
        <div className="mb-6 text-sm" style={{ color: 'var(--text-secondary)' }}>
          网络: ↑ {formatBytesPS(txTotal)} ↓ {formatBytesPS(rxTotal)}
        </div>
      )}

      {/* GPU 信息 */}
      {gpu && (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
          <MetricsChart value={gpu.usage_percent} label="GPU 使用率" color="var(--accent)" />
          <MetricsChart
            value={gpu.memory_total > 0 ? (gpu.memory_used / gpu.memory_total) * 100 : 0}
            label="GPU 显存"
          />
          <MetricsChart value={gpu.temperature} label="GPU 温度" unit="°C" color="var(--warning)" />
        </div>
      )}

      {/* Docker 容器 */}
      {containers.length > 0 && (
        <div
          className="rounded-xl p-5 mb-6"
          style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}
        >
          <h2 className="text-lg font-semibold mb-4" style={{ color: 'var(--text-primary)' }}>
            Docker 容器 ({containers.filter(c => c.state === 'running').length}/{containers.length})
          </h2>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr style={{ color: 'var(--text-secondary)', borderBottom: '1px solid var(--border)' }}>
                  <th className="text-left py-2 px-3">名称</th>
                  <th className="text-left py-2 px-3">状态</th>
                  <th className="text-right py-2 px-3">CPU</th>
                  <th className="text-right py-2 px-3">内存</th>
                  <th className="text-left py-2 px-3">镜像</th>
                </tr>
              </thead>
              <tbody>
                {containers.map((c) => (
                  <tr key={c.container_id} style={{ borderBottom: '1px solid var(--border)' }}>
                    <td className="py-2 px-3" style={{ color: 'var(--text-primary)' }}>{c.name}</td>
                    <td className="py-2 px-3">
                      <StatusBadge status={c.state === 'running' ? 'online' : 'offline'} label={c.state} />
                    </td>
                    <td className="py-2 px-3 text-right" style={{ color: 'var(--text-primary)' }}>
                      {c.cpu_percent.toFixed(1)}%
                    </td>
                    <td className="py-2 px-3 text-right" style={{ color: 'var(--text-primary)' }}>
                      {formatBytes(c.memory_usage)}
                    </td>
                    <td className="py-2 px-3 text-xs" style={{ color: 'var(--text-secondary)' }}>
                      {c.image}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
