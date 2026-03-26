import { useEffect, useState } from 'react'
import { getBilling, type BillingItem } from '../../api/client'

export default function Billing() {
  const [items, setItems] = useState<BillingItem[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    getBilling()
      .then((data) => {
        setItems(data.sort((a, b) => a.days_left - b.days_left))
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }, [])

  const urgent = items.filter((i) => i.days_left >= 0 && i.days_left <= 30)
  const warning = items.filter((i) => i.days_left > 30 && i.days_left <= 60)
  const ecsCount = items.filter((i) => i.type === 'ecs').length
  const rdsCount = items.filter((i) => i.type === 'rds').length
  const sslCount = items.filter((i) => i.type === 'ssl').length

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-8 h-8 border-2 border-primary/30 border-t-primary rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-headline font-bold text-on-surface">资源到期情况</h1>
        <p className="text-sm text-on-surface-variant mt-1">阿里云 ECS / RDS / SSL 续费到期提醒</p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
        <StatCard
          icon="error"
          label="紧急（30天内）"
          value={`${urgent.length}`}
          color={urgent.length > 0 ? 'text-error' : 'text-tertiary'}
          borderColor={urgent.length > 0 ? 'border-error' : 'border-tertiary'}
        />
        <StatCard
          icon="warning"
          label="预警（60天内）"
          value={`${warning.length}`}
          color={warning.length > 0 ? 'text-warning' : 'text-tertiary'}
          borderColor={warning.length > 0 ? 'border-warning' : 'border-tertiary'}
        />
        <StatCard icon="dns" label="ECS 实例" value={`${ecsCount}`} color="text-primary" borderColor="border-primary" />
        <StatCard icon="database" label="RDS 实例" value={`${rdsCount}`} color="text-tertiary" borderColor="border-tertiary" />
        <StatCard icon="lock" label="SSL 证书" value={`${sslCount}`} color="text-warning" borderColor="border-warning" />
      </div>

      {/* Urgent Alert */}
      {urgent.length > 0 && (
        <div className="bg-error/10 border border-error/30 rounded-xl p-4 flex items-start gap-3">
          <span className="material-symbols-outlined text-error text-xl mt-0.5">notification_important</span>
          <div>
            <div className="text-sm font-bold text-error mb-1">
              {urgent.length} 个资源将在 30 天内到期
            </div>
            <div className="text-xs text-error/80">
              {urgent.map((i) => `${i.name}（${i.days_left}天）`).join('、')}
            </div>
          </div>
        </div>
      )}

      {/* Table */}
      <div className="glass-card rounded-xl p-6 border border-outline-variant/15">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-[10px] text-on-surface-variant uppercase tracking-wider border-b border-outline-variant/15">
                <th className="text-left py-3 px-3">类型</th>
                <th className="text-left py-3 px-3">名称</th>
                <th className="text-left py-3 px-3">规格 / 引擎</th>
                <th className="text-left py-3 px-3">计费方式</th>
                <th className="text-left py-3 px-3">到期日期</th>
                <th className="text-right py-3 px-3">剩余天数</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => {
                const isUrgent = item.days_left >= 0 && item.days_left <= 30
                const isWarning = item.days_left > 30 && item.days_left <= 60
                return (
                  <tr
                    key={`${item.type}-${item.id}`}
                    className={`border-b border-outline-variant/10 transition-colors ${
                      isUrgent ? 'bg-error/5' : isWarning ? 'bg-warning/5' : 'hover:bg-surface-container-low'
                    }`}
                  >
                    <td className="py-3 px-3">
                      <span
                        className={`text-[10px] py-0.5 px-2 rounded font-bold uppercase ${
                          item.type === 'ecs'
                            ? 'bg-primary/20 text-primary'
                            : item.type === 'ssl'
                            ? 'bg-warning/20 text-warning'
                            : 'bg-tertiary/20 text-tertiary'
                        }`}
                      >
                        {item.type}
                      </span>
                    </td>
                    <td className="py-3 px-3">
                      <div className="font-medium text-on-surface">{item.name}</div>
                      <div className="text-[10px] font-mono text-on-surface-variant mt-0.5">{item.id}</div>
                    </td>
                    <td className="py-3 px-3 text-on-surface-variant text-xs font-mono">
                      {item.type === 'ssl' ? `${item.spec} · ${item.engine}` : item.type === 'rds' ? item.engine : item.spec}
                    </td>
                    <td className="py-3 px-3 text-on-surface-variant">{item.charge_type}</td>
                    <td className="py-3 px-3 text-on-surface-variant font-mono">{item.expire_date}</td>
                    <td
                      className={`py-3 px-3 text-right font-headline font-bold text-lg ${
                        isUrgent ? 'text-error' : isWarning ? 'text-warning' : 'text-tertiary'
                      }`}
                    >
                      {item.days_left >= 0 ? `${item.days_left}` : '—'}
                      <span className="text-xs font-normal ml-1">天</span>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

function StatCard({
  icon,
  label,
  value,
  color,
  borderColor,
}: {
  icon: string
  label: string
  value: string
  color: string
  borderColor: string
}) {
  return (
    <div className={`glass-card rounded-xl p-4 border-l-4 ${borderColor}`}>
      <div className="flex items-center gap-2 mb-2">
        <span className={`material-symbols-outlined text-lg ${color}`}>{icon}</span>
        <span className="text-[10px] text-on-surface-variant uppercase font-bold tracking-wider">{label}</span>
      </div>
      <div className={`text-3xl font-headline font-bold ${color}`}>{value}</div>
    </div>
  )
}
