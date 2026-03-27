import { useEffect, useState, useRef } from 'react'
import {
  AreaChart,
  LineChart,
  Area,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { queryRange } from '../api/vm'

const DEFAULT_COLORS = ['#a4c9ff', '#4edea3', '#fbbf24', '#ffb4ab', '#c084fc', '#67e8f9', '#fb923c']

interface QueryDef {
  query: string
  label: string
  color: string
}

interface HistoryChartProps {
  title: string
  queries: QueryDef[]
  start: number
  end: number
  step: number
  unit?: string
  formatValue?: (v: number) => string
  chartType?: 'area' | 'line'
}

interface SeriesMeta {
  key: string
  label: string
  color: string
}

function formatTime(ts: number, range: number): string {
  const d = new Date(ts * 1000)
  const hh = d.getHours().toString().padStart(2, '0')
  const mm = d.getMinutes().toString().padStart(2, '0')
  const ss = d.getSeconds().toString().padStart(2, '0')
  if (range <= 3600) return `${hh}:${mm}:${ss}`
  if (range <= 86400) return `${hh}:${mm}`
  const mo = (d.getMonth() + 1).toString().padStart(2, '0')
  const dd = d.getDate().toString().padStart(2, '0')
  return `${mo}/${dd} ${hh}:${mm}`
}

export function HistoryChart({
  title,
  queries,
  start,
  end,
  step,
  unit = '',
  formatValue,
  chartType = 'area',
}: HistoryChartProps) {
  const [data, setData] = useState<Record<string, number | string>[]>([])
  const [seriesList, setSeriesList] = useState<SeriesMeta[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const range = end - start

  function fetchData() {
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    setLoading(true)
    setError(null)

    Promise.all(
      queries.map((q) => queryRange(q.query, start, end, step, controller.signal).then((results) => ({ def: q, results })))
    )
      .then((allResults) => {
        if (controller.signal.aborted) return

        // Build series metadata and collect all timestamps
        const tsSet = new Set<number>()
        const series: SeriesMeta[] = []
        const seriesData: Map<string, Map<number, number>> = new Map()

        let colorIdx = 0
        for (const { def, results } of allResults) {
          if (results.length === 0) continue

          for (const r of results) {
            // Build a descriptive key for multi-series results
            let suffix = ''
            const metric = r.metric
            // Common grouping labels
            for (const tag of ['iface', 'mount', 'device', 'gpu', 'name']) {
              if (metric[tag]) {
                suffix += suffix ? ` ${metric[tag]}` : ` (${metric[tag]}`
              }
            }
            if (suffix) suffix += ')'

            const key = results.length > 1 ? `${def.label}${suffix}` : def.label
            const color = results.length > 1
              ? DEFAULT_COLORS[colorIdx++ % DEFAULT_COLORS.length]
              : def.color

            series.push({ key, label: key, color })
            const valMap = new Map<number, number>()
            for (const [ts, val] of r.values) {
              tsSet.add(ts)
              valMap.set(ts, parseFloat(val))
            }
            seriesData.set(key, valMap)
          }
        }

        // Build unified dataset
        const timestamps = Array.from(tsSet).sort((a, b) => a - b)
        const dataset = timestamps.map((ts) => {
          const row: Record<string, number | string> = { time: ts }
          for (const s of series) {
            row[s.key] = seriesData.get(s.key)?.get(ts) ?? 0
          }
          return row
        })

        setSeriesList(series)
        setData(dataset)
        setLoading(false)
      })
      .catch((err) => {
        if (controller.signal.aborted) return
        setError(err.message || 'Failed to fetch data')
        setLoading(false)
      })
  }

  // 序列化 queries 为稳定的 key，避免对象引用变化触发无限重渲染
  const queriesKey = queries.map((q) => q.query).join('|')

  useEffect(() => {
    fetchData()
    return () => { abortRef.current?.abort() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [start, end, step, queriesKey])

  const fmtVal = formatValue ?? ((v: number) => {
    const abs = Math.abs(v)
    const digits = abs === 0 ? 1 : abs < 0.01 ? 4 : abs < 0.1 ? 3 : abs < 10 ? 2 : abs < 100 ? 1 : 0
    return `${v.toFixed(digits)}${unit}`
  })

  // Loading state
  if (loading) {
    return (
      <div className="bg-surface-container-low rounded-xl p-5 border border-outline-variant/15">
        <div className="text-[10px] text-on-surface-variant uppercase font-bold tracking-wider mb-3">{title}</div>
        <div className="flex items-center justify-center h-[200px]">
          <div className="w-6 h-6 border-2 border-primary/30 border-t-primary rounded-full animate-spin" />
        </div>
      </div>
    )
  }

  // Error state
  if (error) {
    return (
      <div className="bg-surface-container-low rounded-xl p-5 border border-outline-variant/15">
        <div className="text-[10px] text-on-surface-variant uppercase font-bold tracking-wider mb-3">{title}</div>
        <div className="flex flex-col items-center justify-center h-[200px] gap-3">
          <span className="material-symbols-outlined text-error text-2xl">error</span>
          <span className="text-xs text-error">{error}</span>
          <button
            onClick={fetchData}
            className="text-xs text-primary hover:text-primary/80 underline"
          >
            重试
          </button>
        </div>
      </div>
    )
  }

  // Empty state
  if (data.length === 0 || seriesList.length === 0) {
    return (
      <div className="bg-surface-container-low rounded-xl p-5 border border-outline-variant/15">
        <div className="text-[10px] text-on-surface-variant uppercase font-bold tracking-wider mb-3">{title}</div>
        <div className="flex items-center justify-center h-[200px]">
          <span className="text-xs text-on-surface-variant">暂无数据</span>
        </div>
      </div>
    )
  }

  const ChartComponent = chartType === 'line' ? LineChart : AreaChart

  return (
    <div className="bg-surface-container-low rounded-xl p-5 border border-outline-variant/15">
      <div className="text-[10px] text-on-surface-variant uppercase font-bold tracking-wider mb-3">{title}</div>
      <ResponsiveContainer width="100%" height={200}>
        <ChartComponent data={data} margin={{ top: 5, right: 10, left: 0, bottom: 0 }}>
          <defs>
            {seriesList.map((s) => (
              <linearGradient key={s.key} id={`grad-${s.key}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={s.color} stopOpacity={0.3} />
                <stop offset="100%" stopColor={s.color} stopOpacity={0} />
              </linearGradient>
            ))}
          </defs>
          <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.04)" />
          <XAxis
            dataKey="time"
            tickFormatter={(v: number) => formatTime(v, range)}
            tick={{ fontSize: 10, fill: 'var(--color-on-surface-variant)' }}
            stroke="rgba(255,255,255,0.06)"
            interval="preserveStartEnd"
          />
          <YAxis
            tickFormatter={(v: number) => fmtVal(v)}
            tick={{ fontSize: 10, fill: 'var(--color-on-surface-variant)' }}
            stroke="rgba(255,255,255,0.06)"
            width={60}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: 'var(--color-surface-container)',
              border: '1px solid var(--color-outline-variant)',
              borderRadius: '8px',
              fontSize: '12px',
              color: 'var(--color-on-surface)',
            }}
            labelFormatter={(v) => formatTime(Number(v), range)}
            formatter={(value, name) => [fmtVal(Number(value)), String(name)]}
          />
          {seriesList.map((s) =>
            chartType === 'line' ? (
              <Line
                key={s.key}
                type="monotone"
                dataKey={s.key}
                stroke={s.color}
                strokeWidth={1.5}
                dot={false}
                name={s.label}
              />
            ) : (
              <Area
                key={s.key}
                type="monotone"
                dataKey={s.key}
                stroke={s.color}
                strokeWidth={1.5}
                fill={`url(#grad-${s.key})`}
                name={s.label}
              />
            )
          )}
        </ChartComponent>
      </ResponsiveContainer>
    </div>
  )
}
