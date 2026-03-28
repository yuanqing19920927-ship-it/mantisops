import { useEffect, useState, useCallback, useMemo } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useServerStore } from '../../stores/serverStore'
import { StatusBadge } from '../../components/StatusBadge'
import { ProgressBar } from '../../components/ProgressBar'
import { getProbeStatus } from '../../api/client'
import type { ProbeResult } from '../../types'
import { formatBytesPS } from '../../utils/format'
import { latestReport } from '../../api/ai'
import type { AIReport } from '../../api/ai'

export default function Dashboard() {
  const { servers, metrics, groups, loading, fetchDashboard } = useServerStore()
  const [probes, setProbes] = useState<ProbeResult[]>([])
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set())
  const [aiReport, setAiReport] = useState<AIReport | null>(null)
  const location = useLocation()

  const refreshAll = useCallback(() => {
    fetchDashboard()
    getProbeStatus().then(setProbes).catch(() => {})
    latestReport().then(setAiReport).catch(() => setAiReport(null))
  }, [fetchDashboard])

  // 每次进入页面（路由切换）或页面重新可见时刷新
  useEffect(() => {
    refreshAll()
    const timer = setInterval(refreshAll, 15000)
    const onVisible = () => { if (document.visibilityState === 'visible') refreshAll() }
    document.addEventListener('visibilitychange', onVisible)
    return () => { clearInterval(timer); document.removeEventListener('visibilitychange', onVisible) }
  }, [refreshAll, location.key])

  const onlineCount = servers.filter((s) => s.status === 'online').length
  const totalContainers = Object.values(metrics).reduce(
    (sum, m) => sum + (m.containers?.filter((c) => c.state === 'running').length ?? 0), 0
  )
  const probesUp = probes.filter((p) => p.status === 'up').length

  const serverMetrics = servers.map((s) => {
    const m = metrics[s.host_id]
    return {
      ...s,
      cpuPercent: m?.cpu?.usage_percent ?? 0,
      memPercent: m?.memory?.usage_percent ?? 0,
      diskPercent: Math.max(0, ...(m?.disks?.map(d => d.usage_percent) ?? [0])),
      netRx: m?.networks?.reduce((sum, n) => sum + (n.rx_bytes_per_sec ?? 0), 0) ?? 0,
      netTx: m?.networks?.reduce((sum, n) => sum + (n.tx_bytes_per_sec ?? 0), 0) ?? 0,
    }
  })

  const avgCpu = serverMetrics.length > 0
    ? (serverMetrics.reduce((s, m) => s + m.cpuPercent, 0) / serverMetrics.length).toFixed(1)
    : '0'

  // Top 3 by CPU
  const topByCpu = [...serverMetrics].sort((a, b) => b.cpuPercent - a.cpuPercent).slice(0, 3)

  // Group servers
  const grouped = useMemo(() => {
    const map = new Map<number | 'ungrouped', typeof serverMetrics>()
    for (const g of (groups || [])) map.set(g.id, [])
    map.set('ungrouped', [])
    for (const s of serverMetrics) {
      const gid = s.group_id ?? 'ungrouped'
      const arr = map.get(gid) || map.get('ungrouped')!
      arr.push(s)
    }
    const result: { name: string; servers: typeof serverMetrics }[] = []
    for (const g of (groups || [])) {
      const svrs = map.get(g.id) || []
      if (svrs.length > 0) result.push({ name: g.name, servers: svrs })
    }
    const ug = map.get('ungrouped') || []
    if (ug.length > 0) result.push({ name: '未分组', servers: ug })
    return result
  }, [serverMetrics, groups])

  const toggleGroup = useCallback((key: string) => {
    setCollapsedGroups(prev => {
      const next = new Set(prev)
      next.has(key) ? next.delete(key) : next.add(key)
      return next
    })
  }, [])

  if (loading && servers.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-on-surface-variant font-body">
        <span className="material-symbols-outlined animate-spin mr-3 text-primary">progress_activity</span>
        加载中...
      </div>
    )
  }

  // Derived badge values for stat cards
  const serverOnlineRate = servers.length > 0
    ? ((onlineCount / servers.length) * 100).toFixed(1)
    : null
  const probeOkRate = probes.length > 0
    ? ((probesUp / probes.length) * 100).toFixed(0)
    : null
  const cpuNum = parseFloat(avgCpu)

  return (
    <div className="space-y-6">
      {/* AI Analysis Summary — full width */}
      <div className="glass-card p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3 min-w-0">
            <div className="w-9 h-9 rounded-full bg-[var(--color-primary)]/10 flex items-center justify-center flex-shrink-0">
              <span className="material-symbols-outlined text-[var(--color-primary)] text-lg">analytics</span>
            </div>
            {aiReport ? (
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 flex-wrap">
                  <h4 className="text-sm font-semibold text-on-surface">{aiReport.title}</h4>
                  <span className="text-[11px] text-on-surface-variant/60">
                    {new Date(aiReport.created_at).toLocaleDateString('zh-CN')}
                  </span>
                </div>
                <p className="text-xs text-on-surface-variant leading-relaxed mt-1 line-clamp-1">{aiReport.summary}</p>
              </div>
            ) : (
              <div>
                <h4 className="text-sm font-semibold text-on-surface">AI 分析</h4>
                <p className="text-xs text-on-surface-variant mt-0.5">暂无报告，在设置中配置 AI 提供商后即可生成</p>
              </div>
            )}
          </div>
          <Link
            to={aiReport ? `/ai-reports/${aiReport.id}` : '/ai-reports'}
            className="flex-shrink-0 inline-flex items-center gap-1 text-xs text-primary hover:text-primary-container transition-colors ml-4"
          >
            {aiReport ? '查看报告' : '全部报告'}
            <span className="material-symbols-outlined text-xs">arrow_forward</span>
          </Link>
        </div>
      </div>

      {/* Top Stats Row — 4 cards */}
      <div className="grid grid-cols-2 xl:grid-cols-4 gap-4">

        {/* Card 1: Servers Online */}
        <div className="glass-card p-5">
          <div className="flex items-center gap-4">
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-on-surface-variant mb-2">服务器在线数</p>
              <div className="flex items-center gap-3 flex-wrap">
                <span className="text-[22px] font-semibold text-on-surface leading-none">
                  {onlineCount} / {servers.length}
                </span>
                {serverOnlineRate !== null && (
                  <span
                    className={`inline-flex items-center gap-1 text-xs font-semibold px-2 py-0.5 rounded-full ${
                      onlineCount === servers.length
                        ? 'bg-tertiary/10 text-tertiary'
                        : 'bg-error/10 text-error'
                    }`}
                  >
                    {serverOnlineRate}%
                  </span>
                )}
              </div>
            </div>
            <div className="stat-icon flex-shrink-0">
              <span className="material-symbols-outlined">dns</span>
            </div>
          </div>
        </div>

        {/* Card 2: Containers */}
        <div className="glass-card p-5">
          <div className="flex items-center gap-4">
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-on-surface-variant mb-2">运行中容器</p>
              <span className="text-[22px] font-semibold text-on-surface leading-none">
                {totalContainers}
              </span>
            </div>
            <div
              className="stat-icon flex-shrink-0"
              style={{ color: '#61bcf7', backgroundColor: 'rgba(97,188,247,0.1)' }}
            >
              <span className="material-symbols-outlined">box</span>
            </div>
          </div>
        </div>

        {/* Card 3: Probes OK */}
        <div className="glass-card p-5">
          <div className="flex items-center gap-4">
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-on-surface-variant mb-2">端口探测正常</p>
              <div className="flex items-center gap-3 flex-wrap">
                <span className="text-[22px] font-semibold text-on-surface leading-none">
                  {probesUp} / {probes.length}
                </span>
                {probeOkRate !== null && (
                  <span
                    className={`text-xs font-semibold px-2 py-0.5 rounded-full ${
                      probesUp === probes.length
                        ? 'bg-tertiary/10 text-tertiary'
                        : 'bg-error/10 text-error'
                    }`}
                  >
                    {probeOkRate}%
                  </span>
                )}
              </div>
            </div>
            <div className="stat-icon flex-shrink-0">
              <span className="material-symbols-outlined">sensors</span>
            </div>
          </div>
        </div>

        {/* Card 4: Avg CPU */}
        <div className="glass-card p-5">
          <div className="flex items-center gap-4">
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-on-surface-variant mb-2">平均 CPU 使用率</p>
              <div className="flex items-center gap-3 flex-wrap">
                <span className="text-[22px] font-semibold text-on-surface leading-none">
                  {avgCpu}%
                </span>
                {serverMetrics.length > 0 && (
                  <span
                    className={`text-xs font-semibold px-2 py-0.5 rounded-full ${
                      cpuNum >= 80
                        ? 'bg-error/10 text-error'
                        : cpuNum >= 60
                          ? 'bg-warning/10 text-warning'
                          : 'bg-tertiary/10 text-tertiary'
                    }`}
                  >
                    {cpuNum >= 80 ? '高负载' : cpuNum >= 60 ? '中等' : '正常'}
                  </span>
                )}
              </div>
            </div>
            <div
              className="stat-icon flex-shrink-0"
              style={{ color: '#f7b84b', backgroundColor: 'rgba(247,184,75,0.1)' }}
            >
              <span className="material-symbols-outlined">memory</span>
            </div>
          </div>
        </div>

      </div>

      {/* Main Grid: Server List (7/12) + Right Panel (5/12) */}
      <div className="grid grid-cols-1 xl:grid-cols-12 gap-6">

        {/* Left Column: Server Status List */}
        <div className="xl:col-span-7">
          <div className="glass-card xl:h-[calc(100vh-280px)] flex flex-col">
            <div className="flex items-center justify-between px-5 pt-5 pb-4 flex-shrink-0">
              <h4 className="text-base font-semibold text-on-surface">服务器状态列表</h4>
              <Link
                to="/servers"
                className="text-xs text-primary hover:text-primary-container transition-colors"
              >
                查看全部 &rarr;
              </Link>
            </div>

            <div className="overflow-y-auto flex-1 min-h-0 px-1 pb-3">
              {grouped.map(({ name, servers: groupSvrs }) => {
                const key = `dash-${name}`
                const isCollapsed = collapsedGroups.has(key)
                const onlineInGroup = groupSvrs.filter(s => s.status === 'online').length
                return (
                  <div key={key}>
                    {/* Group Header */}
                    <button
                      onClick={() => toggleGroup(key)}
                      className="flex items-center gap-2 w-full text-left px-4 py-2 hover:bg-surface-container-low transition-colors"
                      aria-expanded={!isCollapsed}
                    >
                      <span
                        className={`material-symbols-outlined text-sm text-on-surface-variant transition-transform duration-200 ${
                          isCollapsed ? '' : 'rotate-90'
                        }`}
                      >
                        chevron_right
                      </span>
                      <span className="text-xs font-semibold text-on-surface">{name}</span>
                      <span className="text-[11px] text-on-surface-variant">
                        ({onlineInGroup}/{groupSvrs.length})
                      </span>
                    </button>

                    {/* Server Rows */}
                    {!isCollapsed && (
                      <div>
                        {groupSvrs.map((s) => {
                          let ip = ''
                          try { ip = JSON.parse(s.ip_addresses)[0] } catch { /* ignore */ }
                          const isOnline = s.status === 'online'
                          return (
                            <Link
                              key={s.host_id}
                              to={`/servers/${s.host_id}`}
                              className="flex items-center gap-3 px-4 py-3 hover:bg-surface-container-low transition-colors border-t border-outline-variant/50"
                              aria-label={`查看服务器 ${s.display_name || s.hostname}`}
                            >
                              {/* Server Icon */}
                              <div
                                className="flex-shrink-0 w-10 h-10 rounded-full flex items-center justify-center text-xl"
                                style={
                                  isOnline
                                    ? { color: '#2ca07a', backgroundColor: 'rgba(44,160,122,0.1)' }
                                    : { color: '#f06548', backgroundColor: 'rgba(240,101,72,0.1)' }
                                }
                              >
                                <span className="material-symbols-outlined" style={{ fontSize: '20px' }}>
                                  computer
                                </span>
                              </div>

                              {/* Hostname + IP */}
                              <div className="w-36 flex-shrink-0 min-w-0">
                                <div className="flex items-center gap-1.5 flex-wrap">
                                  <span className="text-sm font-semibold text-on-surface truncate">
                                    {s.display_name || s.hostname}
                                  </span>
                                  <StatusBadge status={s.status} label={isOnline ? '在线' : '离线'} />
                                </div>
                                {ip && (
                                  <p className="text-[11px] text-on-surface-variant mt-0.5 truncate">{ip}</p>
                                )}
                              </div>

                              {/* CPU bar */}
                              <div className="hidden sm:block w-28 flex-shrink-0">
                                <ProgressBar percent={s.cpuPercent} label="CPU" size="sm" />
                              </div>

                              {/* MEM bar */}
                              <div className="hidden sm:block w-28 flex-shrink-0">
                                <ProgressBar percent={s.memPercent} label="MEM" size="sm" />
                              </div>

                              {/* Network speeds */}
                              <div className="hidden md:block flex-shrink-0 text-right text-[11px] text-on-surface-variant whitespace-nowrap ml-auto">
                                <div className="flex items-center justify-end gap-1">
                                  <span className="material-symbols-outlined text-[12px] text-error">arrow_downward</span>
                                  {isOnline ? formatBytesPS(s.netRx) : '-'}
                                </div>
                                <div className="flex items-center justify-end gap-1 mt-0.5">
                                  <span className="material-symbols-outlined text-[12px] text-tertiary">arrow_upward</span>
                                  {isOnline ? formatBytesPS(s.netTx) : '-'}
                                </div>
                              </div>
                            </Link>
                          )
                        })}
                      </div>
                    )}
                  </div>
                )
              })}

              {servers.length === 0 && (
                <div className="text-center py-12 text-on-surface-variant text-sm">
                  暂无服务器数据，请部署 Agent 后等待上报
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Right Column */}
        <div className="xl:col-span-5 flex flex-col gap-6 xl:h-[calc(100vh-280px)]">

          {/* Port Probes Summary */}
          <div className="glass-card flex-1 min-h-0 flex flex-col">
            <div className="flex items-center justify-between px-5 pt-5 pb-4 flex-shrink-0">
              <h4 className="text-base font-semibold text-on-surface">端口监控摘要</h4>
              <Link
                to="/probes"
                className="text-xs px-3 py-1 rounded-md bg-surface-container-high text-on-surface-variant hover:bg-surface-container-highest transition-colors font-medium"
              >
                View All
              </Link>
            </div>

            {probes.length > 0 ? (
              <div className="space-y-2 overflow-y-auto flex-1 min-h-0 px-5 pb-5">
                {probes.map((p) => {
                  const isUp = p.status === 'up'
                  return (
                    <div
                      key={p.rule_id}
                      className={`flex items-center justify-between px-3 py-2.5 rounded-lg border transition-colors ${
                        isUp
                          ? 'bg-surface-container-low border-outline-variant/50'
                          : 'bg-error/5 border-error/20'
                      }`}
                    >
                      <div className="flex items-center gap-2.5 min-w-0">
                        {/* Glow dot */}
                        <span
                          className={`w-2 h-2 rounded-full flex-shrink-0 ${
                            isUp
                              ? 'bg-tertiary shadow-[0_0_8px_rgba(10,179,156,0.6)]'
                              : 'bg-error shadow-[0_0_8px_rgba(240,101,72,0.6)]'
                          }`}
                          aria-hidden="true"
                        />
                        <span
                          className={`text-sm font-medium truncate ${
                            isUp ? 'text-on-surface' : 'text-error'
                          }`}
                        >
                          {p.name}
                        </span>
                      </div>
                      <div className="flex-shrink-0 text-right ml-3">
                        <span
                          className={`inline-block text-[11px] font-medium px-2 py-0.5 rounded ${
                            isUp
                              ? 'bg-surface-container-high text-on-surface-variant'
                              : 'bg-error text-white'
                          }`}
                        >
                          {p.host}:{p.port}
                        </span>
                        <div
                          className={`text-[11px] mt-1 ${
                            isUp ? 'text-on-surface-variant' : 'text-error'
                          }`}
                        >
                          {isUp ? `${(p.latency_ms ?? 0).toFixed(1)}ms` : '连接失败'}
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            ) : (
              <div className="text-sm py-8 text-center text-on-surface-variant px-5 pb-5">
                暂无探测规则，
                <Link to="/probes" className="text-primary hover:underline">去添加</Link>
              </div>
            )}
          </div>

          {/* Top 3 CPU Resource Ranking */}
          <div className="glass-card flex-1 min-h-0 flex flex-col">
            <div className="px-5 pt-5 pb-4 flex-shrink-0">
              <h4 className="text-base font-semibold text-on-surface">资源使用排行 (Top 3 CPU)</h4>
            </div>

            {topByCpu.length > 0 ? (
              <div className="space-y-5 overflow-y-auto flex-1 min-h-0 px-5 pb-5">
                {topByCpu.map((s) => {
                  const cpu = s.cpuPercent
                  const cpuColor =
                    cpu >= 80 ? 'text-error' : cpu >= 60 ? 'text-warning' : 'text-tertiary'
                  const barColor =
                    cpu >= 80 ? 'bg-error' : cpu >= 60 ? 'bg-warning' : 'bg-tertiary'
                  return (
                    <Link
                      key={s.host_id}
                      to={`/servers/${s.host_id}`}
                      className="block group"
                      aria-label={`查看服务器 ${s.display_name || s.hostname}`}
                    >
                      <div className="flex items-center justify-between mb-2">
                        <h6 className="text-sm font-semibold text-on-surface group-hover:text-primary transition-colors truncate">
                          {s.display_name || s.hostname}
                        </h6>
                        <span className={`text-sm font-bold flex-shrink-0 ml-3 ${cpuColor}`}>
                          {cpu.toFixed(1)}%
                        </span>
                      </div>
                      <div className="w-full h-1.5 bg-surface-container-high rounded-full overflow-hidden mb-2">
                        <div
                          className={`h-full ${barColor} rounded-full transition-all duration-500`}
                          style={{ width: `${Math.min(100, cpu)}%` }}
                        />
                      </div>
                      <div className="text-[11px] text-on-surface-variant">
                        MEM: {s.memPercent.toFixed(1)}%
                      </div>
                    </Link>
                  )
                })}
              </div>
            ) : (
              <div className="text-sm py-8 text-center text-on-surface-variant px-5 pb-5">
                暂无数据
              </div>
            )}
          </div>

        </div>
      </div>

    </div>
  )
}
