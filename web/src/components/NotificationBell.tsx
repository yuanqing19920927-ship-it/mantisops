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
      prevCount.current = firingCount
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

  const levelColor = (level: string) => {
    switch (level) {
      case 'critical': return 'text-[#f06548]'
      case 'warning': return 'text-[#f7b84b]'
      default: return 'text-[#2ca07a]'
    }
  }

  const levelIcon = (level: string) => {
    switch (level) {
      case 'critical': return 'error'
      case 'warning': return 'warning'
      default: return 'info'
    }
  }

  return (
    <div className="relative" ref={panelRef}>
      <button
        onClick={() => setOpen(!open)}
        className={`w-8 h-8 flex items-center justify-center rounded-lg text-[#878a99] hover:text-[#495057] hover:bg-[#eff2f7] transition-colors relative ${animate ? 'animate-pulse' : ''}`}
      >
        <span className="material-symbols-outlined text-[18px]">notifications</span>
        {firingCount > 0 && (
          <span className="absolute -top-0.5 -right-0.5 min-w-[16px] h-4 bg-[#f06548] text-white text-[10px] font-bold rounded-full flex items-center justify-center px-0.5">
            {firingCount > 99 ? '99+' : firingCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-80 bg-white rounded-[10px] shadow-[0_4px_24px_rgba(56,65,74,0.15)] border border-[#e9ebec] overflow-hidden z-50">
          <div className="px-4 py-3 border-b border-[#e9ebec] flex items-center justify-between">
            <span className="font-semibold text-sm text-[#495057]">告警通知</span>
            {firingCount > 0 && (
              <span className="text-[11px] px-1.5 py-0.5 rounded bg-[#f06548]/10 text-[#f06548] font-medium">
                {firingCount} 条活跃
              </span>
            )}
          </div>
          <div className="max-h-72 overflow-y-auto">
            {firingEvents.length === 0 ? (
              <div className="px-4 py-8 text-center">
                <span className="material-symbols-outlined text-2xl text-[#ced4da] block mb-2">notifications_off</span>
                <p className="text-[#878a99] text-sm">暂无告警</p>
              </div>
            ) : (
              firingEvents.slice(0, 10).map((event) => (
                <div
                  key={event.id}
                  className="px-4 py-3 hover:bg-[#f8f9fa] transition-colors cursor-pointer border-b border-[#f2f2f2] last:border-0"
                >
                  <div className="flex items-start gap-2">
                    <span className={`material-symbols-outlined text-base mt-0.5 flex-shrink-0 ${levelColor(event.level)}`}>
                      {levelIcon(event.level)}
                    </span>
                    <div className="min-w-0">
                      <div className="flex items-center gap-1.5">
                        <span className="text-sm font-medium text-[#495057] truncate">{event.rule_name}</span>
                        {event.silenced && (
                          <span className="text-[10px] px-1 py-0.5 rounded bg-[#eff2f7] text-[#878a99] flex-shrink-0">已静默</span>
                        )}
                      </div>
                      <div className="text-[11px] text-[#878a99] mt-0.5 truncate">{event.target_label}</div>
                      <div className="text-[11px] text-[#adb5bd] mt-0.5">{new Date(event.fired_at).toLocaleString()}</div>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
          <div
            className="px-4 py-2.5 border-t border-[#e9ebec] text-center text-sm text-[#2ca07a] font-medium cursor-pointer hover:bg-[#f8f9fa] transition-colors"
            onClick={() => { setOpen(false); navigate('/alerts') }}
          >
            查看全部告警
          </div>
        </div>
      )}
    </div>
  )
}
