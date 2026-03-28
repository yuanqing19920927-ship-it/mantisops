import { useParams, Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { useNasStore } from '../../stores/nasStore'
import { getNasMetrics } from '../../api/nas'
import type { NasDevice, NasMetrics, NasRaid, NasDisk, NasVolume } from '../../api/nas'
import { HistoryChart } from '../../components/HistoryChart'

// ─── helpers ─────────────────────────────────────────────────────────────────

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

interface ParsedSysInfo {
  model?: string
  os_version?: string
  serial?: string
  packages?: unknown[]
}

function parseSystemInfo(info: string): ParsedSysInfo {
  try { return JSON.parse(info) } catch { return {} }
}

function calcWindow(range: TimeRange) {
  const now = Math.floor(Date.now() / 1000)
  const durations: Record<TimeRange, number> = { '1h': 3600, '6h': 21600, '24h': 86400, '7d': 604800 }
  const steps: Record<TimeRange, number> = { '1h': 15, '6h': 60, '24h': 300, '7d': 1800 }
  return { start: now - durations[range], end: now, step: steps[range] }
}

type TimeRange = '1h' | '6h' | '24h' | '7d'

// ─── sub-components ───────────────────────────────────────────────────────────

function ProgressBar({ value, className = '' }: { value: number; className?: string }) {
  const barColor = value >= 90 ? 'bg-red-500' : value >= 75 ? 'bg-[#f59e0b]' : 'bg-[#2ca07a]'
  return (
    <div className={`h-1.5 w-full rounded-full bg-[#e9ecef] overflow-hidden ${className}`}>
      <div
        className={`h-full rounded-full transition-all ${barColor}`}
        style={{ width: `${Math.min(100, Math.max(0, value))}%` }}
      />
    </div>
  )
}

function StatusBadge({ status }: { status: NasDevice['status'] }) {
  const config: Record<NasDevice['status'], { bg: string; dot: string; text: string; label: string }> = {
    online:   { bg: 'rgba(44,160,122,0.12)',  dot: '#2ca07a', text: '#2ca07a', label: 'online' },
    offline:  { bg: 'rgba(239,68,68,0.12)',   dot: '#ef4444', text: '#ef4444', label: 'offline' },
    degraded: { bg: 'rgba(245,158,11,0.12)',  dot: '#f59e0b', text: '#f59e0b', label: 'degraded' },
    unknown:  { bg: 'rgba(156,163,175,0.12)', dot: '#9ca3af', text: '#9ca3af', label: 'unknown' },
  }
  const c = config[status]
  return (
    <span
      className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[13px] font-medium"
      style={{ background: c.bg, color: c.text }}
    >
      <span
        className="inline-block rounded-full"
        style={{ width: 7, height: 7, background: c.dot, boxShadow: `0 0 6px ${c.dot}99` }}
      />
      {c.label}
    </span>
  )
}

function NasTypeBadge({ type }: { type: NasDevice['nas_type'] }) {
  const label = type === 'synology' ? 'Synology' : 'fnOS'
  const cls =
    type === 'synology'
      ? 'bg-[rgba(44,160,122,0.12)] text-[#2ca07a]'
      : 'bg-[rgba(99,102,241,0.12)] text-[#6366f1]'
  return (
    <span className={`text-[11px] font-semibold px-2 py-0.5 rounded ${cls}`}>
      {label}
    </span>
  )
}

function RaidStatusBadge({ status }: { status: string }) {
  const lower = status.toLowerCase()
  const isOk  = lower === 'normal' || lower === 'healthy' || lower === 'ok'
  const isDeg = lower.includes('degrad') || lower.includes('rebuild')
  const cls = isOk
    ? 'bg-[rgba(44,160,122,0.1)] text-[#2ca07a]'
    : isDeg
    ? 'bg-[rgba(245,158,11,0.1)] text-[#f59e0b]'
    : 'bg-[rgba(239,68,68,0.1)] text-[#ef4444]'
  return (
    <span className={`text-[11px] font-semibold px-1.5 py-0.5 rounded ${cls}`}>
      {status}
    </span>
  )
}

function TempText({ temp }: { temp: number }) {
  const color = temp >= 55 ? '#ef4444' : temp >= 45 ? '#f59e0b' : '#2ca07a'
  return <span className="font-mono text-[13px]" style={{ color }}>{temp}°C</span>
}

// ─── Section card wrapper ─────────────────────────────────────────────────────

function SectionCard({ title, children, action }: { title: string; children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div
      className="bg-white"
      style={{ borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.05)', padding: '20px 24px' }}
    >
      <div className="flex items-center justify-between mb-4">
        <h5 className="mb-0 font-semibold text-[15px] text-[#212529]">{title}</h5>
        {action}
      </div>
      {children}
    </div>
  )
}

// ─── RAID section ─────────────────────────────────────────────────────────────

function RaidSection({ raids }: { raids: NasRaid[] }) {
  if (raids.length === 0) return null
  return (
    <SectionCard title="RAID 阵列">
      <div className="overflow-x-auto">
        <table className="w-full" style={{ fontSize: 14 }}>
          <thead>
            <tr style={{ background: '#f8f9fa' }}>
              <th className="text-left py-2.5 px-4 font-semibold text-[13px] text-[#495057]">阵列名</th>
              <th className="text-left py-2.5 px-4 font-semibold text-[13px] text-[#495057]">类型</th>
              <th className="text-left py-2.5 px-4 font-semibold text-[13px] text-[#495057]">状态</th>
              <th className="text-left py-2.5 px-4 font-semibold text-[13px] text-[#495057]">成员磁盘</th>
              <th className="text-left py-2.5 px-4 font-semibold text-[13px] text-[#495057]">重建进度</th>
            </tr>
          </thead>
          <tbody>
            {raids.map((r, i) => {
              const isRebuilding = r.status.toLowerCase().includes('rebuild') && r.rebuild_percent > 0
              return (
                <tr
                  key={i}
                  className="transition-colors"
                  style={{ borderTop: '1px solid #f3f4f6' }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = '#f8f9fa')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = '')}
                >
                  <td className="py-3 px-4 font-medium text-[#212529]">{r.array}</td>
                  <td className="py-3 px-4 text-[#878a99] font-mono text-[12px]">{r.raid_type}</td>
                  <td className="py-3 px-4"><RaidStatusBadge status={r.status} /></td>
                  <td className="py-3 px-4 text-[#878a99] text-[12px]">
                    {r.disks.length > 0 ? r.disks.join(', ') : '—'}
                  </td>
                  <td className="py-3 px-4" style={{ minWidth: 140 }}>
                    {isRebuilding ? (
                      <div className="flex flex-col gap-1">
                        <ProgressBar value={r.rebuild_percent} />
                        <span className="text-[11px] text-[#f59e0b] font-mono">{r.rebuild_percent.toFixed(1)}%</span>
                      </div>
                    ) : (
                      <span className="text-[#878a99] text-[12px]">—</span>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </SectionCard>
  )
}

// ─── Volumes section ──────────────────────────────────────────────────────────

function VolumesSection({ volumes }: { volumes: NasVolume[] }) {
  if (volumes.length === 0) return null
  return (
    <SectionCard title="存储卷">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {volumes.map((v, i) => (
          <div
            key={i}
            className="flex flex-col gap-2 p-4 rounded-lg"
            style={{ background: '#f8f9fa', border: '1px solid #f0f0f0' }}
          >
            <div className="flex items-center justify-between gap-2">
              <span className="font-mono text-[13px] font-semibold text-[#495057] truncate">{v.mount}</span>
              <span className="text-[11px] text-[#878a99] bg-white border border-[#e9ecef] px-1.5 py-0.5 rounded">
                {v.fs_type}
              </span>
            </div>
            <ProgressBar value={v.usage_percent} />
            <div className="flex items-center justify-between text-[11px] font-mono text-[#878a99]">
              <span>{formatBytes(v.used)} 已用</span>
              <span className="font-semibold text-[#495057]">{v.usage_percent.toFixed(1)}%</span>
              <span>{formatBytes(v.total)} 总计</span>
            </div>
          </div>
        ))}
      </div>
    </SectionCard>
  )
}

// ─── Disk health table ────────────────────────────────────────────────────────

function DiskHealthSection({ disks }: { disks: NasDisk[] }) {
  const [expandedIdx, setExpandedIdx] = useState<number | null>(null)

  if (disks.length === 0) return null

  return (
    <SectionCard title="磁盘健康">
      <div className="overflow-x-auto">
        <table className="w-full" style={{ fontSize: 14 }}>
          <thead>
            <tr style={{ background: '#f8f9fa' }}>
              <th className="text-left py-2.5 px-4 font-semibold text-[13px] text-[#495057]">磁盘</th>
              <th className="text-left py-2.5 px-4 font-semibold text-[13px] text-[#495057]">型号</th>
              <th className="text-right py-2.5 px-4 font-semibold text-[13px] text-[#495057]">容量</th>
              <th className="text-right py-2.5 px-4 font-semibold text-[13px] text-[#495057]">温度</th>
              <th className="text-right py-2.5 px-4 font-semibold text-[13px] text-[#495057]">通电时长</th>
              <th className="text-center py-2.5 px-4 font-semibold text-[13px] text-[#495057]">健康</th>
              <th className="text-center py-2.5 px-4 font-semibold text-[13px] text-[#495057]">详情</th>
            </tr>
          </thead>
          <tbody>
            {disks.map((d, i) => (
              <>
                <tr
                  key={`disk-${i}`}
                  className="transition-colors cursor-pointer"
                  style={{ borderTop: '1px solid #f3f4f6' }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = '#f8f9fa')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = '')}
                >
                  <td className="py-3 px-4 font-mono font-semibold text-[#212529]">{d.name}</td>
                  <td className="py-3 px-4 text-[#878a99] text-[12px] max-w-[180px] truncate">{d.model || '—'}</td>
                  <td className="py-3 px-4 text-right font-mono text-[13px] text-[#878a99]">
                    {d.size > 0 ? formatBytes(d.size) : '—'}
                  </td>
                  <td className="py-3 px-4 text-right">
                    {d.temperature > 0 ? <TempText temp={d.temperature} /> : <span className="text-[#878a99]">—</span>}
                  </td>
                  <td className="py-3 px-4 text-right font-mono text-[13px] text-[#878a99]">
                    {d.power_on_hours > 0
                      ? d.power_on_hours >= 8760
                        ? `${(d.power_on_hours / 8760).toFixed(1)} 年`
                        : d.power_on_hours >= 24
                        ? `${Math.floor(d.power_on_hours / 24)} 天`
                        : `${d.power_on_hours} h`
                      : '—'}
                  </td>
                  <td className="py-3 px-4 text-center">
                    <span
                      className="inline-flex items-center gap-1 text-[12px] font-medium px-1.5 py-0.5 rounded"
                      style={
                        d.smart_healthy
                          ? { color: '#2ca07a', background: 'rgba(44,160,122,0.1)' }
                          : { color: '#ef4444', background: 'rgba(239,68,68,0.1)' }
                      }
                    >
                      <span
                        className="inline-block rounded-full"
                        style={{
                          width: 6, height: 6,
                          background: d.smart_healthy ? '#2ca07a' : '#ef4444',
                        }}
                      />
                      {d.smart_healthy ? '正常' : '异常'}
                    </span>
                  </td>
                  <td className="py-3 px-4 text-center">
                    <button
                      onClick={() => setExpandedIdx(expandedIdx === i ? null : i)}
                      className="inline-flex items-center justify-center w-6 h-6 rounded transition-colors hover:bg-[#e9ecef]"
                      style={{ color: '#878a99' }}
                      aria-label={expandedIdx === i ? '收起详情' : '展开详情'}
                      aria-expanded={expandedIdx === i}
                    >
                      <span className="material-symbols-outlined" style={{ fontSize: 16 }}>
                        {expandedIdx === i ? 'expand_less' : 'expand_more'}
                      </span>
                    </button>
                  </td>
                </tr>
                {expandedIdx === i && (
                  <tr key={`disk-detail-${i}`} style={{ borderTop: '1px solid #f3f4f6' }}>
                    <td colSpan={7} className="px-4 py-3 bg-[#fafafa]">
                      <div className="flex flex-wrap gap-6 text-[13px]">
                        <div>
                          <span className="text-[#878a99] mr-2">S.M.A.R.T. 重分配扇区:</span>
                          <span
                            className="font-mono font-semibold"
                            style={{ color: d.reallocated_sectors > 0 ? '#ef4444' : '#2ca07a' }}
                          >
                            {d.reallocated_sectors}
                          </span>
                          {d.reallocated_sectors > 0 && (
                            <span className="ml-2 text-[11px] text-[#ef4444]">警告：磁盘可能存在坏扇区</span>
                          )}
                        </div>
                        <div>
                          <span className="text-[#878a99] mr-2">通电时长:</span>
                          <span className="font-mono font-semibold text-[#495057]">
                            {d.power_on_hours > 0 ? `${d.power_on_hours.toLocaleString()} 小时` : '—'}
                          </span>
                        </div>
                        <div>
                          <span className="text-[#878a99] mr-2">容量:</span>
                          <span className="font-mono font-semibold text-[#495057]">
                            {d.size > 0 ? formatBytes(d.size) : '—'}
                          </span>
                        </div>
                      </div>
                    </td>
                  </tr>
                )}
              </>
            ))}
          </tbody>
        </table>
      </div>
    </SectionCard>
  )
}

// ─── UPS section ──────────────────────────────────────────────────────────────

function UpsSection({ ups }: { ups: NonNullable<NasMetrics['ups']> }) {
  return (
    <SectionCard title="UPS 电源">
      <div className="flex flex-wrap gap-6">
        <div className="flex items-center gap-3 p-4 rounded-lg flex-1 min-w-[200px]"
          style={{ background: '#f8f9fa', border: '1px solid #f0f0f0' }}>
          <div
            className="w-10 h-10 rounded-full flex items-center justify-center shrink-0"
            style={{ background: 'rgba(245,158,11,0.12)' }}
          >
            <span className="material-symbols-outlined text-[#f59e0b]" style={{ fontSize: 22 }}>
              battery_charging_full
            </span>
          </div>
          <div>
            <div className="text-[11px] text-[#878a99]">状态</div>
            <div className="text-[15px] font-semibold text-[#495057]">{ups.status}</div>
          </div>
        </div>

        <div className="flex items-center gap-3 p-4 rounded-lg flex-1 min-w-[200px]"
          style={{ background: '#f8f9fa', border: '1px solid #f0f0f0' }}>
          <div
            className="w-10 h-10 rounded-full flex items-center justify-center shrink-0"
            style={{ background: 'rgba(44,160,122,0.12)' }}
          >
            <span className="material-symbols-outlined text-[#2ca07a]" style={{ fontSize: 22 }}>
              battery_full
            </span>
          </div>
          <div className="flex-1">
            <div className="text-[11px] text-[#878a99]">电池电量</div>
            <div className="flex items-center gap-2 mt-0.5">
              <span
                className="text-[22px] font-bold leading-tight"
                style={{ color: ups.battery_percent < 20 ? '#ef4444' : ups.battery_percent < 50 ? '#f59e0b' : '#2ca07a' }}
              >
                {ups.battery_percent}%
              </span>
            </div>
            <ProgressBar value={ups.battery_percent} className="mt-1 w-24" />
          </div>
        </div>

        {ups.model && (
          <div className="flex items-center gap-3 p-4 rounded-lg flex-1 min-w-[200px]"
            style={{ background: '#f8f9fa', border: '1px solid #f0f0f0' }}>
            <div
              className="w-10 h-10 rounded-full flex items-center justify-center shrink-0"
              style={{ background: 'rgba(99,102,241,0.12)' }}
            >
              <span className="material-symbols-outlined text-[#6366f1]" style={{ fontSize: 22 }}>
                electrical_services
              </span>
            </div>
            <div>
              <div className="text-[11px] text-[#878a99]">型号</div>
              <div className="text-[14px] font-semibold text-[#495057]">{ups.model}</div>
            </div>
          </div>
        )}
      </div>
    </SectionCard>
  )
}

// ─── History charts section ───────────────────────────────────────────────────

function HistorySection({ nasId, disks }: { nasId: number; disks: NasDisk[] }) {
  const [timeRange, setTimeRange] = useState<TimeRange>('1h')
  const [timeWindow, setTimeWindow] = useState(() => calcWindow('1h'))

  function handleRangeChange(r: TimeRange) { setTimeRange(r); setTimeWindow(calcWindow(r)) }
  function handleRefresh() { setTimeWindow(calcWindow(timeRange)) }

  const id = String(nasId)
  const hasDiskTemps = disks.some((d) => d.temperature > 0)

  return (
    <div
      className="bg-white"
      style={{ borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.05)', padding: '20px 24px' }}
    >
      <div className="flex items-center justify-between mb-5">
        <h5 className="mb-0 font-semibold text-[15px] text-[#212529]">历史趋势</h5>
        <div className="flex items-center gap-2">
          <div
            className="flex"
            style={{ border: '1px solid #dee2e6', borderRadius: 6, overflow: 'hidden' }}
          >
            {(['1h', '6h', '24h', '7d'] as TimeRange[]).map((r, i) => (
              <button
                key={r}
                onClick={() => handleRangeChange(r)}
                style={{
                  padding: '3px 12px',
                  fontSize: 13,
                  fontWeight: 500,
                  background: timeRange === r ? '#2ca07a' : 'white',
                  color: timeRange === r ? 'white' : '#495057',
                  borderRight: i < 3 ? '1px solid #dee2e6' : 'none',
                  cursor: 'pointer',
                  transition: 'background 0.15s, color 0.15s',
                }}
              >
                {r}
              </button>
            ))}
          </div>
          <button
            onClick={handleRefresh}
            className="flex items-center justify-center transition-colors hover:text-gray-700"
            style={{ color: '#878a99', background: 'none', border: 'none', cursor: 'pointer', padding: 4 }}
            title="刷新"
          >
            <span className="material-symbols-outlined" style={{ fontSize: 20 }}>refresh</span>
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <HistoryChart
          title="CPU 使用率"
          queries={[{ query: `mantisops_nas_cpu_usage_percent{nas_id="${id}"}`, label: 'CPU', color: '#2ca07a' }]}
          {...timeWindow}
          unit="%"
        />
        <HistoryChart
          title="内存使用率"
          queries={[{ query: `mantisops_nas_memory_usage_percent{nas_id="${id}"}`, label: '内存', color: '#f7b84b' }]}
          {...timeWindow}
          unit="%"
        />
        <HistoryChart
          title="网络流量"
          queries={[
            { query: `sum(mantisops_nas_network_rx_bytes_per_sec{nas_id="${id}"})`, label: '入站', color: '#0ab39c' },
            { query: `sum(mantisops_nas_network_tx_bytes_per_sec{nas_id="${id}"})`, label: '出站', color: '#f7b84b' },
          ]}
          {...timeWindow}
          chartType="line"
          formatValue={(v) => formatBytesPS(v)}
        />
        {hasDiskTemps && (
          <HistoryChart
            title="磁盘温度"
            queries={disks
              .filter((d) => d.temperature > 0)
              .map((d) => ({
                query: `mantisops_nas_disk_temperature{nas_id="${id}",disk="${d.name}"}`,
                label: d.name,
                color: '#f97316',
              }))}
            {...timeWindow}
            chartType="line"
            unit="°C"
          />
        )}
      </div>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function NASDetail() {
  const { id } = useParams<{ id: string }>()
  const nasId = id ? parseInt(id, 10) : null

  const { devices, metrics: storeMetrics, fetchDevices, updateMetrics, updateStatus } = useNasStore()
  const [localMetrics, setLocalMetrics] = useState<NasMetrics | null>(null)
  const [loadError, setLoadError] = useState(false)
  const [metricsLoading, setMetricsLoading] = useState(false)

  // Find device from store
  const device = nasId != null ? devices.find((d) => d.id === nasId) ?? null : null
  const storeMetric = nasId != null ? storeMetrics[nasId] : undefined

  // Merge: prefer real-time store metrics, fall back to locally fetched
  const metrics: NasMetrics | null = storeMetric ?? localMetrics

  // Load devices if not loaded
  useEffect(() => {
    if (devices.length === 0) {
      fetchDevices().catch(() => setLoadError(true))
    }
  }, [devices.length, fetchDevices])

  // Load metrics directly if not in store
  useEffect(() => {
    if (nasId == null) return
    if (storeMetric) return // already in store
    setMetricsLoading(true)
    getNasMetrics(nasId)
      .then((m) => { setLocalMetrics(m); updateMetrics(nasId, m) })
      .catch(() => {})
      .finally(() => setMetricsLoading(false))
  }, [nasId, storeMetric, updateMetrics])

  // Real-time WebSocket events
  useEffect(() => {
    const onMetrics = (e: Event) => {
      const ev = e as CustomEvent<{ nas_id: number } & Record<string, unknown>>
      if (ev.detail?.nas_id === nasId) {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        updateMetrics(ev.detail.nas_id, ev.detail as any)
      }
    }
    const onStatus = (e: Event) => {
      const ev = e as CustomEvent<{ nas_id: number; status: string }>
      if (ev.detail?.nas_id === nasId) {
        updateStatus(ev.detail.nas_id, ev.detail.status)
      }
    }
    window.addEventListener('nas_metrics', onMetrics)
    window.addEventListener('nas_status', onStatus)
    return () => {
      window.removeEventListener('nas_metrics', onMetrics)
      window.removeEventListener('nas_status', onStatus)
    }
  }, [nasId, updateMetrics, updateStatus])

  // ── Error / loading states ──

  if (loadError || (devices.length > 0 && device === null)) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="flex flex-col items-center gap-4">
          <span className="material-symbols-outlined text-4xl" style={{ color: '#f06548' }}>error</span>
          <span className="text-sm font-medium text-[#878a99]">NAS 设备不存在或加载失败</span>
          <Link to="/nas" className="text-sm hover:underline text-[#2ca07a]">返回 NAS 列表</Link>
        </div>
      </div>
    )
  }

  if (devices.length === 0 || device === null) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="flex flex-col items-center gap-4">
          <div className="w-10 h-10 border-2 rounded-full animate-spin"
            style={{ borderColor: 'rgba(44,160,122,0.3)', borderTopColor: '#2ca07a' }} />
          <span className="text-sm font-medium tracking-wide text-[#878a99]">加载 NAS 数据...</span>
        </div>
      </div>
    )
  }

  const sysInfo = parseSystemInfo(device.system_info)

  const cpu    = metrics?.cpu?.usage_percent ?? 0
  const mem    = metrics?.memory?.usage_percent ?? 0
  const memUsed  = metrics?.memory?.used ?? 0
  const memTotal = metrics?.memory?.total ?? 0
  const hasMem   = !!metrics?.memory

  const networks = metrics?.networks ?? []
  const totalRx  = networks.reduce((s, n) => s + (n.rx_bytes_per_sec ?? 0), 0)
  const totalTx  = networks.reduce((s, n) => s + (n.tx_bytes_per_sec ?? 0), 0)

  const raids   = metrics?.raids   ?? []
  const volumes = metrics?.volumes ?? []
  const disks   = metrics?.disks   ?? []
  const ups     = metrics?.ups

  const infoRows = [
    { label: '设备名称',   value: device.name },
    { label: '类型',       value: device.nas_type === 'synology' ? 'Synology' : 'fnOS' },
    { label: 'IP / 主机',  value: device.host, mono: true },
    { label: 'SSH 端口',   value: String(device.port), mono: true },
    { label: 'SSH 用户',   value: device.ssh_user, mono: true },
    { label: '型号',       value: sysInfo.model || '—' },
    { label: '序列号',     value: sysInfo.serial || '—', mono: true },
    { label: '系统版本',   value: sysInfo.os_version || '—' },
    { label: '采集间隔',   value: `${device.collect_interval} 秒` },
    { label: '最后上报',   value: device.last_seen
        ? new Date(device.last_seen).toLocaleString('zh-CN', { hour12: false })
        : '—' },
  ]

  // Synology packages (from system_info)
  const hasPackages =
    device.nas_type === 'synology' &&
    Array.isArray(sysInfo.packages) &&
    sysInfo.packages.length > 0

  return (
    <div style={{ color: '#495057' }}>

      {/* ── Header ── */}
      <div
        className="flex items-center gap-3 mb-6 pb-4"
        style={{ borderBottom: '1px solid #e9ecef' }}
      >
        <Link
          to="/nas"
          className="flex items-center justify-center w-8 h-8 rounded-lg transition-colors hover:bg-gray-100"
          style={{ color: '#495057' }}
          aria-label="返回 NAS 列表"
        >
          <span className="material-symbols-outlined" style={{ fontSize: 22 }}>arrow_back</span>
        </Link>

        <div className="flex-1 min-w-0">
          <div className="font-semibold uppercase tracking-widest mb-1 text-[11px] text-[#878a99]">
            NAS 详情
          </div>
          <div className="flex items-center gap-2 flex-wrap">
            <h3
              className="mb-0 font-bold text-[22px] text-[#212529]"
              style={{ fontFamily: 'var(--font-heading, inherit)' }}
            >
              {device.name}
            </h3>
            <NasTypeBadge type={device.nas_type} />
            {sysInfo.os_version && (
              <span className="text-[12px] text-[#878a99] hidden sm:inline">{sysInfo.os_version}</span>
            )}
            <StatusBadge status={device.status} />
          </div>
        </div>

        {metricsLoading && (
          <span className="text-[12px] text-[#878a99] flex items-center gap-1">
            <span className="material-symbols-outlined animate-spin" style={{ fontSize: 14 }}>
              progress_activity
            </span>
            加载中
          </span>
        )}
      </div>

      {/* ── Bento Grid: device info + realtime overview ── */}
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-5 mb-5">

        {/* Left — device info */}
        <div
          className="bg-white"
          style={{ borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.05)', padding: '20px 24px' }}
        >
          <h5 className="font-semibold mb-4 text-[15px] text-[#212529]">设备信息</h5>
          <ul className="m-0 p-0 list-none">
            {infoRows.map((row) => (
              <li
                key={row.label}
                className="flex items-center justify-between gap-2 py-2"
                style={{ borderBottom: '1px solid #f3f4f6', fontSize: 14, color: '#6c757d' }}
              >
                <span className="font-medium text-[#212529] whitespace-nowrap">{row.label}</span>
                <span
                  className="text-right truncate"
                  style={row.mono ? { fontFamily: 'monospace', fontSize: 13 } : undefined}
                >
                  {row.value || '—'}
                </span>
              </li>
            ))}
          </ul>
        </div>

        {/* Right — real-time overview */}
        <div
          className="xl:col-span-2 bg-white"
          style={{ borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.05)', padding: '20px 24px' }}
        >
          <div className="flex items-center justify-between mb-4">
            <h5 className="font-semibold mb-0 text-[15px] text-[#212529]">实时概览</h5>
            <span
              className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded border text-xs font-medium"
              style={{ background: '#f8f9fa', borderColor: '#dee2e6', color: '#878a99' }}
            >
              <span
                className="inline-block rounded-full"
                style={{ width: 6, height: 6, background: '#0ab39c', boxShadow: '0 0 5px rgba(10,179,156,0.7)' }}
              />
              实时
            </span>
          </div>

          {metrics ? (
            <div className="flex flex-col gap-5">
              {/* CPU + Memory bars */}
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div
                  className="p-4 rounded-lg"
                  style={{ background: '#f8f9fa', border: '1px solid #eeeeee' }}
                >
                  <div className="text-[12px] font-semibold uppercase text-[#878a99] tracking-wide mb-3">
                    CPU 使用率
                  </div>
                  <div className="text-[28px] font-bold text-[#2ca07a] leading-tight mb-2">
                    {cpu.toFixed(1)}%
                  </div>
                  <ProgressBar value={cpu} />
                </div>

                <div
                  className="p-4 rounded-lg"
                  style={{ background: '#f8f9fa', border: '1px solid #eeeeee' }}
                >
                  <div className="text-[12px] font-semibold uppercase text-[#878a99] tracking-wide mb-3">
                    内存使用率
                  </div>
                  {hasMem ? (
                    <>
                      <div className="text-[28px] font-bold text-[#f7b84b] leading-tight mb-1">
                        {mem.toFixed(1)}%
                      </div>
                      <div className="text-[11px] text-[#878a99] mb-2">
                        {formatBytes(memUsed)} / {formatBytes(memTotal)}
                      </div>
                      <ProgressBar value={mem} />
                    </>
                  ) : (
                    <div className="text-[14px] text-[#878a99]">暂无数据</div>
                  )}
                </div>
              </div>

              {/* Network throughput */}
              {networks.length > 0 && (
                <div>
                  <div className="text-[12px] font-semibold uppercase text-[#878a99] tracking-wide mb-3">
                    网络流量
                  </div>
                  <div className="flex flex-wrap gap-4">
                    <div
                      className="flex items-center gap-3 px-4 py-3 rounded-lg flex-1"
                      style={{ background: 'rgba(240,101,72,0.06)', border: '1px solid rgba(240,101,72,0.15)' }}
                    >
                      <span className="material-symbols-outlined text-[#f06548]" style={{ fontSize: 20 }}>
                        arrow_downward
                      </span>
                      <div>
                        <div className="text-[11px] text-[#878a99]">下载</div>
                        <div className="font-mono font-semibold text-[15px] text-[#f06548]">
                          {formatBytesPS(totalRx)}
                        </div>
                      </div>
                    </div>
                    <div
                      className="flex items-center gap-3 px-4 py-3 rounded-lg flex-1"
                      style={{ background: 'rgba(44,160,122,0.06)', border: '1px solid rgba(44,160,122,0.15)' }}
                    >
                      <span className="material-symbols-outlined text-[#2ca07a]" style={{ fontSize: 20 }}>
                        arrow_upward
                      </span>
                      <div>
                        <div className="text-[11px] text-[#878a99]">上传</div>
                        <div className="font-mono font-semibold text-[15px] text-[#2ca07a]">
                          {formatBytesPS(totalTx)}
                        </div>
                      </div>
                    </div>
                    {networks.length > 1 && (
                      <div className="flex-1 min-w-[120px]">
                        <div className="text-[11px] text-[#878a99] mb-1.5">各网卡</div>
                        {networks.map((n, i) => (
                          <div key={i} className="flex items-center gap-2 text-[11px] font-mono text-[#878a99]">
                            <span className="text-[#495057] w-16 truncate">{n.interface}</span>
                            <span className="text-[#f06548]">↓{formatBytesPS(n.rx_bytes_per_sec)}</span>
                            <span className="text-[#2ca07a]">↑{formatBytesPS(n.tx_bytes_per_sec)}</span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="flex items-center justify-center h-32 text-[14px] text-[#878a99] italic">
              {device.status === 'offline' ? '设备离线，暂无监控数据' : '暂无指标数据'}
            </div>
          )}
        </div>
      </div>

      {/* ── RAID section ── */}
      {raids.length > 0 && (
        <div className="mb-5">
          <RaidSection raids={raids} />
        </div>
      )}

      {/* ── Volumes section ── */}
      {volumes.length > 0 && (
        <div className="mb-5">
          <VolumesSection volumes={volumes} />
        </div>
      )}

      {/* ── Disk health section ── */}
      {disks.length > 0 && (
        <div className="mb-5">
          <DiskHealthSection disks={disks} />
        </div>
      )}

      {/* ── UPS section (conditional) ── */}
      {ups && (
        <div className="mb-5">
          <UpsSection ups={ups} />
        </div>
      )}

      {/* ── Synology packages section ── */}
      {hasPackages && (
        <div className="mb-5">
          <SectionCard title="Synology 套件">
            <div className="flex flex-wrap gap-2">
              {(sysInfo.packages as Array<{ name?: string; version?: string }>).map((pkg, i) => (
                <span
                  key={i}
                  className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[12px]"
                  style={{ background: 'rgba(44,160,122,0.08)', color: '#2ca07a', border: '1px solid rgba(44,160,122,0.2)' }}
                >
                  <span className="material-symbols-outlined" style={{ fontSize: 13 }}>widgets</span>
                  <span>{pkg.name || '—'}</span>
                  {pkg.version && (
                    <span className="text-[10px] text-[#878a99]">{pkg.version}</span>
                  )}
                </span>
              ))}
            </div>
          </SectionCard>
        </div>
      )}

      {/* Synology note when no packages yet */}
      {device.nas_type === 'synology' && !hasPackages && (
        <div className="mb-5">
          <SectionCard title="Synology 套件">
            <div className="flex items-center gap-2 text-[13px] text-[#878a99] italic">
              <span className="material-symbols-outlined" style={{ fontSize: 16 }}>info</span>
              套件信息将在首次连接采集后显示
            </div>
          </SectionCard>
        </div>
      )}

      {/* ── History charts section ── */}
      {nasId != null && (
        <HistorySection nasId={nasId} disks={disks} />
      )}

    </div>
  )
}
