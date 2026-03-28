import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useNasStore } from '../../stores/nasStore'
import type { NasDevice, NasMetrics, NasRaid, NasDisk } from '../../api/nas'

// ─── helpers ────────────────────────────────────────────────────────────────

function formatBytes(bytes: number): string {
  if (bytes >= 1e12) return (bytes / 1e12).toFixed(1) + ' TB'
  if (bytes >= 1e9) return (bytes / 1e9).toFixed(1) + ' GB'
  if (bytes >= 1e6) return (bytes / 1e6).toFixed(1) + ' MB'
  return (bytes / 1e3).toFixed(0) + ' KB'
}

function formatBytesPS(bytes: number): string {
  if (bytes >= 1e6) return (bytes / 1e6).toFixed(1) + ' MB/s'
  if (bytes >= 1e3) return (bytes / 1e3).toFixed(0) + ' KB/s'
  return bytes.toFixed(0) + ' B/s'
}

function parseSystemInfo(info: string): { model?: string; os_version?: string } {
  try { return JSON.parse(info) } catch { return {} }
}

// ─── sub-components ──────────────────────────────────────────────────────────

function ProgressBar({ value, className = '' }: { value: number; className?: string }) {
  const barColor = value >= 90 ? 'bg-red-500' : value >= 75 ? 'bg-[#f59e0b]' : 'bg-[#2ca07a]'
  return (
    <div className={`h-1.5 w-full rounded-full bg-[#e9ecef] overflow-hidden ${className}`}>
      <div
        className={`h-full rounded-full transition-all ${barColor}`}
        style={{ width: `${Math.min(100, value)}%` }}
      />
    </div>
  )
}

function StatusDot({ status }: { status: NasDevice['status'] }) {
  const colors: Record<NasDevice['status'], string> = {
    online:   'bg-[#2ca07a]',
    offline:  'bg-[#ef4444]',
    degraded: 'bg-[#f59e0b]',
    unknown:  'bg-[#9ca3af]',
  }
  return <span className={`inline-block w-2 h-2 rounded-full ${colors[status]}`} />
}

function NasTypeBadge({ type }: { type: NasDevice['nas_type'] }) {
  const label = type === 'synology' ? 'Synology' : 'fnOS'
  const cls =
    type === 'synology'
      ? 'bg-[rgba(44,160,122,0.12)] text-[#2ca07a]'
      : 'bg-[rgba(99,102,241,0.12)] text-[#6366f1]'
  return (
    <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${cls}`}>
      {label}
    </span>
  )
}

function RaidStatusBadge({ status }: { status: string }) {
  const lower = status.toLowerCase()
  const isOk = lower === 'normal' || lower === 'healthy' || lower === 'ok'
  const isDeg = lower.includes('degrad') || lower.includes('rebuild')
  const cls = isOk
    ? 'bg-[rgba(44,160,122,0.1)] text-[#2ca07a]'
    : isDeg
    ? 'bg-[rgba(245,158,11,0.1)] text-[#f59e0b]'
    : 'bg-[rgba(239,68,68,0.1)] text-[#ef4444]'
  return (
    <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${cls}`}>
      {status}
    </span>
  )
}

function DiskHealthDot({ healthy }: { healthy: boolean }) {
  return (
    <span
      className={`inline-block w-1.5 h-1.5 rounded-full ${healthy ? 'bg-[#2ca07a]' : 'bg-[#ef4444]'}`}
      title={healthy ? '正常' : '异常'}
    />
  )
}

// ─── NAS device card ─────────────────────────────────────────────────────────

interface NasCardProps {
  device: NasDevice
  metrics: NasMetrics | undefined
}

