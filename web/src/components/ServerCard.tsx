import { Link } from 'react-router-dom'
import type { Server, ServerGroup, MetricsPayload } from '../types'
import { formatBytes, formatBytesPS } from '../utils/format'

interface Props {
  server: Server
  metrics?: MetricsPayload
  groups?: ServerGroup[]
  onGroupChange?: (hostId: string, groupId: number | null) => void
}

// Inline progress bar styled to match mockup (5px height, semantic colors)
function MiniProgressBar({ percent, label, offline }: { percent: number; label: string; offline?: boolean }) {
  const clamped = Math.min(100, Math.max(0, percent))
  const barColor = offline
    ? 'bg-[#ced4da]'
    : clamped >= 80
    ? 'bg-[#f06548]'
    : clamped >= 60
    ? 'bg-[#f7b84b]'
    : 'bg-[#0ab39c]'
  const valueText = offline ? '-' : `${clamped.toFixed(0)}%`

  return (
    <div className="mb-2">
      <div className="flex justify-between mb-1">
        <span className="text-[11px] text-[#878a99]">{label}</span>
        <span className="text-[11px] text-[#878a99]">{valueText}</span>
      </div>
      <div className="w-full h-[5px] bg-[#e9ecef] rounded-full overflow-hidden">
        <div
          className={`h-[5px] ${barColor} rounded-full transition-all duration-500`}
          style={{ width: offline ? '0%' : `${clamped}%` }}
        />
      </div>
    </div>
  )
}

