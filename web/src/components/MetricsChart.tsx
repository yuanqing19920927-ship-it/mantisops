import { AreaChart, Area, ResponsiveContainer, Tooltip } from 'recharts'
import { useEffect, useState } from 'react'

interface Props {
  value: number
  label: string
  unit?: string
  color?: string
  maxPoints?: number
}

interface DataPoint {
  time: number
  value: number
}

export function MetricsChart({ value, label, unit = '%', color, maxPoints = 60 }: Props) {
  const [data, setData] = useState<DataPoint[]>([])
  const chartColor = color || (value >= 80 ? 'var(--danger)' : value >= 60 ? 'var(--warning)' : 'var(--success)')

  useEffect(() => {
    setData((prev) => {
      const next = [...prev, { time: Date.now(), value }]
      return next.slice(-maxPoints)
    })
  }, [value, maxPoints])

  return (
    <div
      className="rounded-xl p-4"
      style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}
    >
      <div className="flex justify-between items-center mb-2">
        <span className="text-sm" style={{ color: 'var(--text-secondary)' }}>{label}</span>
        <span className="text-lg font-bold" style={{ color: chartColor }}>
          {value.toFixed(1)}{unit}
        </span>
      </div>
      <div className="h-20">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data}>
            <defs>
              <linearGradient id={`grad-${label}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={chartColor} stopOpacity={0.3} />
                <stop offset="100%" stopColor={chartColor} stopOpacity={0} />
              </linearGradient>
            </defs>
            <Area
              type="monotone"
              dataKey="value"
              stroke={chartColor}
              fill={`url(#grad-${label})`}
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
            <Tooltip
              contentStyle={{
                backgroundColor: 'var(--bg-card)',
                border: '1px solid var(--border)',
                borderRadius: '8px',
                color: 'var(--text-primary)',
                fontSize: '12px',
              }}
              formatter={(val) => [`${Number(val ?? 0).toFixed(1)}${unit}`, label]}
              labelFormatter={() => ''}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
