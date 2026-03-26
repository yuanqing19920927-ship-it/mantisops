import { useEffect, useState, useCallback, useMemo } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useServerStore } from '../../stores/serverStore'
import { StatusBadge } from '../../components/StatusBadge'
import { ProgressBar } from '../../components/ProgressBar'
import { getProbeStatus, getAlertStats, getAlertEvents, getDatabases, getBilling } from '../../api/client'
import type { ProbeResult, AlertStats, AlertEvent } from '../../types'
import type { BillingItem, RDSInfo } from '../../api/client'
import { formatBytesPS } from '../../utils/format'

export default function Dashboard() {
  const { servers, metrics, groups, loading, fetchDashboard } = useServerStore()
  const [probes, setProbes] = useState<ProbeResult[]>([])
  const [alertStats, setAlertStats] = useState<AlertStats | null>(null)
  const [alertEvents, setAlertEvents] = useState<AlertEvent[]>([])
  const [databases, setDatabases] = useState<RDSInfo[]>([])
  const [billing, setBilling] = useState<BillingItem[]>([])
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set())
  const location = useLocation()

  const refreshAll = useCallback(() => {
    fetchDashboard()
    Promise.all([
      getProbeStatus().then(setProbes).catch(() => {}),
      getAlertStats().then(setAlertStats).catch(() => {}),
      getAlertEvents({ status: 'firing', silenced: false, limit: 5 }).then(setAlertEvents).catch(() => {}),
      getDatabases().then(setDatabases).catch(() => {}),
      getBilling().then(setBilling).catch(() => {}),
    ])
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

  const topByCpu = [...serverMetrics].sort((a, b) => b.cpuPercent - a.cpuPercent).slice(0, 5)

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

  const statCards = [
    {
      label: '服务器',
      value: `${onlineCount}/${servers.length}`,
      sub: '在线',
      icon: 'dns',
      borderColor: 'border-primary',
      valueColor: onlineCount === servers.length ? 'text-tertiary' : 'text-warning',
    },
    {
      label: '容器',
      value: `${totalContainers}`,
      sub: '运行中',
      icon: 'cyclone',
      borderColor: 'border-tertiary',
      valueColor: 'text-tertiary',
    },
    {
      label: '端口探测',
      value: `${probesUp}/${probes.length}`,
      sub: '正常',
      icon: 'sensors',
      borderColor: probesUp === probes.length ? 'border-tertiary' : 'border-error',
      valueColor: probesUp === probes.length ? 'text-tertiary' : 'text-error',
    },
    {
      label: '平均 CPU',
      value: `${avgCpu}%`,
      sub: '使用率',
      icon: 'speed',
      borderColor: 'border-primary',
      valueColor: 'text-primary',
    },
    {
      label: '告警中',
      value: `${alertStats?.firing_unsilenced ?? 0}`,
      sub: '未处理',
      icon: 'error',
      borderColor: (alertStats?.firing_unsilenced ?? 0) > 0 ? 'border-error' : 'border-tertiary',
      valueColor: (alertStats?.firing_unsilenced ?? 0) > 0 ? 'text-error' : 'text-tertiary',
    },
    {
      label: '即将到期',
      value: `${billing.filter(b => b.days_left >= 0 && b.days_left <= 30).length}`,
      sub: '30天内',
      icon: 'event_upcoming',
      borderColor: billing.some(b => b.days_left >= 0 && b.days_left <= 30) ? 'border-warning' : 'border-tertiary',
      valueColor: billing.some(b => b.days_left >= 0 && b.days_left <= 30) ? 'text-warning' : 'text-tertiary',
    },
  ]

  return (
    <div className="space-y-8">
      {/* Stat Cards Row */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        {statCards.map((stat) => (
          <div
            key={stat.label}
            className={`glass-card p-4 md:p-6 rounded-xl border-l-4 ${stat.borderColor} relative overflow-hidden`}
          >
            <span className="material-symbols-outlined absolute right-3 top-3 text-5xl md:text-7xl text-white/30 select-none pointer-events-none">
              {stat.icon}
            </span>
            <div className="text-xs font-body text-on-surface-variant uppercase tracking-wider mb-2">
              {stat.label}
            </div>
            <div className={`text-2xl md:text-3xl font-headline font-bold ${stat.valueColor} mb-1`}>
              {stat.value}
            </div>
            <div className="text-xs font-body text-secondary">
              {stat.sub}
            </div>
          </div>
        ))}
      </div>

      {/* Summary Row */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* Alert Summary */}
        <div className="glass-card rounded-xl p-5">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-headline font-bold text-on-surface flex items-center gap-2">
              <span className="material-symbols-outlined text-error text-lg">notification_important</span>
              未处理告警
            </h3>
            {(alertStats?.firing_unsilenced ?? 0) > 0 && (
              <span className="text-[10px] px-2 py-0.5 rounded-full bg-error/20 text-error font-bold">
                {alertStats?.firing_unsilenced}
              </span>
            )}
          </div>
          {alertEvents.length > 0 ? (
            <div className="space-y-2">
              {alertEvents.map(e => (
                <div key={e.id} className="flex items-center gap-2 text-xs">
                  <span className={`w-2 h-2 rounded-full flex-shrink-0 ${e.level === 'critical' ? 'bg-error' : 'bg-warning'}`} />
                  <span className="text-on-surface truncate flex-1">{e.rule_name}</span>
                  <span className="text-on-surface-variant flex-shrink-0">{e.target_label}</span>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-sm text-tertiary text-center py-4">一切正常</div>
          )}
          <Link to="/alerts" className="block text-xs text-primary mt-3 text-right hover:underline">查看全部 &rarr;</Link>
        </div>

        {/* RDS Overview */}
        <div className="glass-card rounded-xl p-5">
          <h3 className="text-sm font-headline font-bold text-on-surface flex items-center gap-2 mb-4">
            <span className="material-symbols-outlined text-tertiary text-lg">database</span>
            数据库状态
          </h3>
          {databases.length > 0 ? (
            <div className="space-y-2">
              {databases.map(db => (
                <div key={db.host_id} className="flex items-center gap-2">
                  <span className="text-xs text-on-surface truncate w-20">{db.name || db.host_id}</span>
                  <div className="flex-1 flex gap-1">
                    <div className="flex-1 h-1.5 bg-surface-container-high rounded-full overflow-hidden" title="CPU">
                      <div className="h-full bg-primary rounded-full" style={{ width: `${db.metrics?.cpu_usage ?? 0}%` }} />
                    </div>
                    <div className="flex-1 h-1.5 bg-surface-container-high rounded-full overflow-hidden" title="内存">
                      <div className="h-full bg-tertiary rounded-full" style={{ width: `${db.metrics?.memory_usage ?? 0}%` }} />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-sm text-tertiary text-center py-4">暂无数据库</div>
          )}
          <Link to="/databases" className="block text-xs text-primary mt-3 text-right hover:underline">查看详情 &rarr;</Link>
        </div>

        {/* Expiry Warning */}
        <div className="glass-card rounded-xl p-5">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-headline font-bold text-on-surface flex items-center gap-2">
              <span className="material-symbols-outlined text-warning text-lg">event_upcoming</span>
              30天内到期
            </h3>
            {billing.filter(b => b.days_left >= 0 && b.days_left <= 30).length > 0 && (
              <span className="text-[10px] px-2 py-0.5 rounded-full bg-warning/20 text-warning font-bold">
                {billing.filter(b => b.days_left >= 0 && b.days_left <= 30).length}
              </span>
            )}
          </div>
          {(() => {
            const urgent = billing.filter(b => b.days_left >= 0 && b.days_left <= 30)
            return urgent.length > 0 ? (
              <div className="space-y-2">
                {urgent.slice(0, 5).map(b => (
                  <div key={`${b.type}-${b.id}`} className="flex items-center gap-2 text-xs">
                    <span className={`px-1.5 py-0.5 rounded font-bold uppercase text-[10px] ${
                      b.type === 'ecs' ? 'bg-primary/20 text-primary' :
                      b.type === 'rds' ? 'bg-tertiary/20 text-tertiary' :
                      'bg-warning/20 text-warning'
                    }`}>{b.type}</span>
                    <span className="text-on-surface truncate flex-1">{b.name}</span>
                    <span className="text-error font-bold flex-shrink-0">{b.days_left}天</span>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-sm text-tertiary text-center py-4">全部安全</div>
            )
          })()}
          <Link to="/billing" className="block text-xs text-primary mt-3 text-right hover:underline">查看全部 &rarr;</Link>
        </div>
      </div>

      {/* Main Grid: Server List + Right Panel */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-8">
        {/* Left Column: Server Status List (Grouped) */}
        <div className="lg:col-span-7">
          <div className="glass-card rounded-xl p-6 lg:h-[calc(100vh-320px)] flex flex-col">
            <div className="flex items-center justify-between mb-5 flex-shrink-0">
              <h2 className="text-xl font-headline font-bold text-primary flex items-center gap-2">
                <span className="material-symbols-outlined text-primary">dns</span>
                服务器状态
              </h2>
              <Link to="/servers" className="text-xs font-body text-primary hover:text-on-surface transition-colors">
                查看全部 &rarr;
              </Link>
            </div>
            <div className="space-y-1 overflow-y-auto flex-1 min-h-0">
              {grouped.map(({ name, servers: groupSvrs }) => {
                const key = `dash-${name}`
                const isCollapsed = collapsedGroups.has(key)
                const onlineInGroup = groupSvrs.filter(s => s.status === 'online').length
                return (
                  <div key={key}>
                    {/* Group Header */}
                    <button
                      onClick={() => toggleGroup(key)}
                      className="flex items-center gap-2 w-full text-left px-3 py-2 rounded-lg hover:bg-surface-container-low transition-colors"
                    >
                      <span className={`material-symbols-outlined text-xs text-on-surface-variant transition-transform ${isCollapsed ? '' : 'rotate-90'}`}>
                        chevron_right
                      </span>
                      <span className="text-xs font-headline font-bold text-on-surface">{name}</span>
                      <span className="text-[10px] text-on-surface-variant">
                        ({onlineInGroup}/{groupSvrs.length})
                      </span>
                    </button>
                    {/* Group Servers */}
                    {!isCollapsed && (
                      <div className="space-y-1 ml-1">
                        {groupSvrs.map((s) => {
                          let ip = ''
                          try { ip = JSON.parse(s.ip_addresses)[0] } catch { /* ignore */ }
                          return (
                            <Link
                              key={s.host_id}
                              to={`/servers/${s.host_id}`}
                              className="flex items-center gap-4 p-3 rounded-lg bg-surface-container-low hover:bg-surface-container-high transition-colors group"
                            >
                              {/* Status Icon Box */}
                              <div className="flex-shrink-0 w-10 h-10 rounded-lg bg-surface-container flex items-center justify-center">
                                <StatusBadge status={s.status} />
                              </div>

                              {/* Info + Bars */}
                              <div className="flex-1 min-w-0">
                                <div className="flex items-center gap-2 mb-1.5">
                                  <span className="text-sm font-headline font-semibold text-on-surface truncate">
                                    {s.display_name || s.hostname}
                                  </span>
                                  {ip && (
                                    <span className="hidden sm:inline text-[11px] font-body text-on-surface-variant">
                                      {ip}
                                    </span>
                                  )}
                                </div>
                                <div className="hidden sm:grid grid-cols-3 gap-3">
                                  <ProgressBar percent={s.cpuPercent} label="CPU" size="sm" />
                                  <ProgressBar percent={s.memPercent} label="内存" size="sm" />
                                  <ProgressBar percent={s.diskPercent} label="磁盘" size="sm" />
                                </div>
                              </div>

                              {/* Network Speed */}
                              <div className="hidden sm:block flex-shrink-0 text-right text-[11px] font-body text-on-surface-variant whitespace-nowrap">
                                <div className="flex items-center justify-end gap-1">
                                  <span className="material-symbols-outlined text-xs text-tertiary">arrow_upward</span>
                                  {formatBytesPS(s.netTx)}
                                </div>
                                <div className="flex items-center justify-end gap-1 mt-0.5">
                                  <span className="material-symbols-outlined text-xs text-primary">arrow_downward</span>
                                  {formatBytesPS(s.netRx)}
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
                <div className="text-center py-12 text-on-surface-variant font-body">
                  暂无服务器数据，请部署 Agent 后等待上报
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Right Column: Port Monitoring + Resource Ranking */}
        <div className="lg:col-span-5 flex flex-col gap-8 lg:h-[calc(100vh-320px)]">
          {/* Port Monitoring */}
          <div className="glass-card rounded-xl p-6 flex-1 min-h-0 flex flex-col">
            <div className="flex items-center justify-between mb-5 flex-shrink-0">
              <h2 className="text-xl font-headline font-bold text-primary flex items-center gap-2">
                <span className="material-symbols-outlined text-primary">sensors</span>
                端口监控
              </h2>
              <Link to="/probes" className="text-xs font-body text-primary hover:text-on-surface transition-colors">
                管理 &rarr;
              </Link>
            </div>
            {probes.length > 0 ? (
              <div className="space-y-1 overflow-y-auto flex-1 min-h-0">
                {probes.map((p) => {
                  const isUp = p.status === 'up'
                  return (
                    <div
                      key={p.rule_id}
                      className="flex items-center justify-between py-2.5 px-3 rounded-lg hover:bg-surface-container-low transition-colors"
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <span
                          className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${
                            isUp ? 'bg-tertiary shadow-[0_0_8px_rgba(78,222,163,0.5)]' : 'bg-error shadow-[0_0_8px_rgba(255,180,171,0.5)]'
                          }`}
                        />
                        <span className="text-sm font-body text-on-surface truncate">
                          {p.name}
                        </span>
                        <span className="hidden sm:inline text-[11px] font-body text-on-surface-variant flex-shrink-0">
                          {p.host}:{p.port}
                        </span>
                      </div>
                      <span
                        className={`text-xs font-headline font-bold flex-shrink-0 ml-3 ${
                          isUp ? 'text-tertiary' : 'text-error'
                        }`}
                      >
                        {isUp ? `${(p.latency_ms ?? 0).toFixed(1)}ms` : '异常'}
                      </span>
                    </div>
                  )
                })}
              </div>
            ) : (
              <div className="text-sm py-8 text-center text-on-surface-variant font-body">
                暂无探测规则，
                <Link to="/probes" className="text-primary hover:underline">去添加</Link>
              </div>
            )}
          </div>

          {/* Resource Ranking */}
          <div className="glass-card rounded-xl p-6 flex-1 min-h-0 flex flex-col">
            <h2 className="text-xl font-headline font-bold text-primary flex items-center gap-2 mb-5 flex-shrink-0">
              <span className="material-symbols-outlined text-primary">leaderboard</span>
              资源使用排行
            </h2>

            {topByCpu.length > 0 ? (
              <div className="space-y-4 overflow-y-auto flex-1 min-h-0">
                {topByCpu.map((s, idx) => (
                  <Link
                    key={s.host_id}
                    to={`/servers/${s.host_id}`}
                    className="block p-3 rounded-lg bg-surface-container-low hover:bg-surface-container transition-colors"
                  >
                    <div className="flex items-center gap-3 mb-3">
                      <span
                        className={`w-6 h-6 rounded-md flex items-center justify-center text-xs font-headline font-bold ${
                          idx === 0
                            ? 'bg-primary/20 text-primary'
                            : idx === 1
                              ? 'bg-tertiary/20 text-tertiary'
                              : 'bg-surface-container-high text-on-surface-variant'
                        }`}
                      >
                        {idx + 1}
                      </span>
                      <span className="text-sm font-headline font-semibold text-on-surface truncate">
                        {s.display_name || s.hostname}
                      </span>
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                      <ProgressBar percent={s.cpuPercent} label="CPU" />
                      <ProgressBar percent={s.memPercent} label="内存" />
                    </div>
                  </Link>
                ))}
              </div>
            ) : (
              <div className="text-sm py-8 text-center text-on-surface-variant font-body">
                暂无数据
              </div>
            )}
          </div>
        </div>
      </div>

    </div>
  )
}