function NasCard({ device, metrics }: NasCardProps) {
  const sysInfo = parseSystemInfo(device.system_info)

  const cpu = metrics?.cpu?.usage_percent ?? 0
  const mem = metrics?.memory?.usage_percent ?? 0
  const hasMem = !!metrics?.memory

  const totalRx = metrics?.networks?.reduce((s, n) => s + (n.rx_bytes_per_sec ?? 0), 0) ?? 0
  const totalTx = metrics?.networks?.reduce((s, n) => s + (n.tx_bytes_per_sec ?? 0), 0) ?? 0

  const raids: NasRaid[] = metrics?.raids ?? []
  const disks: NasDisk[] = metrics?.disks ?? []
  const ups = metrics?.ups

  // Find a volume matched to each raid (by disk members, or just first volume)
  const volumes = metrics?.volumes ?? []

  return (
    <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] border border-[#e9ecef] p-4 flex flex-col gap-3">

      {/* ── Header ── */}
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <StatusDot status={device.status} />
          <span className="text-[14px] font-semibold text-[#495057] truncate">{device.name}</span>
        </div>
        <NasTypeBadge type={device.nas_type} />
      </div>

      {/* ── IP + version ── */}
      <div className="flex items-center gap-3 text-[11px] text-[#878a99]">
        <span className="flex items-center gap-1">
          <span className="material-symbols-outlined" style={{ fontSize: '13px' }}>lan</span>
          <span className="font-mono">{device.host}</span>
        </span>
        {sysInfo.os_version && (
          <span className="truncate">{sysInfo.os_version}</span>
        )}
        {sysInfo.model && !sysInfo.os_version && (
          <span className="truncate">{sysInfo.model}</span>
        )}
      </div>

      {/* ── CPU / Memory ── */}
      {metrics && (
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-2">
            <span className="text-[11px] text-[#878a99] w-8">CPU</span>
            <ProgressBar value={cpu} className="flex-1" />
            <span className="text-[11px] font-mono text-[#495057] w-10 text-right">
              {cpu.toFixed(1)}%
            </span>
          </div>
          {hasMem && (
            <div className="flex items-center gap-2">
              <span className="text-[11px] text-[#878a99] w-8">内存</span>
              <ProgressBar value={mem} className="flex-1" />
              <span className="text-[11px] font-mono text-[#495057] w-10 text-right">
                {mem.toFixed(1)}%
              </span>
            </div>
          )}
        </div>
      )}

      {/* ── RAID / Volume pools ── */}
      {raids.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <span className="text-[11px] font-semibold text-[#878a99] uppercase tracking-wide">存储池</span>
          {raids.map((r, i) => {
            // Try to pair with a volume that matches the array name
            const vol =
              volumes.find((v) => v.mount === r.array || v.mount.includes(r.array)) ??
              volumes[i] ??
              null
            const usePct = vol ? vol.usage_percent : 0
            return (
              <div key={i} className="flex flex-col gap-1">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-[11px] text-[#495057] truncate max-w-[120px]">
                    {r.array} · {r.raid_type}
                  </span>
                  <RaidStatusBadge status={r.status} />
                </div>
                {vol && (
                  <div className="flex items-center gap-2">
                    <ProgressBar value={usePct} className="flex-1" />
                    <span className="text-[10px] font-mono text-[#878a99] whitespace-nowrap">
                      {formatBytes(vol.used)}/{formatBytes(vol.total)}
                    </span>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* Volumes without raid pairing */}
      {raids.length === 0 && volumes.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <span className="text-[11px] font-semibold text-[#878a99] uppercase tracking-wide">存储卷</span>
          {volumes.map((v, i) => (
            <div key={i} className="flex flex-col gap-1">
              <div className="flex items-center justify-between gap-1">
                <span className="text-[11px] text-[#495057] truncate">{v.mount}</span>
                <span className="text-[10px] font-mono text-[#878a99]">{v.usage_percent.toFixed(0)}%</span>
              </div>
              <ProgressBar value={v.usage_percent} />
            </div>
          ))}
        </div>
      )}

      {/* ── Disks ── */}
      {disks.length > 0 && (
        <div className="flex flex-col gap-1">
          <span className="text-[11px] font-semibold text-[#878a99] uppercase tracking-wide">磁盘</span>
          <div className="flex flex-col gap-0.5">
            {disks.map((d, i) => (
              <div key={i} className="flex items-center gap-1.5 text-[11px]">
                <DiskHealthDot healthy={d.smart_healthy} />
                <span className="text-[#495057] font-mono w-12 truncate">{d.name}</span>
                <span className="text-[#878a99] truncate flex-1">{d.model || '—'}</span>
                {d.temperature > 0 && (
                  <span className={`font-mono ${d.temperature >= 55 ? 'text-[#ef4444]' : d.temperature >= 45 ? 'text-[#f59e0b]' : 'text-[#878a99]'}`}>
                    {d.temperature}°C
                  </span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── Network ── */}
      {metrics?.networks && metrics.networks.length > 0 && (
        <div className="flex items-center gap-4 text-[11px] font-mono text-[#878a99] border-t border-[#f0f0f0] pt-2 mt-0.5">
          <span className="flex items-center gap-1">
            <span className="material-symbols-outlined" style={{ fontSize: '13px', color: '#f06548' }}>arrow_downward</span>
            {formatBytesPS(totalRx)}
          </span>
          <span className="flex items-center gap-1">
            <span className="material-symbols-outlined" style={{ fontSize: '13px', color: '#2ca07a' }}>arrow_upward</span>
            {formatBytesPS(totalTx)}
          </span>
        </div>
      )}

      {/* ── UPS ── */}
      {ups && (
        <div className="flex items-center gap-2 bg-[#fafafa] border border-[#e9ecef] rounded px-2 py-1.5">
          <span className="material-symbols-outlined text-[#f59e0b]" style={{ fontSize: '14px' }}>battery_charging_full</span>
          <span className="text-[11px] text-[#495057] font-semibold">{ups.status}</span>
          <span className="text-[11px] text-[#878a99]">{ups.battery_percent}%</span>
          {ups.model && (
            <span className="text-[11px] text-[#878a99] truncate ml-auto">{ups.model}</span>
          )}
        </div>
      )}

      {/* Offline/unknown state — no metrics */}
      {!metrics && device.status !== 'online' && (
        <div className="text-[11px] text-[#878a99] italic">暂无监控数据</div>
      )}
    </div>
  )
}

// ─── Stat card ────────────────────────────────────────────────────────────────

interface StatCardProps {
  icon: string
  label: string
  value: number | string
  iconColor?: string
  valueColor?: string
}

function StatCard({ icon, label, value, iconColor = '#2ca07a', valueColor = '#495057' }: StatCardProps) {
  return (
    <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] border border-[#e9ecef] px-5 py-4 flex items-center gap-4">
      <div
        className="w-10 h-10 rounded-full flex items-center justify-center shrink-0"
        style={{ background: `${iconColor}18` }}
      >
        <span className="material-symbols-outlined" style={{ fontSize: '20px', color: iconColor }}>
          {icon}
        </span>
      </div>
      <div>
        <div className="text-[11px] text-[#878a99]">{label}</div>
        <div className="text-[22px] font-bold leading-tight" style={{ color: valueColor }}>
          {value}
        </div>
      </div>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function NAS() {
  const { devices, metrics, loading, fetchDevices, updateMetrics, updateStatus } = useNasStore()

  // Initial load
  useEffect(() => {
    fetchDevices()
  }, [fetchDevices])

  // Real-time WebSocket events
  useEffect(() => {
    const onMetrics = (e: Event) => {
      const ev = e as CustomEvent<{ nas_id: number } & Record<string, unknown>>
      if (ev.detail?.nas_id != null) {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        updateMetrics(ev.detail.nas_id, ev.detail as any)
      }
    }
    const onStatus = (e: Event) => {
      const ev = e as CustomEvent<{ nas_id: number; status: string }>
      if (ev.detail?.nas_id != null) {
        updateStatus(ev.detail.nas_id, ev.detail.status)
      }
    }
    window.addEventListener('nas_metrics', onMetrics)
    window.addEventListener('nas_status', onStatus)
    return () => {
      window.removeEventListener('nas_metrics', onMetrics)
      window.removeEventListener('nas_status', onStatus)
    }
  }, [updateMetrics, updateStatus])

  // ── Aggregated stats ──
  const totalCount   = devices.length
  const onlineCount  = devices.filter((d) => d.status === 'online').length

  const raidDegradedCount = Object.values(metrics).reduce((sum, m) => {
    if (!m?.raids) return sum
    const deg = m.raids.filter((r) => {
      const s = r.status.toLowerCase()
      return s !== 'normal' && s !== 'healthy' && s !== 'ok'
    }).length
    return sum + deg
  }, 0)

  const diskUnhealthyCount = Object.values(metrics).reduce((sum, m) => {
    if (!m?.disks) return sum
    return sum + m.disks.filter((d) => !d.smart_healthy).length
  }, 0)

  return (
    <div className="flex flex-col gap-5 pb-6">

      {/* ── Page header ── */}
      <div className="flex items-center justify-between">
        <h4 className="text-[18px] font-semibold text-[#495057] mb-0">NAS 存储</h4>
        {loading && (
          <span className="text-[12px] text-[#878a99] flex items-center gap-1">
            <span className="material-symbols-outlined animate-spin" style={{ fontSize: '14px' }}>progress_activity</span>
            加载中
          </span>
        )}
      </div>

      {/* ── Stats bar ── */}
      {totalCount > 0 && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <StatCard icon="storage" label="设备总数" value={totalCount} />
          <StatCard
            icon="check_circle"
            label="在线"
            value={onlineCount}
            iconColor="#2ca07a"
            valueColor="#2ca07a"
          />
          <StatCard
            icon="warning"
            label="RAID 降级"
            value={raidDegradedCount}
            iconColor={raidDegradedCount > 0 ? '#f59e0b' : '#9ca3af'}
            valueColor={raidDegradedCount > 0 ? '#f59e0b' : '#495057'}
          />
          <StatCard
            icon="hard_drive"
            label="磁盘异常"
            value={diskUnhealthyCount}
            iconColor={diskUnhealthyCount > 0 ? '#ef4444' : '#9ca3af'}
            valueColor={diskUnhealthyCount > 0 ? '#ef4444' : '#495057'}
          />
        </div>
      )}

      {/* ── Empty state ── */}
      {!loading && totalCount === 0 && (
        <div className="bg-white rounded-[10px] border border-[#e9ecef] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-16 text-center">
          <div className="w-14 h-14 rounded-full bg-[rgba(44,160,122,0.1)] flex items-center justify-center mx-auto mb-4">
            <span className="material-symbols-outlined text-[#2ca07a] text-3xl">storage</span>
          </div>
          <p className="text-[#495057] text-[15px] mb-1">暂无 NAS 设备</p>
          <p className="text-[#878a99] text-[12px] mb-4">
            请前往设置页面添加 NAS 设备以开始监控
          </p>
          <Link
            to="/settings"
            className="inline-flex items-center gap-1.5 px-4 py-2 bg-[#2ca07a] hover:bg-[#1f7d5e] text-white text-[13px] rounded transition-colors"
          >
            <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>settings</span>
            前往设置
          </Link>
        </div>
      )}

      {/* ── Device grid ── */}
      {totalCount > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {devices.map((d) => (
            <NasCard key={d.id} device={d} metrics={metrics[d.id]} />
          ))}
        </div>
      )}

    </div>
  )
}
