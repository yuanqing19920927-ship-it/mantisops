interface Props {
  percent: number
  label?: string
  showValue?: boolean
  size?: 'sm' | 'md'
}

export function ProgressBar({ percent, label, showValue = true }: Props) {
  const clamped = Math.min(100, Math.max(0, percent))
  const barColor =
    clamped >= 80
      ? 'bg-[#f06548]'
      : clamped >= 60
      ? 'bg-[#f7b84b]'
      : 'bg-[#0ab39c]'
  const textColor =
    clamped >= 80
      ? 'text-[#f06548]'
      : clamped >= 60
      ? 'text-[#f7b84b]'
      : 'text-[#0ab39c]'

  return (
    <div>
      {(label || showValue) && (
        <div className="flex justify-between mb-1">
          {label && (
            <span className="text-[11px] text-[#878a99]">{label}</span>
          )}
          {showValue && (
            <span className={`text-[11px] font-medium ${textColor}`}>
              {clamped.toFixed(1)}%
            </span>
          )}
        </div>
      )}
      <div className="w-full h-[5px] bg-[#eff2f7] rounded-full overflow-hidden">
        <div
          className={`h-[5px] ${barColor} rounded-full transition-all duration-500`}
          style={{ width: `${clamped}%` }}
        />
      </div>
    </div>
  )
}
