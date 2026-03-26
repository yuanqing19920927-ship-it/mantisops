import { Link } from 'react-router-dom'
import type { Server, MetricsPayload } from '../types'
import { ProgressBar } from './ProgressBar'
import { StatusBadge } from './StatusBadge'

interface Props {
  server: Server
  metrics?: MetricsPayload
}

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

export function ServerCard({ server, metrics }: Props) {
  const cpuPercent = metrics?.cpu?.usage_percent ?? 0
  const memPercent = metrics?.memory?.usage_percent ?? 0
  const diskPercent = metrics?.disks?.[0]?.usage_percent ?? 0
  const rxPS = metrics?.networks?.reduce((sum, n) => sum + n.rx_bytes_per_sec, 0) ?? 0
  const txPS = metrics?.networks?.reduce((sum, n) => sum + n.tx_bytes_per_sec, 0) ?? 0
  const containerCount = metrics?.containers?.filter((c) => c.state === 'running').length ?? 0

  return (
    <Link
      to={`/servers/${server.host_id}`}
      className="block rounded-xl p-5 transition-all hover:scale-[1.02]"
      style={{
        backgroundColor: 'var(--bg-card)',
        border: '1px solid var(--border)',
      }}
    >
      <div className="flex items-center justify-between mb-3">
        <div>
          <div className="font-semibold" style={{ color: 'var(--text-primary)' }}>
            {server.display_name || server.hostname}
          </div>
          <div className="text-xs" style={{ color: 'var(--text-secondary)' }}>
            {JSON.parse(server.ip_addresses || '[]')[0] || ''} | {server.os}
          </div>
        </div>
        <StatusBadge status={server.status} />
      </div>

      <div className="text-xs mb-3" style={{ color: 'var(--text-secondary)' }}>
        {server.cpu_cores}核 {formatBytes(server.memory_total)}
        {server.gpu_model ? ` | ${server.gpu_model}` : ''}
      </div>

      <ProgressBar percent={cpuPercent} label="CPU" />
      <ProgressBar percent={memPercent} label="MEM" />
      <ProgressBar percent={diskPercent} label="DISK" />

      <div className="flex justify-between text-xs mt-2" style={{ color: 'var(--text-secondary)' }}>
        <span>↑ {formatBytesPS(txPS)}</span>
        <span>↓ {formatBytesPS(rxPS)}</span>
      </div>

      {containerCount > 0 && (
        <div className="text-xs mt-2" style={{ color: 'var(--text-secondary)' }}>
          Docker: {containerCount} running
        </div>
      )}

      {metrics?.gpu && (
        <div className="mt-2">
          <ProgressBar percent={metrics.gpu.usage_percent} label="GPU" />
        </div>
      )}
    </Link>
  )
}
