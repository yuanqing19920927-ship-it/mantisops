import { AreaChart, Area, ResponsiveContainer, Tooltip } from 'recharts'
import { useEffect, useId, useState } from 'react'

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
  const gradId = useId()
  const [data, setData] = useState<DataPoint[]>([])

  const chartColor = color || (value >= 80 ? '#ffb4ab' : value >= 60 ? '#fbbf24' : '#a4c9ff')

  useEffect(() => {
    setData((prev) => {
      const next = [...prev, { time: Date.now(), value }]
      return next.slice(-maxPoints)
    })
  }, [value, maxPoints])

  return (
    <div className="bg-surface-container-low rounded-xl p-4 border border-outline-variant/15">
      <div className="flex justify-between items-center mb-2">
        <span className="text-[10px] text-on-surface-variant uppercase font-bold tracking-wider">{label}</span>
        <span className="text-lg font-bold font-headline" style={{ color: chartColor }}>
          {(value ?? 0).toFixed(1)}{unit}
        </span>
      </div>
      <div className="h-20">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data}>
            <defs>
              <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={chartColor} stopOpacity={0.2} />
                <stop offset="100%" stopColor={chartColor} stopOpacity={0} />
              </linearGradient>
            </defs>
            <Area
              type="monotone"
              dataKey="value"
              stroke={chartColor}
              fill={`url(#${gradId})`}
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
            <Tooltip
              contentStyle={{
                backgroundColor: 'var(--color-surface-container)',
                border: '1px solid var(--color-outline-variant)',
                borderRadius: '8px',
                color: 'var(--color-on-surface)',
                fontSize: '12px',
                boxShadow: '0 8px 24px rgba(0, 0, 0, 0.25)',
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
