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
  const totalRx = servers.reduce((sum, s) => {
    const nets = metrics[s.host_id]?.networks
    if (!nets) return sum
    return sum + nets.reduce((n, iface) => n + (iface.rx_bytes_per_sec ?? 0), 0)
  }, 0)
  const totalTx = servers.reduce((sum, s) => {
    const nets = metrics[s.host_id]?.networks
    if (!nets) return sum
    return sum + nets.reduce((n, iface) => n + (iface.tx_bytes_per_sec ?? 0), 0)
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

  const avgCpuColor =
    avgCpu >= 80 ? 'text-[#f06548]' : avgCpu >= 60 ? 'text-[#f7b84b]' : 'text-[#495057]'

  return (
    <div className="flex flex-col gap-5 pb-16">

      {/* Page Header */}
      <div className="flex items-center justify-between">
        <h4 className="text-[18px] font-semibold text-[#495057] mb-0">服务器列表</h4>
        <div className="flex items-center gap-2">
          {/* Group management toggle */}
          <button
            onClick={() => setShowGroupMgmt(!showGroupMgmt)}
            className={`flex items-center gap-1 px-3 py-1.5 text-[13px] border rounded transition-colors ${
              showGroupMgmt
                ? 'border-[#2ca07a] text-[#2ca07a] bg-[rgba(44,160,122,0.05)]'
                : 'border-[#ced4da] text-[#878a99] hover:border-[#2ca07a] hover:text-[#2ca07a]'
            }`}
          >
            <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>folder_managed</span>
            <span className="hidden sm:inline">管理分组</span>
          </button>

          {/* Card / Table toggle (btn-group style) */}
          <div className="flex">
            <button
              onClick={() => setView('card')}
              className={`flex items-center px-3 py-1.5 border text-[13px] rounded-l transition-colors ${
                view === 'card'
                  ? 'bg-[#2ca07a] border-[#2ca07a] text-white'
                  : 'bg-white border-[#2ca07a] text-[#2ca07a] hover:bg-[rgba(44,160,122,0.05)]'
              }`}
              title="卡片视图"
            >
              <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>grid_view</span>
            </button>
            <button
              onClick={() => setView('table')}
              className={`flex items-center px-3 py-1.5 border-t border-b border-r text-[13px] rounded-r transition-colors ${
                view === 'table'
                  ? 'bg-[#2ca07a] border-[#2ca07a] text-white'
                  : 'bg-white border-[#2ca07a] text-[#2ca07a] hover:bg-[rgba(44,160,122,0.05)]'
              }`}
              title="表格视图"
            >
              <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>view_list</span>
            </button>
          </div>

        </div>
      </div>

      {/* Group Management Panel */}
      {showGroupMgmt && (
        <div className="bg-white rounded-[10px] border border-[#e9ecef] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-4">
          <h6 className="text-[13px] font-semibold text-[#495057] mb-3 flex items-center gap-2">
            <span className="material-symbols-outlined text-[#2ca07a]" style={{ fontSize: '16px' }}>folder_managed</span>
            分组管理
          </h6>
          <div className="flex flex-wrap gap-2 mb-3">
            {groups.map(g => (
              <div key={g.id} className="flex items-center gap-1.5 px-3 py-1 bg-[#f8f9fa] border border-[#e9ecef] rounded text-[12px] text-[#495057]">
                <span>{g.name}</span>
                <button
                  onClick={() => handleDeleteGroup(g.id)}
                  className="text-[#878a99] hover:text-[#f06548] transition-colors leading-none"
                >
                  <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>close</span>
                </button>
              </div>
            ))}
            {groups.length === 0 && (
              <span className="text-[12px] text-[#878a99]">暂无分组</span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleCreateGroup()}
              placeholder="新分组名称"
              className="text-[12px] px-3 py-1.5 bg-[#f8f9fa] border border-[#e9ecef] rounded text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a]"
            />
            <button
              onClick={handleCreateGroup}
              className="px-3 py-1.5 text-[12px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors"
            >
              创建
            </button>
          </div>
        </div>
      )}

      {/* Empty State */}
      {servers.length === 0 && (
        <div className="bg-white rounded-[10px] border border-[#e9ecef] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-16 text-center">
          <div className="w-14 h-14 rounded-full bg-[rgba(44,160,122,0.1)] flex items-center justify-center mx-auto mb-4">
            <span className="material-symbols-outlined text-[#2ca07a] text-3xl">dns</span>
          </div>
          <p className="text-[#495057] text-[15px] mb-1">暂无服务器数据</p>
          <p className="text-[#878a99] text-[12px]">请先部署 Agent 到目标服务器，数据将自动上报</p>
        </div>
      )}

      {/* Card View */}
      {servers.length > 0 && view === 'card' && (
        <div className="space-y-5">
          {grouped.map(({ group, servers: groupServers }) => {
            const key = group ? `g-${group.id}` : 'ungrouped'
            const isCollapsed = collapsed.has(key)
            const onlineInGroup = groupServers.filter(s => s.status === 'online').length
            return (
              <div key={key}>
                {/* Group header row */}
                <button
                  onClick={() => toggleCollapse(key)}
                  className="flex items-center gap-2 w-full text-left mb-3"
                >
                  <span className={`material-symbols-outlined text-[#878a99] transition-transform ${isCollapsed ? '' : 'rotate-90'}`} style={{ fontSize: '16px' }}>
                    chevron_right
                  </span>
                  <span className="text-[13px] font-semibold text-[#495057]">
                    {group?.name ?? '未分组'}
                  </span>
                  <span className="text-[12px] text-[#878a99]">
                    ({onlineInGroup}/{groupServers.length} 在线)
                  </span>
                </button>
                {!isCollapsed && (
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                    {groupServers.map((s) => (
                      <ServerCard
                        key={s.host_id}
                        server={s}
                        metrics={metrics[s.host_id]}
                        groups={groups.length > 0 ? groups : undefined}
                        onGroupChange={groups.length > 0 ? handleSetGroup : undefined}
                      />
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
        <div className="bg-white rounded-[10px] border border-[#e9ecef] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
          <table className="w-full text-[13px]">
            <thead>
              <tr className="bg-[#f8f9fa] border-b border-[#e9ecef]">
                <th className="text-left px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide">状态</th>
                <th className="text-left px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide">主机名</th>
                <th className="text-left px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide hidden md:table-cell">IP 地址</th>
                <th className="text-left px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide hidden md:table-cell">系统</th>
                <th className="text-center px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide">CPU</th>
                <th className="text-center px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide">内存</th>
                <th className="text-center px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide hidden md:table-cell">磁盘</th>
                <th className="text-right px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide">流量</th>
                <th className="text-center px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide hidden md:table-cell">容器</th>
                {groups.length > 0 && (
                  <th className="text-center px-4 py-3 text-[11px] text-[#878a99] font-semibold uppercase tracking-wide hidden md:table-cell">分组</th>
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
                  <tr key={`header-${key}`} className="bg-[#f3f3f9] border-b border-[#e9ecef]">
                    <td colSpan={colSpan} className="p-0">
                      <button
                        onClick={() => toggleCollapse(key)}
                        className="flex items-center gap-2 w-full text-left px-4 py-2"
                      >
                        <span className={`material-symbols-outlined text-[#878a99] transition-transform ${isCollapsed ? '' : 'rotate-90'}`} style={{ fontSize: '14px' }}>
                          chevron_right
                        </span>
                        <span className="text-[12px] font-semibold text-[#495057]">
                          {group?.name ?? '未分组'}
                        </span>
                        <span className="text-[11px] text-[#878a99]">
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
                    const hasMetrics = !!m

                    let ip = ''
                    try { ip = JSON.parse(s.ip_addresses || '[]')[0] || '' } catch { /* empty */ }

                    const pctColor = (v: number) =>
                      v >= 80 ? 'text-[#f06548] font-semibold' : v >= 60 ? 'text-[#f7b84b] font-semibold' : 'text-[#495057]'

                    return (
                      <tr
                        key={s.host_id}
                        className="border-b border-[#e9ecef] hover:bg-[#f8f9fa] transition-colors"
                      >
                        <td className="px-4 py-3">
                          <StatusBadge status={s.status} />
                        </td>
                        <td className="px-4 py-3">
                          <Link
                            to={`/servers/${s.host_id}`}
                            className="text-[#2ca07a] hover:text-[#1f7d5e] font-medium transition-colors"
                          >
                            {s.display_name || s.hostname}
                          </Link>
                        </td>
                        <td className="px-4 py-3 font-mono text-[12px] text-[#878a99] hidden md:table-cell">{ip}</td>
                        <td className="px-4 py-3 text-[12px] text-[#878a99] hidden md:table-cell">
                          {s.os?.split(' ').slice(0, 3).join(' ')}
                        </td>
                        <td className={`text-center px-4 py-3 font-mono text-[12px] ${pctColor(cpuPct)}`}>
                          {hasMetrics ? cpuPct.toFixed(1) + '%' : '-'}
                        </td>
                        <td className={`text-center px-4 py-3 font-mono text-[12px] ${pctColor(memPct)}`}>
                          {hasMetrics ? memPct.toFixed(1) + '%' : '-'}
                        </td>
                        <td className={`text-center px-4 py-3 font-mono text-[12px] hidden md:table-cell ${pctColor(diskPct)}`}>
                          {hasMetrics ? diskPct.toFixed(1) + '%' : '-'}
                        </td>
                        <td className="text-right px-4 py-3 text-[12px] font-mono text-[#878a99]">
                          <span className="flex items-center justify-end gap-3">
                            <span className="flex items-center gap-0.5">
                              <span className="material-symbols-outlined" style={{ fontSize: '13px', color: '#f06548' }}>arrow_downward</span>
                              {hasMetrics ? formatBytesPS(rxPS) : '-'}
                            </span>
                            <span className="flex items-center gap-0.5">
                              <span className="material-symbols-outlined" style={{ fontSize: '13px', color: '#0ab39c' }}>arrow_upward</span>
                              {hasMetrics ? formatBytesPS(txPS) : '-'}
                            </span>
                          </span>
                        </td>
                        <td className="text-center px-4 py-3 hidden md:table-cell">
                          {containers > 0 ? (
                            <span className="text-[11px] py-0.5 px-2 bg-[rgba(44,160,122,0.1)] text-[#2ca07a] rounded font-semibold">
                              {containers}
                            </span>
                          ) : (
                            <span className="text-[#878a99] text-[12px]">-</span>
                          )}
                        </td>
                        {groups.length > 0 && (
                          <td className="text-center px-4 py-3 hidden md:table-cell">
                            <select
                              value={s.group_id ?? ''}
                              onChange={(e) => {
                                const val = e.target.value
                                handleSetGroup(s.host_id, val ? Number(val) : null)
                              }}
                              className="text-[11px] bg-[#f8f9fa] border border-[#e9ecef] text-[#878a99] rounded px-1.5 py-0.5 cursor-pointer focus:outline-none focus:border-[#2ca07a]"
                            >
                              <option value="">未分组</option>
                              {groups.map(g => (
                                <option key={g.id} value={g.id}>{g.name}</option>
                              ))}
                            </select>
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

      {/* Bottom Stats Bar — fixed bottom */}
      {servers.length > 0 && (
        <div className="fixed bottom-0 left-0 right-0 bg-white/90 backdrop-blur-sm border-t border-[#e9ecef] z-50 px-6 py-2.5 flex items-center gap-6 md:gap-8"
          style={{ left: 'var(--sidebar-width, 0px)' }}
        >
          <div>
            <span className="text-[12px] text-[#878a99] mr-2">服务器</span>
            <span className="text-[13px] font-bold text-[#495057]">{onlineCount} / {servers.length}</span>
          </div>
          <div>
            <span className="text-[12px] text-[#878a99] mr-2">平均 CPU</span>
            <span className={`text-[13px] font-bold ${avgCpuColor}`}>{avgCpu.toFixed(1)}%</span>
          </div>
          <div>
            <span className="text-[12px] text-[#878a99] mr-2">总流量</span>
            <span className="text-[13px] font-bold text-[#495057]">
              <span className="material-symbols-outlined" style={{ fontSize: '12px', color: '#f06548', verticalAlign: 'middle' }}>arrow_downward</span>
              {' '}{formatBytesPS(totalRx)}
              {'  '}
              <span className="material-symbols-outlined ml-2" style={{ fontSize: '12px', color: '#0ab39c', verticalAlign: 'middle' }}>arrow_upward</span>
              {' '}{formatBytesPS(totalTx)}
            </span>
          </div>
          <div className="hidden md:block">
            <span className="text-[12px] text-[#878a99] mr-2">运行容器</span>
            <span className="text-[13px] font-bold text-[#495057]">{totalContainers}</span>
          </div>
        </div>
      )}

    </div>
  )
}
