interface Props {
  status: 'online' | 'offline' | 'up' | 'down'
  label?: string
}

export function StatusBadge({ status, label }: Props) {
  const isGood = status === 'online' || status === 'up'
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span
        className="w-2 h-2 rounded-full"
        style={{ backgroundColor: isGood ? 'var(--success)' : 'var(--danger)' }}
      />
      {label && <span style={{ color: 'var(--text-secondary)' }}>{label}</span>}
    </span>
  )
}
