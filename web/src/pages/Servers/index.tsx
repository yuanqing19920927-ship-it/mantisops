import { useEffect, useState, useMemo, useCallback } from 'react'
import { useServerStore } from '../../stores/serverStore'
import { ServerCard } from '../../components/ServerCard'
import { StatusBadge } from '../../components/StatusBadge'
import { Link } from 'react-router-dom'
import { formatBytesPS } from '../../utils/format'
import { createGroup, deleteGroup, setServerGroup } from '../../api/client'
import type { ServerGroup } from '../../types'

export default function Servers() {
  const { servers, metrics, groups, fetchDashboard } = useServerStore()
  const [view, setView] = useState<'card' | 'table'>('card')
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const [showGroupMgmt, setShowGroupMgmt] = useState(false)
  const [newGroupName, setNewGroupName] = useState('')
  useEffect(() => { fetchDashboard() }, [fetchDashboard])

  // Aggregated stats
  const onlineCount = servers.filter((s) => s.status === 'online').length
  const avgCpu =
    servers.length > 0
      ? servers.reduce((sum, s) => sum + (metrics[s.host_id]?.cpu?.usage_percent ?? 0), 0) / servers.length
      : 0
  const totalTraffic = servers.reduce((sum, s) => {
    const nets = metrics[s.host_id]?.networks
    if (!nets) return sum
    return sum + nets.reduce((n, iface) => n + (iface.rx_bytes_per_sec ?? 0) + (iface.tx_bytes_per_sec ?? 0), 0)
  }, 0)
  const totalContainers = servers.reduce((sum, s) => {
    return sum + (metrics[s.host_id]?.containers?.filter((c) => c.state === 'running').length ?? 0)
  }, 0)

  // Build server metrics for table view
  const serverMetrics = useMemo(() => servers.map((s) => {
    const m = metrics[s.host_id]
    return {
      ...s,
      cpuPercent: m?.cpu?.usage_percent ?? 0,
      memPercent: m?.memory?.usage_percent ?? 0,
      diskPercent: Math.max(0, ...(m?.disks?.map(d => d.usage_percent) ?? [0])),
      netRx: m?.networks?.reduce((sum, n) => sum + (n.rx_bytes_per_sec ?? 0), 0) ?? 0,
      netTx: m?.networks?.reduce((sum, n) => sum + (n.tx_bytes_per_sec ?? 0), 0) ?? 0,
    }
  }), [servers, metrics])

  // Group servers
  const grouped = useMemo(() => {
    const map = new Map<number | 'ungrouped', typeof serverMetrics>()
    for (const g of groups) {
      map.set(g.id, [])
    }
    map.set('ungrouped', [])
    for (const s of serverMetrics) {
      const gid = s.group_id ?? 'ungrouped'
      if (!map.has(gid)) {
        map.get('ungrouped')!.push(s)
      } else {
        map.get(gid)!.push(s)
      }
    }
    const result: { group: ServerGroup | null; servers: typeof serverMetrics }[] = []
    for (const g of groups) {
      const svrs = map.get(g.id) || []
      if (svrs.length > 0) result.push({ group: g, servers: svrs })
    }
    const ungrouped = map.get('ungrouped') || []
    if (ungrouped.length > 0) result.push({ group: null, servers: ungrouped })
    return result
  }, [serverMetrics, groups])

  const toggleCollapse = useCallback((key: string) => {
    setCollapsed(prev => {
      const next = new Set(prev)
      next.has(key) ? next.delete(key) : next.add(key)
      return next
    })
  }, [])

  const handleCreateGroup = async () => {
    if (!newGroupName.trim()) return
    await createGroup(newGroupName.trim())
    setNewGroupName('')
    fetchDashboard()
  }

  const handleDeleteGroup = async (id: number) => {
    await deleteGroup(id)
    fetchDashboard()
  }

  const handleSetGroup = async (hostId: string, groupId: number | null) => {
    await setServerGroup(hostId, groupId)
    fetchDashboard()
  }

  const GroupSelector = ({ hostId, currentGroupId }: { hostId: string; currentGroupId?: number | null }) => (
    <select
      value={currentGroupId ?? ''}
      onChange={(e) => {
        const val = e.target.value
        handleSetGroup(hostId, val ? Number(val) : null)
      }}
      onClick={(e) => e.preventDefault()}
      className="text-[10px] bg-surface-container border border-outline-variant/20 text-on-surface-variant rounded px-1.5 py-0.5 cursor-pointer"
    >
      <option value="">未分组</option>
      {groups.map(g => (
        <option key={g.id} value={g.id}>{g.name}</option>
      ))}
    </select>
  )

  return (
    <div className="flex flex-col gap-6">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-end sm:justify-between gap-4">
        <div>
          <h1 className="font-headline text-2xl md:text-4xl font-bold tracking-tight text-on-surface">
            集群监控中心
          </h1>
          <p className="mt-2 text-on-surface-variant">
            共 {servers.length} 台服务器，{onlineCount} 台在线 · 实时监控
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setShowGroupMgmt(!showGroupMgmt)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs text-on-surface-variant hover:text-on-surface bg-surface-container-low hover:bg-surface-container rounded-lg transition-colors"
          >
            <span className="material-symbols-outlined text-sm">folder_managed</span>
            <span className="hidden sm:inline">管理分组</span>
          </button>
          <div className="flex items-center bg-surface-container-low p-1 rounded-lg">
            <button
              onClick={() => setView('card')}
              className={
                view === 'card'
                  ? 'flex items-center gap-2 px-4 py-1.5 bg-primary-container text-on-primary-container rounded-md shadow-lg'
                  : 'flex items-center gap-2 px-4 py-1.5 text-on-surface-variant hover:text-on-surface'
              }
            >
              <span className="material-symbols-outlined text-sm">grid_view</span>
              <span className="text-sm font-medium hidden sm:inline">卡片</span>
            </button>
            <button
              onClick={() => setView('table')}
              className={
                view === 'table'
                  ? 'flex items-center gap-2 px-4 py-1.5 bg-primary-container text-on-primary-container rounded-md shadow-lg'
                  : 'flex items-center gap-2 px-4 py-1.5 text-on-surface-variant hover:text-on-surface'
              }
            >
              <span className="material-symbols-outlined text-sm">table_rows</span>
              <span className="text-sm font-medium hidden sm:inline">表格</span>
            </button>
          </div>
        </div>
      </div>

      {/* Group Management Panel */}
      {showGroupMgmt && (
        <div className="glass-card rounded-xl p-5">
          <h3 className="text-sm font-headline font-bold text-on-surface mb-4 flex items-center gap-2">
            <span className="material-symbols-outlined text-primary text-lg">folder_managed</span>
            分组管理
          </h3>
          <div className="flex flex-wrap gap-2 mb-4">
            {groups.map(g => (
              <div key={g.id} className="flex items-center gap-2 px-3 py-1.5 bg-surface-container rounded-lg text-sm">
                <span className="text-on-surface">{g.name}</span>
                <button
                  onClick={() => handleDeleteGroup(g.id)}
                  className="text-on-surface-variant hover:text-error transition-colors"
                >
                  <span className="material-symbols-outlined text-sm">close</span>
                </button>
              </div>
            ))}
            {groups.length === 0 && (
              <span className="text-xs text-on-surface-variant">暂无分组</span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleCreateGroup()}
              placeholder="新分组名称"
              className="text-sm px-3 py-1.5 bg-surface-container border border-outline-variant/20 rounded-lg text-on-surface placeholder:text-on-surface-variant/50 focus:outline-none focus:border-primary"
            />
            <button
              onClick={handleCreateGroup}
              className="px-3 py-1.5 text-sm bg-primary text-on-primary rounded-lg hover:bg-primary/80 transition-colors"
            >
              创建
            </button>
          </div>
        </div>
      )}

      {/* Stats Bar */}
      {servers.length > 0 && (
        <div className="bg-surface-container rounded-xl p-5 flex items-center justify-between flex-wrap gap-4">
          <div className="flex items-center gap-2">
            <span className="material-symbols-outlined text-primary text-lg">dns</span>
            <span className="text-xs text-on-surface-variant">服务器</span>
            <span className="text-sm font-bold text-on-surface">{servers.length}</span>
            <span className="text-[10px] text-tertiary ml-1">{onlineCount} 在线</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="material-symbols-outlined text-primary text-lg">speed</span>
            <span className="text-xs text-on-surface-variant">平均 CPU</span>
            <span className={`text-sm font-bold font-mono ${avgCpu >= 80 ? 'text-error' : avgCpu >= 60 ? 'text-warning' : 'text-on-surface'}`}>
              {avgCpu.toFixed(1)}%
            </span>
          </div>
          <div className="flex items-center gap-2">
            <span className="material-symbols-outlined text-primary text-lg">swap_vert</span>
            <span className="text-xs text-on-surface-variant">总流量</span>
            <span className="text-sm font-bold font-mono text-on-surface">{formatBytesPS(totalTraffic)}</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="material-symbols-outlined text-primary text-lg">deployed_code</span>
            <span className="text-xs text-on-surface-variant">运行容器</span>
            <span className="text-sm font-bold text-on-surface">{totalContainers}</span>
          </div>
        </div>
      )}

      {/* Empty State */}
      {servers.length === 0 && (
        <div className="glass-card rounded-2xl p-16 text-center">
          <div className="w-16 h-16 rounded-2xl bg-surface-container-high flex items-center justify-center mx-auto mb-4">
            <span className="material-symbols-outlined text-3xl text-on-surface-variant">dns</span>
          </div>
          <p className="text-on-surface-variant font-body text-lg mb-2">暂无服务器数据</p>
          <p className="text-on-surface-variant/60 text-sm">请先部署 Agent 到目标服务器，数据将自动上报</p>
        </div>
      )}

      {/* Card View */}
      {servers.length > 0 && view === 'card' && (
        <div className="space-y-6">
          {grouped.map(({ group, servers: groupServers }) => {
            const key = group ? `g-${group.id}` : 'ungrouped'
            const isCollapsed = collapsed.has(key)
            const onlineInGroup = groupServers.filter(s => s.status === 'online').length
            return (
              <div key={key} className="space-y-4">
                <button
                  onClick={() => toggleCollapse(key)}
                  className="flex items-center gap-2 w-full text-left"
                >
                  <span className={`material-symbols-outlined text-sm text-on-surface-variant transition-transform ${isCollapsed ? '' : 'rotate-90'}`}>
                    chevron_right
                  </span>
                  <span className="text-sm font-headline font-bold text-on-surface">
                    {group?.name ?? '未分组'}
                  </span>
                  <span className="text-xs text-on-surface-variant">
                    ({onlineInGroup}/{groupServers.length} 在线)
                  </span>
                </button>
                {!isCollapsed && (
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
                    {groupServers.map((s) => (
                      <div key={s.host_id} className="relative">
                        <ServerCard server={s} metrics={metrics[s.host_id]} />
                        {groups.length > 0 && (
                          <div className="absolute top-2 right-16" onClick={(e) => e.stopPropagation()}>
                            <GroupSelector hostId={s.host_id} currentGroupId={s.group_id} />
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* Table View */}
      {servers.length > 0 && view === 'table' && (
        <div className="bg-surface-container-low rounded-xl overflow-hidden border border-outline-variant/10">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-surface-container/50">
                <th className="text-left p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold">状态</th>
                <th className="text-left p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold">主机名</th>
                <th className="text-left p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold hidden md:table-cell">IP 地址</th>
                <th className="text-left p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold hidden md:table-cell">系统</th>
                <th className="text-center p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold">CPU</th>
                <th className="text-center p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold">内存</th>
                <th className="text-center p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold hidden md:table-cell">磁盘</th>
                <th className="text-right p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold">流量</th>
                <th className="text-center p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold hidden md:table-cell">容器</th>
                {groups.length > 0 && (
                  <th className="text-center p-4 text-[10px] text-on-surface-variant uppercase tracking-wider font-bold hidden md:table-cell">分组</th>
                )}
              </tr>
            </thead>
            <tbody>
              {grouped.map(({ group, servers: groupServers }) => {
                const key = group ? `g-${group.id}` : 'ungrouped'
                const isCollapsed = collapsed.has(key)
                const onlineInGroup = groupServers.filter(s => s.status === 'online').length
                const colSpan = groups.length > 0 ? 10 : 9
                return [
                  <tr key={`header-${key}`} className="bg-surface-container/30">
                    <td colSpan={colSpan} className="p-0">
                      <button
                        onClick={() => toggleCollapse(key)}
                        className="flex items-center gap-2 w-full text-left px-4 py-2.5"
                      >
                        <span className={`material-symbols-outlined text-sm text-on-surface-variant transition-transform ${isCollapsed ? '' : 'rotate-90'}`}>
                          chevron_right
                        </span>
                        <span className="text-xs font-headline font-bold text-on-surface">
                          {group?.name ?? '未分组'}
                        </span>
                        <span className="text-[10px] text-on-surface-variant">
                          ({onlineInGroup}/{groupServers.length} 在线)
                        </span>
                      </button>
                    </td>
                  </tr>,
                  ...(!isCollapsed ? groupServers.map((s) => {
                    const m = metrics[s.host_id]
                    const cpuPct = s.cpuPercent
                    const memPct = s.memPercent
                    const diskPct = s.diskPercent
                    const rxPS = s.netRx
                    const txPS = s.netTx
                    const containers = m?.containers?.filter((c) => c.state === 'running').length ?? 0

                    let ip = ''
                    try { ip = JSON.parse(s.ip_addresses || '[]')[0] || '' } catch { /* empty */ }

                    const cpuColor = cpuPct >= 80 ? 'text-error' : cpuPct >= 60 ? 'text-warning' : 'text-tertiary'
                    const memColor = memPct >= 80 ? 'text-error' : memPct >= 60 ? 'text-warning' : 'text-tertiary'
                    const diskColor = diskPct >= 80 ? 'text-error' : diskPct >= 60 ? 'text-warning' : 'text-on-surface'

                    return (
                      <tr
                        key={s.host_id}
                        className="even:bg-surface-container/30 hover:bg-surface-container-high transition-colors"
                      >
                        <td className="p-4">
                          <StatusBadge status={s.status} />
                        </td>
                        <td className="p-4">
                          <Link
                            to={`/servers/${s.host_id}`}
                            className="text-primary hover:text-on-surface font-medium transition-colors"
                          >
                            {s.display_name || s.hostname}
                          </Link>
                        </td>
                        <td className="p-4 text-xs font-mono text-on-surface-variant hidden md:table-cell">{ip}</td>
                        <td className="p-4 text-xs text-on-surface-variant hidden md:table-cell">
                          {s.os?.split(' ').slice(0, 3).join(' ')}
                        </td>
                        <td className={`text-center p-4 font-mono text-xs font-bold ${cpuColor}`}>
                          {metrics[s.host_id] ? cpuPct.toFixed(1) + '%' : '-'}
                        </td>
                        <td className={`text-center p-4 font-mono text-xs font-bold ${memColor}`}>
                          {metrics[s.host_id] ? memPct.toFixed(1) + '%' : '-'}
                        </td>
                        <td className={`text-center p-4 font-mono text-xs hidden md:table-cell ${diskColor}`}>
                          {metrics[s.host_id] ? diskPct.toFixed(1) + '%' : '-'}
                        </td>
                        <td className="text-right p-4 text-xs font-mono text-on-surface-variant">
                          <span className="flex items-center justify-end gap-2">
                            <span className="flex items-center gap-0.5">
                              <span className="material-symbols-outlined text-xs text-primary">arrow_upward</span>
                              {formatBytesPS(txPS)}
                            </span>
                            <span className="flex items-center gap-0.5">
                              <span className="material-symbols-outlined text-xs text-primary">arrow_downward</span>
                              {formatBytesPS(rxPS)}
                            </span>
                          </span>
                        </td>
                        <td className="text-center p-4 hidden md:table-cell">
                          {containers > 0 ? (
                            <span className="text-[10px] py-0.5 px-2 bg-primary/20 text-primary rounded font-bold">
                              {containers}
                            </span>
                          ) : (
                            <span className="text-on-surface-variant text-xs">-</span>
                          )}
                        </td>
                        {groups.length > 0 && (
                          <td className="text-center p-4 hidden md:table-cell">
                            <GroupSelector hostId={s.host_id} currentGroupId={s.group_id} />
                          </td>
                        )}
                      </tr>
                    )
                  }) : []),
                ]
              })}
            </tbody>
          </table>
        </div>
      )}

    </div>
  )
}
