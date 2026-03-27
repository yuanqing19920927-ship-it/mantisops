interface Props {
  status: 'online' | 'offline' | 'up' | 'down'
  label?: string
}

export function StatusBadge({ status, label }: Props) {
  const isGood = status === 'online' || status === 'up'

  if (isGood) {
    return (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded bg-[#0ab39c]/15 text-[#0ab39c] text-[11px] font-medium">
        <span
          className="w-2 h-2 rounded-full bg-[#0ab39c] animate-pulse"
          style={{ boxShadow: '0 0 0 2px rgba(10,179,156,0.25)' }}
        />
        {label ?? (status === 'online' ? 'online' : 'up')}
      </span>
    )
  }

  return (
    <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded bg-[#f06548]/15 text-[#f06548] text-[11px] font-medium">
      <span
        className="w-2 h-2 rounded-full bg-[#f06548]"
        style={{ boxShadow: '0 0 0 2px rgba(240,101,72,0.25)' }}
      />
      {label ?? (status === 'offline' ? 'offline' : 'down')}
    </span>
  )
}
