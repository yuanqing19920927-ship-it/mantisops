import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useServerStore } from '../../stores/serverStore'

export default function Containers() {
  const { servers, metrics, fetchDashboard } = useServerStore()
  useEffect(() => { fetchDashboard() }, [fetchDashboard])
  const [filter, setFilter] = useState<'all' | 'running' | 'stopped'>('all')
  const [search, setSearch] = useState('')

  // Aggregate containers from all servers
  const allContainers = useMemo(() => {
    const list: {
      serverId: string
      serverName: string
      containerId: string
      name: string
      image: string
      state: string
      status: string
      cpuPercent: number
      memoryUsage: number
      memoryLimit: number
      ports: string[]
    }[] = []

    for (const s of servers) {
      const m = metrics[s.host_id]
      if (!m?.containers) continue
      for (const c of m.containers) {
        list.push({
          serverId: s.host_id,
          serverName: s.display_name || s.hostname,
          containerId: c.container_id ?? '',
          name: c.name ?? '',
          image: c.image ?? '',
          state: c.state ?? '',
          status: c.status ?? '',
          cpuPercent: c.cpu_percent ?? 0,
          memoryUsage: c.memory_usage ?? 0,
          memoryLimit: c.memory_limit ?? 0,
          ports: c.ports ?? [],
        })
      }
    }
    return list
  }, [servers, metrics])

  const filtered = useMemo(() => {
    let list = allContainers
    if (filter === 'running') list = list.filter(c => c.state === 'running')
    if (filter === 'stopped') list = list.filter(c => c.state !== 'running')
    if (search) {
      const q = search.toLowerCase()
      list = list.filter(c =>
        c.name.toLowerCase().includes(q) ||
        c.image.toLowerCase().includes(q) ||
        c.serverName.toLowerCase().includes(q)
      )
    }
    return list
  }, [allContainers, filter, search])

  const runningCount = allContainers.filter(c => c.state === 'running').length
  const stoppedCount = allContainers.length - runningCount
  const serverCount = new Set(allContainers.map(c => c.serverId)).size

  const formatMemory = (bytes: number) => {
    if (bytes === 0) return '-'
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
  }

  return (
    <div className="flex flex-col gap-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h4 className="text-[18px] font-semibold text-[#495057]">容器管理</h4>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
              <span className="material-symbols-outlined text-[20px] text-[#2ca07a]">deployed_code</span>
            </div>
            <div>
              <div className="text-2xl font-bold text-[#495057]">{allContainers.length}</div>
              <div className="text-[11px] text-[#878a99]">总容器数</div>
            </div>
          </div>
        </div>
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-full bg-[#0ab39c]/15 flex items-center justify-center">
              <span className="material-symbols-outlined text-[20px] text-[#0ab39c]">play_circle</span>
            </div>
            <div>
              <div className="text-2xl font-bold text-[#0ab39c]">{runningCount}</div>
              <div className="text-[11px] text-[#878a99]">运行中</div>
            </div>
          </div>
        </div>
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-full bg-[#f7b84b]/15 flex items-center justify-center">
              <span className="material-symbols-outlined text-[20px] text-[#f7b84b]">stop_circle</span>
            </div>
            <div>
              <div className="text-2xl font-bold text-[#f7b84b]">{stoppedCount}</div>
              <div className="text-[11px] text-[#878a99]">已停止</div>
            </div>
          </div>
        </div>
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
              <span className="material-symbols-outlined text-[20px] text-[#2ca07a]">dns</span>
            </div>
            <div>
              <div className="text-2xl font-bold text-[#495057]">{serverCount}</div>
              <div className="text-[11px] text-[#878a99]">宿主机</div>
            </div>
          </div>
        </div>
      </div>

      {/* Table */}
      <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
        {/* Toolbar */}
        <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center justify-between flex-wrap gap-3">
          <div className="flex">
            {([
              { key: 'all', label: '全部', count: allContainers.length },
              { key: 'running', label: '运行中', count: runningCount },
              { key: 'stopped', label: '已停止', count: stoppedCount },
            ] as const).map((tab, idx, arr) => (
              <button
                key={tab.key}
                onClick={() => setFilter(tab.key)}
                className={`px-3 py-1.5 text-[12px] font-medium border transition-colors ${
                  idx === 0 ? 'rounded-l' : ''
                }${idx === arr.length - 1 ? 'rounded-r' : ''
                }${idx > 0 ? 'border-l-0' : ''} ${
                  filter === tab.key
                    ? 'bg-[#2ca07a] border-[#2ca07a] text-white'
                    : 'bg-white border-[#ced4da] text-[#878a99] hover:text-[#495057]'
                }`}
              >
                {tab.label} <span className="ml-0.5 opacity-70">{tab.count}</span>
              </button>
            ))}
          </div>
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="搜索容器名、镜像、服务器..."
            className="text-[12px] px-3 py-1.5 bg-[#f8f9fa] border border-[#e9ecef] rounded text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] w-64"
          />
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-[13px]">
            <thead>
              <tr className="bg-[#f8f9fa]">
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">状态</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">容器名</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">镜像</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec] hidden md:table-cell">宿主机</th>
                <th className="text-center text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">CPU</th>
                <th className="text-center text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">内存</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec] hidden lg:table-cell">端口</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec] hidden lg:table-cell">运行状态</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((c) => {
                const isRunning = c.state === 'running'
                const cpuColor = c.cpuPercent >= 80 ? 'text-[#f06548] font-semibold' : c.cpuPercent >= 50 ? 'text-[#f7b84b] font-semibold' : 'text-[#495057]'

                return (
                  <tr
                    key={`${c.serverId}-${c.containerId}`}
                    className="border-b border-[#f2f4f7] hover:bg-[#f8f9fa] transition-colors"
                  >
                    <td className="px-5 py-3">
                      <span className={`inline-flex items-center gap-1.5 text-[11px] font-medium px-2 py-0.5 rounded ${
                        isRunning
                          ? 'bg-[#0ab39c]/10 text-[#0ab39c]'
                          : 'bg-[#878a99]/10 text-[#878a99]'
                      }`}>
                        <span className={`w-1.5 h-1.5 rounded-full ${isRunning ? 'bg-[#0ab39c]' : 'bg-[#878a99]'}`} />
                        {isRunning ? 'running' : c.state}
                      </span>
                    </td>
                    <td className="px-5 py-3">
                      <div className="font-medium text-[#495057]">{c.name}</div>
                      <div className="text-[11px] font-mono text-[#878a99] mt-0.5">{c.containerId}</div>
                    </td>
                    <td className="px-5 py-3">
                      <span className="text-[12px] text-[#878a99] font-mono break-all">{c.image}</span>
                    </td>
                    <td className="px-5 py-3 hidden md:table-cell">
                      <Link
                        to={`/servers/${c.serverId}`}
                        className="text-[#2ca07a] hover:text-[#1f7d5e] text-[12px] font-medium transition-colors"
                      >
                        {c.serverName}
                      </Link>
                    </td>
                    <td className={`text-center px-5 py-3 font-mono text-[12px] ${cpuColor}`}>
                      {isRunning ? `${c.cpuPercent.toFixed(1)}%` : '-'}
                    </td>
                    <td className="text-center px-5 py-3 font-mono text-[12px] text-[#495057]">
                      {isRunning ? `${formatMemory(c.memoryUsage)} / ${formatMemory(c.memoryLimit)}` : '-'}
                    </td>
                    <td className="px-5 py-3 hidden lg:table-cell">
                      {c.ports.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {c.ports.map((p, i) => (
                            <span key={i} className="text-[11px] px-1.5 py-0.5 bg-[#f8f9fa] border border-[#e9ecef] rounded font-mono text-[#878a99]">
                              {p}
                            </span>
                          ))}
                        </div>
                      ) : (
                        <span className="text-[#ced4da] text-[12px]">—</span>
                      )}
                    </td>
                    <td className="px-5 py-3 text-[12px] text-[#878a99] hidden lg:table-cell">
                      {c.status}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>

          {filtered.length === 0 && (
            <div className="py-12 text-center">
              <span className="material-symbols-outlined text-3xl text-[#ced4da] mb-2 block">deployed_code</span>
              <p className="text-[#878a99] text-sm">
                {allContainers.length === 0 ? '暂无容器数据，请确保 Agent 已开启 Docker 采集' : '没有匹配的容器'}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
