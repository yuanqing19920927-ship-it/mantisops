import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { getDatabases, type RDSInfo } from '../../api/client'
import { ProgressBar } from '../../components/ProgressBar'
import { AddCloudAccountDialog } from '../../components/AddCloudAccountDialog'

export default function Databases() {
  const [databases, setDatabases] = useState<RDSInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [showAddCloud, setShowAddCloud] = useState(false)

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

  // 统计
  const total = databases.length
  const avgCpu = databases.length > 0
    ? databases.reduce((s, d) => s + (d.metrics?.cpu_usage ?? 0), 0) / databases.length
    : 0
  const avgConn = databases.length > 0
    ? databases.reduce((s, d) => s + (d.metrics?.connection_usage ?? 0), 0) / databases.length
    : 0
  const totalQps = databases.reduce((s, d) => s + (d.metrics?.qps ?? 0), 0)

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-8 h-8 border-2 border-primary/30 border-t-primary rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Add Cloud Account Dialog */}
      <AddCloudAccountDialog
        open={showAddCloud}
        onClose={() => setShowAddCloud(false)}
        onSuccess={fetchData}
      />

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-headline font-bold text-on-surface">数据库监控</h1>
          <p className="text-sm text-on-surface-variant mt-1">阿里云 RDS 实例监控</p>
        </div>
        <button
          onClick={() => setShowAddCloud(true)}
          className="flex items-center gap-1.5 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded-lg transition-colors"
        >
          <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>add</span>
          添加云账号
        </button>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatCard icon="database" label="RDS 实例" value={`${total}`} sub="全部在线" />
        <StatCard icon="speed" label="平均 CPU" value={`${avgCpu.toFixed(1)}%`} sub="使用率" />
        <StatCard icon="link" label="平均连接数" value={`${avgConn.toFixed(1)}%`} sub="使用率" />
        <StatCard icon="query_stats" label="总 QPS" value={`${totalQps.toFixed(0)}`} sub="查询/秒" />
      </div>

      {/* Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        {databases.map((db) => (
          <RDSCard key={db.host_id} db={db} />
        ))}
      </div>
    </div>
  )
}

function StatCard({ icon, label, value, sub }: { icon: string; label: string; value: string; sub: string }) {
  return (
    <div className="glass-card rounded-xl p-4 border border-outline-variant/15">
      <div className="flex items-center gap-2 mb-2">
        <span className="material-symbols-outlined text-primary text-lg">{icon}</span>
        <span className="text-[10px] text-on-surface-variant uppercase font-bold tracking-wider">{label}</span>
      </div>
      <div className="text-2xl font-headline font-bold text-on-surface">{value}</div>
      <div className="text-[10px] text-on-surface-variant mt-1">{sub}</div>
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
      className="glass-card glow-card p-5 rounded-xl flex flex-col transition-all group border border-outline-variant/15"
    >
      {/* Header */}
      <div className="flex justify-between items-start gap-3 mb-4">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-10 h-10 rounded-lg bg-surface-container flex-shrink-0 flex items-center justify-center border border-primary/20">
            <span className="material-symbols-outlined text-primary text-xl">database</span>
          </div>
          <div className="min-w-0">
            <div className="font-bold text-on-surface group-hover:text-primary transition-colors text-sm truncate">
              {displayName}
            </div>
            <div className="text-xs text-on-surface-variant">{engine}</div>
          </div>
        </div>
        <span className="text-[10px] py-0.5 px-2 bg-tertiary/20 text-tertiary rounded font-bold">在线</span>
      </div>

      {/* Progress Bars */}
      <div className="space-y-2 mb-4">
        <ProgressBar percent={m.cpu_usage ?? 0} label="CPU" size="sm" />
        <ProgressBar percent={m.memory_usage ?? 0} label="内存" size="sm" />
        <ProgressBar percent={m.disk_usage ?? 0} label="磁盘" size="sm" />
        <ProgressBar percent={m.connection_usage ?? 0} label="连接数" size="sm" />
      </div>

      {/* Spec */}
      {db.spec && (
        <div className="text-[10px] text-on-surface-variant mb-2 truncate">{db.spec}</div>
      )}

      {/* Stats */}
      <div className="flex justify-between items-center text-xs font-mono text-on-surface-variant mt-auto">
        {isMySQL ? (
          <>
            <span>QPS: <span className="text-primary">{(m.qps ?? 0).toFixed(1)}</span></span>
            <span>TPS: <span className="text-primary">{(m.tps ?? 0).toFixed(1)}</span></span>
            <span>活跃: <span className="text-primary">{(m.active_sessions ?? 0).toFixed(0)}</span></span>
          </>
        ) : (
          <span>IOPS: <span className="text-primary">{(m.iops_usage ?? 0).toFixed(1)}%</span></span>
        )}
      </div>
    </Link>
  )
}
