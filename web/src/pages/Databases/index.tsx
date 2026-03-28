import { useEffect, useState, useMemo, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { getDatabases, type RDSInfo } from '../../api/client'
import { ProgressBar } from '../../components/ProgressBar'

export default function Databases() {
  const [databases, setDatabases] = useState<RDSInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())

  const fetchData = () => {
    getDatabases().then((data) => {
      setDatabases(data)
      setLoading(false)
    }).catch(() => setLoading(false))
  }

  useEffect(() => {
    fetchData()
    const timer = setInterval(fetchData, 60000)
    return () => clearInterval(timer)
  }, [])

  const toggleCollapse = useCallback((key: string) => {
    setCollapsed(prev => {
      const next = new Set(prev)
      next.has(key) ? next.delete(key) : next.add(key)
      return next
    })
  }, [])

  // Group by cloud account
  const grouped = useMemo(() => {
    const map = new Map<string, RDSInfo[]>()
    for (const db of databases) {
      const key = db.account_name || '默认账号'
      if (!map.has(key)) map.set(key, [])
      map.get(key)!.push(db)
    }
    return Array.from(map.entries()).map(([name, dbs]) => ({ name, databases: dbs }))
  }, [databases])

  const total = databases.length
  const avgCpu = total > 0
    ? databases.reduce((s, d) => s + (d.metrics?.cpu_usage ?? 0), 0) / total
    : 0
  const avgConn = total > 0
    ? databases.reduce((s, d) => s + (d.metrics?.connection_usage ?? 0), 0) / total
    : 0
  const totalQps = databases.reduce((s, d) => s + (d.metrics?.qps ?? 0), 0)

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-8 h-8 border-2 border-[#2ca07a]/30 border-t-[#2ca07a] rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      {/* Header */}
      <div>
        <h1 className="text-[22px] font-semibold text-[#495057]">数据库监控</h1>
        <p className="text-sm text-[#878a99] mt-1">阿里云 RDS 实例监控</p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {[
          { icon: 'database', label: 'RDS 实例', value: `${total}`, sub: '全部在线' },
          { icon: 'speed', label: '平均 CPU', value: `${avgCpu.toFixed(1)}%`, sub: '使用率' },
          { icon: 'link', label: '平均连接数', value: `${avgConn.toFixed(1)}%`, sub: '使用率' },
          { icon: 'query_stats', label: '总 QPS', value: `${totalQps.toFixed(0)}`, sub: '查询/秒' },
        ].map(s => (
          <div key={s.label} className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5">
            <div className="flex items-center gap-2 mb-2">
              <span className="material-symbols-outlined text-[#2ca07a] text-lg">{s.icon}</span>
              <span className="text-[10px] text-[#878a99] uppercase font-bold tracking-wider">{s.label}</span>
            </div>
            <div className="text-2xl font-bold text-[#495057]">{s.value}</div>
            <div className="text-[10px] text-[#878a99] mt-1">{s.sub}</div>
          </div>
        ))}
      </div>

      {/* Grouped cards */}
      {grouped.length > 1 ? (
        grouped.map(({ name, databases: dbs }) => {
          const key = `acct-${name}`
          const isCollapsed = collapsed.has(key)
          return (
            <div key={key} className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
              <button onClick={() => toggleCollapse(key)}
                className="w-full px-5 py-3 flex items-center gap-2 hover:bg-[#f8f9fa] transition-colors text-left border-b border-[#e9ebec]">
                <span className={`material-symbols-outlined text-[16px] text-[#878a99] transition-transform ${isCollapsed ? '-rotate-90' : ''}`}>expand_more</span>
                <span className="material-symbols-outlined text-[16px] text-[#2ca07a]">cloud</span>
                <span className="text-[13px] font-semibold text-[#495057]">{name}</span>
                <span className="text-[11px] text-[#878a99] ml-1">({dbs.length})</span>
              </button>
              {!isCollapsed && (
                <div className="p-5 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
                  {dbs.map(db => <RDSCard key={db.host_id} db={db} />)}
                </div>
              )}
            </div>
          )
        })
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {databases.map(db => <RDSCard key={db.host_id} db={db} />)}
        </div>
      )}
    </div>
  )
}

function RDSCard({ db }: { db: RDSInfo }) {
  const m = db.metrics || {}
  const displayName = db.name || db.host_id
  const engine = db.engine || '未知'
  const isMySQL = engine.toLowerCase().startsWith('mysql')

  return (
    <Link
      to={`/databases/${db.host_id}`}
      className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5 flex flex-col transition-all hover:shadow-[0_2px_8px_rgba(56,65,74,0.15)] border border-transparent hover:border-[#2ca07a]/20"
    >
      <div className="flex justify-between items-start gap-3 mb-4">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-10 h-10 rounded-lg bg-[#f8f9fa] flex-shrink-0 flex items-center justify-center border border-[#e9ebec]">
            <span className="material-symbols-outlined text-[#2ca07a] text-xl">database</span>
          </div>
          <div className="min-w-0">
            <div className="font-semibold text-[#495057] text-sm truncate">{displayName}</div>
            <div className="text-xs text-[#878a99]">{engine}</div>
          </div>
        </div>
        <span className="text-[10px] py-0.5 px-2 bg-[#0ab39c]/10 text-[#0ab39c] rounded font-semibold">在线</span>
      </div>

      <div className="space-y-2 mb-4">
        <ProgressBar percent={m.cpu_usage ?? 0} label="CPU" size="sm" />
        <ProgressBar percent={m.memory_usage ?? 0} label="内存" size="sm" />
        <ProgressBar percent={m.disk_usage ?? 0} label="磁盘" size="sm" />
        <ProgressBar percent={m.connection_usage ?? 0} label="连接数" size="sm" />
      </div>

      {db.spec && <div className="text-[10px] text-[#878a99] mb-2 truncate">{db.spec}</div>}

      <div className="flex justify-between items-center text-xs font-mono text-[#878a99] mt-auto">
        {isMySQL ? (
          <>
            <span>QPS: <span className="text-[#2ca07a]">{(m.qps ?? 0).toFixed(1)}</span></span>
            <span>TPS: <span className="text-[#2ca07a]">{(m.tps ?? 0).toFixed(1)}</span></span>
            <span>活跃: <span className="text-[#2ca07a]">{(m.active_sessions ?? 0).toFixed(0)}</span></span>
          </>
        ) : (
          <span>IOPS: <span className="text-[#2ca07a]">{(m.iops_usage ?? 0).toFixed(1)}%</span></span>
        )}
      </div>
    </Link>
  )
}
