import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAlertStore } from '../stores/alertStore'

export function NotificationBell() {
  const { firingEvents, firingCount, fetchFiringEvents, fetchStats } = useAlertStore()
  const [open, setOpen] = useState(false)
  const [animate, setAnimate] = useState(false)
  const prevCount = useRef(firingCount)
  const panelRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()

  useEffect(() => {
    fetchStats()
    fetchFiringEvents()
  }, [])

  // Flash animation when new alerts arrive
  useEffect(() => {
    if (firingCount > prevCount.current) {
      setAnimate(true)
      const t = setTimeout(() => setAnimate(false), 2000)
      return () => clearTimeout(t)
    }
    prevCount.current = firingCount
  }, [firingCount])

  // Click outside to close
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    if (open) document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  const levelIcon = (level: string) => {
    switch (level) {
      case 'critical': return '🔴'
      case 'warning': return '🟡'
      default: return '🔵'
    }
  }

  return (
    <div className="relative" ref={panelRef}>
      <button
        onClick={() => setOpen(!open)}
        className={`p-2 rounded-lg hover:bg-surface-container-high hover:text-primary transition-all active:scale-95 duration-200 relative ${animate ? 'animate-pulse' : ''}`}
      >
        <span className="material-symbols-outlined text-xl">notifications</span>
        {firingCount > 0 && (
          <span className="absolute -top-0.5 -right-0.5 min-w-[18px] h-[18px] bg-error text-on-error text-xs font-bold rounded-full flex items-center justify-center px-1">
            {firingCount > 99 ? '99+' : firingCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-80 bg-surface-container-low/95 backdrop-blur-lg rounded-xl shadow-2xl border border-outline/10 overflow-hidden z-50">
          <div className="px-4 py-3 border-b border-outline/10">
            <span className="font-display font-semibold text-sm text-on-surface">告警通知</span>
          </div>
          <div className="max-h-80 overflow-y-auto">
            {firingEvents.length === 0 ? (
              <div className="px-4 py-8 text-center text-on-surface-variant text-sm">暂无告警</div>
            ) : (
              firingEvents.slice(0, 10).map((event) => (
                <div key={event.id} className="px-4 py-3 hover:bg-surface-container-high/50 transition-colors cursor-pointer border-b border-outline/5">
                  <div className="flex items-center gap-2">
                    <span>{levelIcon(event.level)}</span>
                    <span className="text-sm font-medium text-on-surface truncate">{event.rule_name}</span>
                    {event.silenced && <span className="text-xs text-on-surface-variant">(已静默)</span>}
                  </div>
                  <div className="text-xs text-on-surface-variant mt-1 truncate">{event.target_label}</div>
                  <div className="text-xs text-on-surface-variant mt-0.5">{new Date(event.fired_at).toLocaleString()}</div>
                </div>
              ))
            )}
          </div>
          <div
            className="px-4 py-3 border-t border-outline/10 text-center text-sm text-primary cursor-pointer hover:bg-surface-container-high/50 transition-colors"
            onClick={() => { setOpen(false); navigate('/alerts') }}
          >
            查看全部
          </div>
        </div>
      )}
    </div>
  )
}
