import { useEffect, useState, useMemo } from 'react'
import { useParams, Link } from 'react-router-dom'
import { getDatabase, type RDSInfo } from '../../api/client'
import { ProgressBar } from '../../components/ProgressBar'
import { HistoryChart } from '../../components/HistoryChart'
import { formatBytesPS } from '../../utils/format'


const TIME_RANGES = [
  { label: '1h', seconds: 3600, step: 60 },
  { label: '6h', seconds: 21600, step: 300 },
  { label: '24h', seconds: 86400, step: 900 },
  { label: '7d', seconds: 604800, step: 3600 },
]

export default function DatabaseDetail() {
  const { id } = useParams<{ id: string }>()
  const [db, setDb] = useState<RDSInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [rangeIdx, setRangeIdx] = useState(0)

  const fetchData = () => {
    if (!id) return
    getDatabase(id).then((data) => {
      setDb(data)
      setLoading(false)
    }).catch(() => setLoading(false))
  }

  useEffect(() => {
    fetchData()
    const timer = setInterval(fetchData, 60000)
    return () => clearInterval(timer)
  }, [id])

  const displayName = db?.name || id || ''
  const engine = db?.engine || '未知'
  const spec = db?.spec || ''
  const endpoint = db?.endpoint || ''
  const m = db?.metrics || {}
  const isMySQL = engine.toLowerCase().startsWith('mysql')

  const tr = TIME_RANGES[rangeIdx]
  const now = useMemo(() => Math.floor(Date.now() / 1000), [rangeIdx])
  const start = now - tr.seconds

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-8 h-8 border-2 border-primary/30 border-t-primary rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-on-surface-variant">
        <Link to="/databases" className="hover:text-primary transition-colors">数据库</Link>
        <span className="material-symbols-outlined text-xs">chevron_right</span>
        <span className="text-on-surface font-medium">{displayName}</span>
      </div>

      {/* Info Card */}
      <div className="glass-card rounded-xl p-6 border border-outline-variant/15">
        <div className="flex items-center gap-4 mb-4">
          <div className="w-12 h-12 rounded-lg bg-surface-container flex items-center justify-center border border-primary/20">
            <span className="material-symbols-outlined text-primary text-2xl">database</span>
          </div>
          <div>
            <h1 className="text-xl font-headline font-bold text-on-surface">{displayName}</h1>
            <div className="text-sm text-on-surface-variant">
              {engine}{spec ? ` · ${spec}` : ''}
              {db?.account_name && (
                <span className="ml-2 text-[11px] py-0.5 px-1.5 bg-surface-container rounded text-on-surface-variant/70">
                  {db.account_name}
                </span>
              )}
            </div>
          </div>
        </div>
        {endpoint && (
          <div className="text-xs font-mono text-on-surface-variant bg-surface-container-lowest rounded-lg px-3 py-2">
            {endpoint}
          </div>
        )}
      </div>

      {/* Real-time Metrics */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-3">
        <MetricTile label="CPU" value={`${(m.cpu_usage ?? 0).toFixed(1)}%`} percent={m.cpu_usage} />
        <MetricTile label="内存" value={`${(m.memory_usage ?? 0).toFixed(1)}%`} percent={m.memory_usage} />
        <MetricTile label="磁盘" value={`${(m.disk_usage ?? 0).toFixed(1)}%`} percent={m.disk_usage} />
        <MetricTile label="IOPS" value={`${(m.iops_usage ?? 0).toFixed(1)}%`} percent={m.iops_usage} />
        <MetricTile label="连接数" value={`${(m.connection_usage ?? 0).toFixed(1)}%`} percent={m.connection_usage} />
        {isMySQL && (
          <>
            <MetricTile label="QPS" value={`${(m.qps ?? 0).toFixed(1)}`} />
            <MetricTile label="TPS" value={`${(m.tps ?? 0).toFixed(1)}`} />
            <MetricTile label="活跃会话" value={`${(m.active_sessions ?? 0).toFixed(0)}`} />
            <MetricTile label="网络入" value={formatBytesPS(m.network_in_bytes ?? 0)} />
            <MetricTile label="网络出" value={formatBytesPS(m.network_out_bytes ?? 0)} />
          </>
        )}
      </div>

      {/* Time Range Selector */}
      <div className="flex items-center gap-2">
        <span className="text-xs text-on-surface-variant font-bold uppercase">历史趋势</span>
        <div className="flex gap-1 ml-auto">
          {TIME_RANGES.map((r, i) => (
            <button
              key={r.label}
              onClick={() => setRangeIdx(i)}
              className={`px-3 py-1 text-xs font-bold rounded-lg transition-colors ${
                i === rangeIdx
                  ? 'bg-primary text-on-primary'
                  : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'
              }`}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>

      {/* History Charts */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <HistoryChart
          title="CPU / 内存 / 磁盘 使用率"
          queries={[
            { query: `opsboard_rds_cpu_usage{host_id="${id}"}`, label: 'CPU', color: '#a4c9ff' },
            { query: `opsboard_rds_memory_usage{host_id="${id}"}`, label: '内存', color: '#4edea3' },
            { query: `opsboard_rds_disk_usage{host_id="${id}"}`, label: '磁盘', color: '#fbbf24' },
          ]}
          start={start}
          end={now}
          step={tr.step}
          unit="%"
        />
        <HistoryChart
          title="IOPS / 连接数 使用率"
          queries={[
            { query: `opsboard_rds_iops_usage{host_id="${id}"}`, label: 'IOPS', color: '#c084fc' },
            { query: `opsboard_rds_connection_usage{host_id="${id}"}`, label: '连接数', color: '#67e8f9' },
          ]}
          start={start}
          end={now}
          step={tr.step}
          unit="%"
        />
        {isMySQL && (
          <>
            <HistoryChart
              title="QPS / TPS"
              queries={[
                { query: `opsboard_rds_qps{host_id="${id}"}`, label: 'QPS', color: '#a4c9ff' },
                { query: `opsboard_rds_tps{host_id="${id}"}`, label: 'TPS', color: '#4edea3' },
              ]}
              start={start}
              end={now}
              step={tr.step}
            />
            <HistoryChart
              title="网络流量"
              queries={[
                { query: `opsboard_rds_network_in_bytes{host_id="${id}"}`, label: '入站', color: '#a4c9ff' },
                { query: `opsboard_rds_network_out_bytes{host_id="${id}"}`, label: '出站', color: '#fb923c' },
              ]}
              start={start}
              end={now}
              step={tr.step}
              formatValue={(v) => formatBytesPS(v)}
            />
          </>
        )}
      </div>
    </div>
  )
}

function MetricTile({ label, value, percent }: { label: string; value: string; percent?: number }) {
  return (
    <div className="glass-card rounded-xl p-4 border border-outline-variant/15">
      <div className="text-[10px] text-on-surface-variant uppercase font-bold tracking-wider mb-2">{label}</div>
      <div className="text-lg font-headline font-bold text-on-surface mb-2">{value}</div>
      {percent !== undefined && <ProgressBar percent={percent} showValue={false} size="sm" />}
    </div>
  )
}