export function ServerCard({ server, metrics, groups, onGroupChange }: Props) {
  const cpuPercent = metrics?.cpu?.usage_percent ?? 0
  const memPercent = metrics?.memory?.usage_percent ?? 0
  const diskPercent = Math.max(0, ...(metrics?.disks?.map(d => d.usage_percent) ?? [0]))
  const rxPS = metrics?.networks?.reduce((sum, n) => sum + (n.rx_bytes_per_sec ?? 0), 0) ?? 0
  const txPS = metrics?.networks?.reduce((sum, n) => sum + (n.tx_bytes_per_sec ?? 0), 0) ?? 0
  const containerCount = metrics?.containers?.filter((c) => c.state === 'running').length ?? 0
  const isOnline = server.status === 'online'
  const offline = !isOnline

  let ip = ''
  try { ip = JSON.parse(server.ip_addresses || '[]')[0] || '' } catch { /* empty */ }

  // Determine icon color: green for online, amber for offline, blue for cloud-type
  const iconBg = isOnline ? 'rgba(44,160,122,0.1)' : 'rgba(247,184,75,0.1)'
  const iconColor = isOnline ? '#2ca07a' : '#f7b84b'

  // Tag badge: show GPU label or group name tag
  const tagLabel = server.gpu_model ? 'GPU' : null

  const osShort = server.os?.split(' ').slice(0, 3).join(' ') ?? ''
  const coresLabel = server.cpu_cores ? `${server.cpu_cores}核` : null
  const ramLabel = server.memory_total ? formatBytes(server.memory_total) : null

  return (
    <Link
      to={`/servers/${server.host_id}`}
      className="block bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] border border-[#e9ecef] cursor-pointer transition-all duration-300 hover:-translate-y-[5px] hover:shadow-[0_10px_25px_rgba(44,160,122,0.15)] hover:border-[rgba(44,160,122,0.3)] h-full"
    >
      <div className="p-4 flex flex-col h-full">

        {/* Header: icon + hostname/badge/ip + optional tag */}
        <div className="flex justify-between mb-3">
          <div className="flex items-center gap-3">
            {/* 48px stat-icon */}
            <div
              className="flex-shrink-0 w-12 h-12 rounded-full flex items-center justify-center shadow-sm"
              style={{ backgroundColor: iconBg, color: iconColor }}
            >
              <span className="material-symbols-outlined text-[28px]">computer</span>
            </div>
            <div>
              <div className="flex items-center gap-2 mb-1">
                <span className="text-[15px] font-semibold text-[#495057] leading-tight">
                  {server.display_name || server.hostname}
                </span>
                {isOnline ? (
                  <span className="text-[11px] px-1.5 py-0.5 rounded bg-[rgba(10,179,156,0.1)] text-[#0ab39c] font-medium">在线</span>
                ) : (
                  <span className="text-[11px] px-1.5 py-0.5 rounded bg-[rgba(240,101,72,0.1)] text-[#f06548] font-medium">离线</span>
                )}
              </div>
              <div className="text-[12px] font-mono text-[#878a99]">{ip}</div>
            </div>
          </div>
          {/* Right-side tag badge */}
          <div className="flex-shrink-0">
            {tagLabel && (
              <span className="text-[11px] px-1.5 py-0.5 rounded bg-[#212529] text-white font-medium">
                {tagLabel}
              </span>
            )}
          </div>
        </div>

        {/* Hardware summary badges */}
        <div className="flex flex-wrap gap-1.5 mb-3">
          {osShort && (
            <span className="text-[11px] px-2 py-0.5 rounded bg-[#f8f9fa] text-[#495057] border border-[#e9ecef]">
              {osShort}
            </span>
          )}
          {coresLabel && (
            <span className="text-[11px] px-2 py-0.5 rounded bg-[#f8f9fa] text-[#495057] border border-[#e9ecef]">
              {coresLabel}
            </span>
          )}
          {ramLabel && (
            <span className="text-[11px] px-2 py-0.5 rounded bg-[#f8f9fa] text-[#495057] border border-[#e9ecef]">
              {ramLabel}
            </span>
          )}
          {containerCount > 0 && (
            <span className="text-[11px] px-2 py-0.5 rounded bg-[#f8f9fa] text-[#495057] border border-[#e9ecef] flex items-center gap-1">
              Docker
              <span className="material-symbols-outlined" style={{ fontSize: '10px', verticalAlign: 'middle' }}>box</span>
              {containerCount}
            </span>
          )}
          {server.gpu_model && (
            <span className="text-[11px] px-2 py-0.5 rounded bg-[#f8f9fa] text-[#495057] border border-[#e9ecef] truncate max-w-[120px]">
              {server.gpu_model}
            </span>
          )}
        </div>

        {/* Progress bars */}
        <div className="mb-1">
          <MiniProgressBar percent={cpuPercent} label="CPU" offline={offline} />
          <MiniProgressBar percent={memPercent} label="内存" offline={offline} />
          <MiniProgressBar percent={diskPercent} label="磁盘" offline={offline} />
        </div>

        {/* Network speed row */}
        <div className="flex justify-between border-t border-[#e9ecef] pt-3 mt-auto">
          <div className="text-[12px] text-[#878a99] flex items-center gap-0.5">
            <span className="material-symbols-outlined" style={{ fontSize: '14px', verticalAlign: 'text-bottom', color: '#f06548' }}>arrow_downward</span>
            {offline ? '-' : formatBytesPS(rxPS)}
          </div>
          <div className="text-[12px] text-[#878a99] flex items-center gap-0.5">
            <span className="material-symbols-outlined" style={{ fontSize: '14px', verticalAlign: 'text-bottom', color: '#0ab39c' }}>arrow_upward</span>
            {offline ? '-' : formatBytesPS(txPS)}
          </div>
        </div>

        {/* Group Selector */}
        {groups && groups.length > 0 && onGroupChange && (
          <div
            className="flex items-center gap-2 mt-3 pt-3 border-t border-[#e9ecef]"
            onClick={(e) => { e.preventDefault(); e.stopPropagation() }}
          >
            <span className="material-symbols-outlined text-xs text-[#878a99]" style={{ fontSize: '14px' }}>folder</span>
            <select
              value={server.group_id ?? ''}
              onChange={(e) => {
                e.preventDefault()
                e.stopPropagation()
                const val = e.target.value
                onGroupChange(server.host_id, val ? Number(val) : null)
              }}
              className="text-[11px] flex-1 bg-[#f8f9fa] border border-[#e9ecef] text-[#878a99] rounded px-2 py-1 cursor-pointer focus:outline-none focus:border-[#2ca07a]"
            >
              <option value="">未分组</option>
              {groups.map(g => (
                <option key={g.id} value={g.id}>{g.name}</option>
              ))}
            </select>
          </div>
        )}

      </div>
    </Link>
  )
}
