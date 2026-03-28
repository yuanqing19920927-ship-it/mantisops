import { useParams, Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { useServerStore } from '../../stores/serverStore'
import { getServer, updateServerName, getAssets, type AssetInfo } from '../../api/client'
import api from '../../api/client'
import { HistoryChart } from '../../components/HistoryChart'
import type { Server } from '../../types'
import { formatBytes, formatBytesPS, timeSince } from '../../utils/format'

// Threshold-aware color for metric values
function metricColor(value: number, warnAt: number, dangerAt: number): string {
  if (value >= dangerAt) return '#f06548'
  if (value >= warnAt) return '#f7b84b'
  return '#0ab39c'
}

export default function ServerDetail() {
  const { id } = useParams<{ id: string }>()
  const [server, setServer] = useState<Server | null>(null)
  const [loadError, setLoadError] = useState(false)
  const metrics = useServerStore((s) => id ? s.metrics[id] : undefined)

  type TimeRange = '1h' | '6h' | '24h' | '7d'
  const [timeRange, setTimeRange] = useState<TimeRange>('1h')
  const [timeWindow, setTimeWindow] = useState(() => calcWindow('1h'))
  const [editing, setEditing] = useState(false)
  const [editName, setEditName] = useState('')
  const [assets, setAssets] = useState<AssetInfo[]>([])
  const [showConfig, setShowConfig] = useState(false)
  const [cfgDocker, setCfgDocker] = useState(false)
  const [cfgGPU, setCfgGPU] = useState(false)
  const [cfgSaving, setCfgSaving] = useState(false)
  const [cfgSaved, setCfgSaved] = useState(false)

  function calcWindow(range: TimeRange) {
    const now = Math.floor(Date.now() / 1000)
    const durations: Record<TimeRange, number> = { '1h': 3600, '6h': 21600, '24h': 86400, '7d': 604800 }
    const steps: Record<TimeRange, number> = { '1h': 15, '6h': 60, '24h': 300, '7d': 1800 }
    return { start: now - durations[range], end: now, step: steps[range] }
  }
  function handleRangeChange(r: TimeRange) { setTimeRange(r); setTimeWindow(calcWindow(r)) }
  function handleRefresh() { setTimeWindow(calcWindow(timeRange)) }

  useEffect(() => {
    if (id) {
      setLoadError(false)
      getServer(id).then(setServer).catch(() => setLoadError(true))
      getAssets().then(setAssets).catch(() => {})
    }
  }, [id])

  if (loadError) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="flex flex-col items-center gap-4">
          <span className="material-symbols-outlined text-4xl" style={{ color: '#f06548' }}>error</span>
          <span className="text-sm font-medium" style={{ color: '#878a99' }}>
            服务器不存在或加载失败
          </span>
          <Link to="/servers" className="text-sm hover:underline" style={{ color: '#2ca07a' }}>
            返回服务器列表
          </Link>
        </div>
      </div>
    )
  }

  if (!server) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="flex flex-col items-center gap-4">
          <div className="w-10 h-10 border-2 rounded-full animate-spin" style={{ borderColor: 'rgba(44,160,122,0.3)', borderTopColor: '#2ca07a' }} />
          <span className="text-sm font-medium tracking-wide" style={{ color: '#878a99' }}>
            加载服务器数据...
          </span>
        </div>
      </div>
    )
  }

  const cpu = metrics?.cpu
  const mem = metrics?.memory
  const disk = metrics?.disks?.[0]
  const nets = metrics?.networks || []
  const rxTotal = nets.reduce((s, n) => s + (n.rx_bytes_per_sec ?? 0), 0)
  const txTotal = nets.reduce((s, n) => s + (n.tx_bytes_per_sec ?? 0), 0)
  const containers = metrics?.containers || []
  const gpu = metrics?.gpu

  const serverName = server.display_name || server.hostname
  const isOnline = server.status === 'online'

  const handleSaveName = async () => {
    if (!id || !editName.trim()) return
    await updateServerName(id, editName.trim())
    setServer({ ...server, display_name: editName.trim() })
    setEditing(false)
  }

  let ipDisplay = '-'
  try {
    const parsed = JSON.parse(server.ip_addresses)
    if (Array.isArray(parsed) && parsed.length > 0) ipDisplay = parsed[0]
  } catch {
    // ignore
  }

  const infoRows = [
    { label: '操作系统', value: server.os },
    { label: '内核', value: server.kernel },
    { label: '处理器', value: server.cpu_model || `${server.cpu_cores} 核` },
    { label: '内存总量', value: formatBytes(server.memory_total) },
    { label: '磁盘总量', value: formatBytes(server.disk_total) },
    ...(server.gpu_model ? [{ label: 'GPU', value: server.gpu_model }] : []),
    { label: 'IP 地址', value: ipDisplay, mono: true },
    { label: '架构', value: server.arch || '-' },
    { label: 'Agent 版本', value: server.agent_version || '-' },
    { label: '最后心跳', value: server.last_seen ? timeSince(server.last_seen) : '-' },
  ]

  const cpuPct = cpu?.usage_percent ?? 0
  const memPct = mem?.usage_percent ?? 0
  const diskPct = disk?.usage_percent ?? 0
  const runningContainers = containers.filter(c => c.state === 'running').length

  const serverAssets = assets.filter(a => a.server_id === server.id)

  return (
    <div style={{ color: '#495057' }}>

      {/* ── Header ── */}
      <div
        className="flex items-center gap-3 mb-6 pb-4"
        style={{ borderBottom: '1px solid #e9ecef' }}
      >
        <Link
          to="/servers"
          className="flex items-center justify-center w-8 h-8 rounded-lg transition-colors hover:bg-gray-100"
          style={{ color: '#495057' }}
        >
          <span className="material-symbols-outlined" style={{ fontSize: 22 }}>arrow_back</span>
        </Link>

        <div className="flex-1 min-w-0">
          <div
            className="font-semibold uppercase tracking-widest mb-1"
            style={{ fontSize: 11, color: '#878a99' }}
          >
            服务器详情
          </div>

          <div className="flex items-center gap-2 flex-wrap">
            {editing ? (
              <>
                <input
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleSaveName()
                    if (e.key === 'Escape') setEditing(false)
                  }}
                  className="border rounded-lg px-3 py-1 text-lg font-bold outline-none"
                  style={{
                    fontFamily: 'var(--font-heading, inherit)',
                    borderColor: '#dee2e6',
                    color: '#212529',
                    maxWidth: 280,
                  }}
                  autoFocus
                />
                <button
                  onClick={handleSaveName}
                  className="flex items-center justify-center w-7 h-7 rounded-lg transition-colors"
                  style={{ background: 'rgba(10,179,156,0.12)', color: '#0ab39c' }}
                >
                  <span className="material-symbols-outlined" style={{ fontSize: 16 }}>check</span>
                </button>
                <button
                  onClick={() => { setEditing(false); setEditName(serverName) }}
                  className="flex items-center justify-center w-7 h-7 rounded-lg transition-colors hover:bg-gray-100"
                  style={{ color: '#878a99' }}
                >
                  <span className="material-symbols-outlined" style={{ fontSize: 16 }}>close</span>
                </button>
              </>
            ) : (
              <>
                <h3
                  className="mb-0 font-bold"
                  style={{ fontFamily: 'var(--font-heading, inherit)', fontSize: 22, color: '#212529' }}
                >
                  {serverName}
                </h3>
                <button
                  onClick={() => { setEditName(serverName); setEditing(true) }}
                  className="flex items-center justify-center transition-colors hover:text-gray-600"
                  style={{ color: '#878a99', padding: 0, background: 'none', border: 'none', cursor: 'pointer' }}
                  title="重命名"
                >
                  <span className="material-symbols-outlined" style={{ fontSize: 18 }}>edit</span>
                </button>
              </>
            )}

            {/* Online / offline badge */}
            <span
              className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-sm font-medium"
              style={
                isOnline
                  ? { background: 'rgba(10,179,156,0.12)', color: '#0ab39c' }
                  : { background: 'rgba(240,101,72,0.12)', color: '#f06548' }
              }
            >
              {/* glow dot */}
              <span
                className="inline-block rounded-full"
                style={{
                  width: 7,
                  height: 7,
                  background: isOnline ? '#0ab39c' : '#f06548',
                  boxShadow: isOnline
                    ? '0 0 6px rgba(10,179,156,0.7)'
                    : '0 0 6px rgba(240,101,72,0.7)',
                }}
              />
              {isOnline ? 'online' : 'offline'}
            </span>

            {/* Config button */}
            <button
              onClick={() => {
                setCfgDocker(server.collect_docker ?? (metrics?.containers !== undefined))
                setCfgGPU(server.collect_gpu ?? !!server.gpu_model)
                setCfgSaved(false)
                setShowConfig(true)
              }}
              className="flex items-center gap-1 px-2.5 py-1 text-[12px] border rounded transition-colors ml-2"
              style={{ borderColor: '#ced4da', color: '#878a99' }}
              onMouseEnter={e => { e.currentTarget.style.borderColor = '#2ca07a'; e.currentTarget.style.color = '#2ca07a' }}
              onMouseLeave={e => { e.currentTarget.style.borderColor = '#ced4da'; e.currentTarget.style.color = '#878a99' }}
            >
              <span className="material-symbols-outlined" style={{ fontSize: 15 }}>settings</span>
              配置
            </button>
          </div>
        </div>
      </div>

      {/* ── Agent Config Dialog ── */}
      {showConfig && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setShowConfig(false)}>
          <div className="bg-white rounded-xl shadow-xl w-[420px] max-w-[90vw] overflow-hidden" onClick={e => e.stopPropagation()}>
            <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
              <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
                <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">settings</span>
              </div>
              <h3 className="text-sm font-semibold text-[#495057]">Agent 采集配置</h3>
            </div>

            <div className="px-5 py-5 space-y-4">
              {/* Docker toggle */}
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-[13px] font-medium text-[#495057]">Docker 容器监控</div>
                  <div className="text-[11px] text-[#878a99] mt-0.5">采集容器 CPU、内存、状态等指标</div>
                </div>
                <button onClick={() => setCfgDocker(!cfgDocker)}
                  className={`relative w-10 h-5 rounded-full transition-colors ${cfgDocker ? 'bg-[#2ca07a]' : 'bg-[#ced4da]'}`}>
                  <span className={`absolute top-0.5 left-0.5 w-4 h-4 bg-white rounded-full shadow transition-transform ${cfgDocker ? 'translate-x-5' : ''}`} />
                </button>
              </div>

              {/* GPU toggle */}
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-[13px] font-medium text-[#495057]">GPU 监控</div>
                  <div className="text-[11px] text-[#878a99] mt-0.5">采集 GPU 使用率、显存、温度（需 nvidia-smi）</div>
                </div>
                <button onClick={() => setCfgGPU(!cfgGPU)}
                  className={`relative w-10 h-5 rounded-full transition-colors ${cfgGPU ? 'bg-[#2ca07a]' : 'bg-[#ced4da]'}`}>
                  <span className={`absolute top-0.5 left-0.5 w-4 h-4 bg-white rounded-full shadow transition-transform ${cfgGPU ? 'translate-x-5' : ''}`} />
                </button>
              </div>

              {/* Status info */}
              <div className="bg-[#f8f9fa] rounded-lg p-3 text-[11px] text-[#878a99] space-y-1">
                <div className="flex items-center gap-2">
                  <span className={`w-1.5 h-1.5 rounded-full ${isOnline ? 'bg-[#0ab39c]' : 'bg-[#f06548]'}`} />
                  Agent 状态: {isOnline ? '在线' : '离线'}
                </div>
                <div>Agent 版本: {server.agent_version || '-'}</div>
                <div>最后心跳: {server.last_seen ? timeSince(server.last_seen) : '-'}</div>
                {server.gpu_model && <div>GPU: {server.gpu_model}</div>}
              </div>

              <div className="bg-[#fff8e1] rounded-lg p-3 text-[11px] text-[#856404] flex gap-2">
                <span className="material-symbols-outlined text-[14px] mt-0.5 shrink-0">info</span>
                <span>配置保存后需重启 Agent 才能生效。托管服务器可通过「重新部署」应用配置。</span>
              </div>
            </div>

            <div className="px-5 py-3 border-t border-[#e9ebec] flex items-center justify-between">
              {cfgSaved && (
                <span className="text-[12px] text-[#0ab39c] flex items-center gap-1">
                  <span className="material-symbols-outlined" style={{ fontSize: 14 }}>check_circle</span>
                  已保存，重新部署 Agent 后生效
                </span>
              )}
              {!cfgSaved && <span />}
              <div className="flex items-center gap-2">
                <button onClick={() => setShowConfig(false)}
                  className="text-[12px] px-4 py-2 border border-[#ced4da] text-[#878a99] rounded-lg hover:bg-[#f8f9fa] transition-colors">
                  关闭
                </button>
                <button
                  disabled={cfgSaving}
                  onClick={async () => {
                    setCfgSaving(true)
                    try {
                      await api.put(`/servers/${id}/config`, { collect_docker: cfgDocker, collect_gpu: cfgGPU })
                      setCfgSaved(true)
                      // Update local server state
                      setServer({ ...server, collect_docker: cfgDocker, collect_gpu: cfgGPU })
                    } catch (err) {
                      console.error('[config] save:', err)
                    }
                    setCfgSaving(false)
                  }}
                  className="text-[12px] px-4 py-2 bg-[#2ca07a] text-white rounded-lg hover:bg-[#248a69] transition-colors disabled:opacity-50">
                  {cfgSaving ? '保存中...' : '保存配置'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ── Bento Grid ── */}
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-5 mb-5">

        {/* Left 1/3 — 系统信息 */}
        <div
          className="bg-white"
          style={{ borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.05)', padding: '20px 24px' }}
        >
          <h5
            className="font-semibold mb-4"
            style={{ fontSize: 15, color: '#212529' }}
          >
            系统信息
          </h5>

          <ul className="m-0 p-0 list-none">
            {infoRows.map((row) => (
              <li
                key={row.label}
                className="flex items-center justify-between gap-2 py-2"
                style={{ borderBottom: '1px solid #f3f4f6', fontSize: 14, color: '#6c757d' }}
              >
                <span className="font-medium" style={{ color: '#212529', whiteSpace: 'nowrap' }}>
                  {row.label}
                </span>
                <span
                  className="text-right truncate"
                  style={row.mono ? { fontFamily: 'monospace' } : undefined}
                >
                  {row.value || '-'}
                </span>
              </li>
            ))}
          </ul>
        </div>

        {/* Right 2/3 — 实时概览 */}
        <div
          className="xl:col-span-2 bg-white"
          style={{ borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.05)', padding: '20px 24px' }}
        >
          <div className="flex items-center justify-between mb-4">
            <h5 className="font-semibold mb-0" style={{ fontSize: 15, color: '#212529' }}>
              实时概览
            </h5>
            {/* Live badge */}
            <span
              className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded border text-xs font-medium"
              style={{ background: '#f8f9fa', borderColor: '#dee2e6', color: '#878a99' }}
            >
              <span
                className="inline-block rounded-full"
                style={{
                  width: 6,
                  height: 6,
                  background: '#0ab39c',
                  boxShadow: '0 0 5px rgba(10,179,156,0.7)',
                }}
              />
              实时
            </span>
          </div>

          {/* 3×3 metric grid */}
          <div
            className="grid gap-4"
            style={{ gridTemplateColumns: 'repeat(3, 1fr)' }}
          >
            {/* CPU */}
            <MetricCard
              label="CPU 使用率"
              value={`${cpuPct.toFixed(1)}%`}
              valueColor={metricColor(cpuPct, 70, 90)}
              subtitle={cpu ? `负载: ${(cpu.load1 ?? 0).toFixed(2)} / ${(cpu.load5 ?? 0).toFixed(2)} / ${(cpu.load15 ?? 0).toFixed(2)}` : undefined}
              highlightBorder={cpuPct >= 70 ? (cpuPct >= 90 ? 'danger' : 'warning') : undefined}
            />

            {/* Memory */}
            <MetricCard
              label="内存"
              value={`${memPct.toFixed(1)}%`}
              valueColor={metricColor(memPct, 80, 95)}
              subtitle={
                mem
                  ? `${formatBytes(mem.used)} / ${formatBytes(mem.total)}${mem.swap_total > 0 ? `  交换: ${((mem.swap_used / mem.swap_total) * 100).toFixed(0)}%` : ''}`
                  : undefined
              }
              highlightBorder={memPct >= 80 ? (memPct >= 95 ? 'danger' : 'warning') : undefined}
            />

            {/* Disk */}
            <MetricCard
              label="磁盘"
              value={`${diskPct.toFixed(1)}%`}
              valueColor={metricColor(diskPct, 80, 95)}
              subtitle={disk ? `${formatBytes(disk.used)} / ${formatBytes(disk.total)}` : undefined}
              highlightBorder={diskPct >= 80 ? (diskPct >= 95 ? 'danger' : 'warning') : undefined}
            />

            {/* Net In */}
            <MetricCard
              label="网络入站"
              value={formatBytesPS(rxTotal)}
              subtitle={`${nets.length} 网卡`}
            />

            {/* Net Out */}
            <MetricCard
              label="网络出站"
              value={formatBytesPS(txTotal)}
            />

            {/* Containers */}
            <MetricCard
              label="容器"
              value={String(runningContainers)}
              valueColor="#2ca07a"
              subtitle={`${runningContainers} / ${containers.length} 运行中`}
              highlightBorder={containers.length > 0 ? 'primary' : undefined}
            />

            {/* GPU (conditional) */}
            {gpu && (
              <>
                <MetricCard
                  label="GPU 使用率"
                  value={`${(gpu.usage_percent ?? 0).toFixed(1)}%`}
                  valueColor={metricColor(gpu.usage_percent ?? 0, 70, 90)}
                  subtitle={gpu.name}
                />
                <MetricCard
                  label="GPU 显存"
                  value={formatBytes(gpu.memory_used)}
                  subtitle={`/ ${formatBytes(gpu.memory_total)}`}
                />
                <MetricCard
                  label="GPU 温度"
                  value={`${gpu.temperature}°C`}
                  valueColor={gpu.temperature >= 80 ? '#f06548' : gpu.temperature >= 60 ? '#f7b84b' : '#0ab39c'}
                  highlightBorder={gpu.temperature >= 80 ? 'danger' : gpu.temperature >= 60 ? 'warning' : undefined}
                />
              </>
            )}
          </div>
        </div>
      </div>

      {/* ── 运行业务 ── */}
      {serverAssets.length > 0 && (
        <div
          className="bg-white overflow-hidden mb-5"
          style={{ borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.05)' }}
        >
          <div className="flex items-center justify-between px-6 py-4" style={{ borderBottom: '1px solid #f3f4f6' }}>
            <h5 className="mb-0 font-semibold" style={{ fontSize: 15, color: '#212529' }}>
              运行业务
            </h5>
            <span className="font-semibold uppercase tracking-widest" style={{ fontSize: 11, color: '#878a99' }}>
              {serverAssets.length} 项
            </span>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full" style={{ fontSize: 14 }}>
              <thead>
                <tr style={{ background: '#f8f9fa' }}>
                  <th className="text-left py-3 px-6 font-semibold" style={{ color: '#495057', fontSize: 13 }}>项目</th>
                  <th className="text-left py-3 px-6 font-semibold" style={{ color: '#495057', fontSize: 13 }}>技术栈</th>
                  <th className="text-left py-3 px-6 font-semibold" style={{ color: '#495057', fontSize: 13 }}>路径</th>
                  <th className="text-left py-3 px-6 font-semibold" style={{ color: '#495057', fontSize: 13 }}>端口</th>
                </tr>
              </thead>
              <tbody>
                {serverAssets.map((a) => (
                  <tr
                    key={a.id}
                    className="transition-colors"
                    style={{ borderTop: '1px solid #f3f4f6' }}
                    onMouseEnter={(e) => (e.currentTarget.style.background = '#f8f9fa')}
                    onMouseLeave={(e) => (e.currentTarget.style.background = '')}
                  >
                    <td className="py-3 px-6">
                      <div className="font-medium" style={{ color: '#212529' }}>{a.name}</div>
                      {a.description && (
                        <div style={{ fontSize: 11, color: '#878a99', marginTop: 2 }}>{a.description}</div>
                      )}
                    </td>
                    <td className="py-3 px-6">
                      <div className="flex flex-wrap gap-1">
                        {a.tech_stack
                          ? a.tech_stack.split(/[,，、/]/).map((t, i) => (
                              <span
                                key={i}
                                className="px-2 py-0.5 rounded"
                                style={{ fontSize: 11, background: 'rgba(44,160,122,0.1)', color: '#2ca07a', border: '1px solid rgba(44,160,122,0.2)' }}
                              >
                                {t.trim()}
                              </span>
                            ))
                          : <span style={{ color: '#878a99' }}>-</span>}
                      </div>
                    </td>
                    <td className="py-3 px-6" style={{ fontFamily: 'monospace', fontSize: 12, color: '#878a99' }}>{a.path || '-'}</td>
                    <td className="py-3 px-6" style={{ fontFamily: 'monospace', fontSize: 13, color: '#495057' }}>{a.port || '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Docker Containers ── */}
      {containers.length > 0 && (
        <div
          className="bg-white overflow-hidden mb-5"
          style={{ borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.05)' }}
        >
          <div className="flex items-center justify-between px-6 py-4" style={{ borderBottom: '1px solid #f3f4f6' }}>
            <h5 className="mb-0 font-semibold" style={{ fontSize: 15, color: '#212529' }}>
              运行中容器 (Docker)
            </h5>
            <span className="font-semibold uppercase tracking-widest" style={{ fontSize: 11, color: '#878a99' }}>
              {runningContainers} / {containers.length} 运行中
            </span>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full" style={{ fontSize: 14 }}>
              <thead>
                <tr style={{ background: '#f8f9fa' }}>
                  <th className="text-left py-3 px-6 font-semibold" style={{ color: '#495057', fontSize: 13 }}>容器名</th>
                  <th className="text-left py-3 px-6 font-semibold" style={{ color: '#495057', fontSize: 13 }}>状态</th>
                  <th className="text-right py-3 px-6 font-semibold" style={{ color: '#495057', fontSize: 13 }}>CPU %</th>
                  <th className="text-right py-3 px-6 font-semibold" style={{ color: '#495057', fontSize: 13 }}>内存</th>
                  <th className="hidden sm:table-cell text-left py-3 px-6 font-semibold" style={{ color: '#495057', fontSize: 13 }}>镜像</th>
                </tr>
              </thead>
              <tbody>
                {containers.map((c) => {
                  const running = c.state === 'running'
                  return (
                    <tr
                      key={c.container_id}
                      className="transition-colors"
                      style={{ borderTop: '1px solid #f3f4f6' }}
                      onMouseEnter={(e) => (e.currentTarget.style.background = '#f8f9fa')}
                      onMouseLeave={(e) => (e.currentTarget.style.background = '')}
                    >
                      <td className="py-3 px-6 font-medium" style={{ color: '#212529' }}>{c.name}</td>
                      <td className="py-3 px-6">
                        <span className="inline-flex items-center gap-1.5" style={{ color: running ? '#0ab39c' : '#f06548' }}>
                          <span
                            className="inline-block rounded-full"
                            style={{
                              width: 7,
                              height: 7,
                              background: running ? '#0ab39c' : '#f06548',
                              boxShadow: running
                                ? '0 0 6px rgba(10,179,156,0.65)'
                                : '0 0 6px rgba(240,101,72,0.65)',
                            }}
                          />
                          <span className="text-sm font-medium">{c.state}</span>
                        </span>
                      </td>
                      <td className="py-3 px-6 text-right" style={{ fontFamily: 'monospace', fontSize: 13, color: '#495057' }}>
                        {(c.cpu_percent ?? 0).toFixed(1)}%
                      </td>
                      <td className="py-3 px-6 text-right" style={{ fontFamily: 'monospace', fontSize: 13, color: '#495057' }}>
                        {formatBytes(c.memory_usage)}
                      </td>
                      <td
                        className="hidden sm:table-cell py-3 px-6 truncate max-w-[220px]"
                        style={{ fontFamily: 'monospace', fontSize: 12, color: '#878a99' }}
                      >
                        {c.image}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── 历史趋势 ── */}
      <div
        className="bg-white"
        style={{ borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.05)', padding: '20px 24px' }}
      >
        <div className="flex items-center justify-between mb-5">
          <h5 className="mb-0 font-semibold" style={{ fontSize: 15, color: '#212529' }}>
            历史趋势
          </h5>

          <div className="flex items-center gap-2">
            {/* btn-group style time range selector */}
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
            queries={[{ query: `mantisops_cpu_usage_percent{host_id="${id}"}`, label: 'CPU', color: '#2ca07a' }]}
            {...timeWindow}
            unit="%"
          />
          <HistoryChart
            title="系统负载"
            queries={[
              { query: `mantisops_cpu_load1{host_id="${id}"}`, label: '1分钟负载', color: '#2ca07a' },
              { query: `mantisops_cpu_load5{host_id="${id}"}`, label: '5分钟负载', color: '#0ab39c' },
            ]}
            {...timeWindow}
            chartType="line"
          />
          <HistoryChart
            title="内存使用率"
            queries={[{ query: `mantisops_memory_usage_percent{host_id="${id}"}`, label: '内存', color: '#f7b84b' }]}
            {...timeWindow}
            unit="%"
          />
          <HistoryChart
            title="磁盘使用率"
            queries={[{ query: `mantisops_disk_usage_percent{host_id="${id}"}`, label: '磁盘', color: '#f06548' }]}
            {...timeWindow}
            unit="%"
          />
          <HistoryChart
            title="网络流量（合计）"
            queries={[
              { query: `sum(mantisops_network_rx_bytes_per_sec{host_id="${id}"})`, label: '入站', color: '#0ab39c' },
              { query: `sum(mantisops_network_tx_bytes_per_sec{host_id="${id}"})`, label: '出站', color: '#f7b84b' },
            ]}
            {...timeWindow}
            chartType="line"
            formatValue={(v) => formatBytes(v)}
          />
          <HistoryChart
            title="网络流量（分网卡）"
            queries={[
              { query: `mantisops_network_rx_bytes_per_sec{host_id="${id}"}`, label: '入站', color: '#0ab39c' },
              { query: `mantisops_network_tx_bytes_per_sec{host_id="${id}"}`, label: '出站', color: '#f7b84b' },
            ]}
            {...timeWindow}
            chartType="line"
            formatValue={(v) => formatBytes(v)}
          />
        </div>

        {server.gpu_model && (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mt-4">
            <HistoryChart
              title="GPU 使用率"
              queries={[{ query: `mantisops_gpu_usage_percent{host_id="${id}"}`, label: 'GPU', color: '#a855f7' }]}
              {...timeWindow}
              unit="%"
            />
            <HistoryChart
              title="GPU 显存"
              queries={[{ query: `mantisops_gpu_memory_used_bytes{host_id="${id}"}`, label: '显存', color: '#3b82f6' }]}
              {...timeWindow}
              formatValue={(v) => formatBytes(v)}
            />
            <HistoryChart
              title="GPU 温度"
              queries={[{ query: `mantisops_gpu_temperature{host_id="${id}"}`, label: '温度', color: '#f97316' }]}
              {...timeWindow}
              unit="°C"
              chartType="line"
            />
          </div>
        )}
      </div>
    </div>
  )
}

// ── MetricCard sub-component ──────────────────────────────────────────────────
interface MetricCardProps {
  label: string
  value: string
  valueColor?: string
  subtitle?: string
  highlightBorder?: 'warning' | 'danger' | 'primary'
}

function MetricCard({ label, value, valueColor, subtitle, highlightBorder }: MetricCardProps) {
  const borderColorMap: Record<string, string> = {
    warning: 'rgba(247,184,75,0.5)',
    danger: 'rgba(240,101,72,0.5)',
    primary: 'rgba(44,160,122,0.3)',
  }
  const bgColorMap: Record<string, string> = {
    warning: 'rgba(247,184,75,0.05)',
    danger: 'rgba(240,101,72,0.05)',
    primary: 'rgba(44,160,122,0.05)',
  }

  return (
    <div
      className="text-center"
      style={{
        background: highlightBorder ? bgColorMap[highlightBorder] : '#f8f9fa',
        borderRadius: 8,
        padding: '15px 12px',
        border: `1px solid ${highlightBorder ? borderColorMap[highlightBorder] : '#eeeeee'}`,
      }}
    >
      <div
        className="font-semibold uppercase"
        style={{ fontSize: 12, color: '#878a99', letterSpacing: '0.05em' }}
      >
        {label}
      </div>
      <div
        className="font-bold"
        style={{
          fontSize: 24,
          color: valueColor ?? '#212529',
          fontFamily: 'var(--font-heading, inherit)',
          margin: '5px 0',
          lineHeight: 1.2,
        }}
      >
        {value}
      </div>
      {subtitle && (
        <div style={{ fontSize: 11, color: '#878a99', marginTop: 2 }}>
          {subtitle}
        </div>
      )}
    </div>
  )
}
