interface Props {
  percent: number
  label?: string
  showValue?: boolean
}

export function ProgressBar({ percent, label, showValue = true }: Props) {
  const clamped = Math.min(100, Math.max(0, percent))
  const color = clamped >= 80 ? 'var(--danger)' : clamped >= 60 ? 'var(--warning)' : 'var(--success)'

  return (
    <div className="mb-2">
      {(label || showValue) && (
        <div className="flex justify-between text-xs mb-1" style={{ color: 'var(--text-secondary)' }}>
          {label && <span>{label}</span>}
          {showValue && <span style={{ color }}>{clamped.toFixed(1)}%</span>}
        </div>
      )}
      <div className="w-full h-2 rounded-full" style={{ backgroundColor: 'var(--border)' }}>
        <div
          className="h-2 rounded-full transition-all duration-500"
          style={{ width: `${clamped}%`, backgroundColor: color }}
        />
      </div>
    </div>
  )
}
