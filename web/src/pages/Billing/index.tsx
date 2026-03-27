import { useEffect, useState } from 'react'
import { getBilling, type BillingItem } from '../../api/client'

export default function Billing() {
  const [items, setItems] = useState<BillingItem[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<'all' | 'ecs' | 'rds' | 'ssl'>('all')

  useEffect(() => {
    getBilling()
      .then((data) => {
        // 有效资源（days_left>=0）按天数升序在前，已过期（<0）按天数降序在后
        setItems(data.sort((a, b) => {
          const aExpired = a.days_left < 0 ? 1 : 0
          const bExpired = b.days_left < 0 ? 1 : 0
          if (aExpired !== bExpired) return aExpired - bExpired
          return a.days_left - b.days_left
        }))
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
        <div className="w-8 h-8 border-2 border-[#2ca07a]/30 border-t-[#2ca07a] rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-[22px] font-semibold text-[#495057]">资源到期情况</h1>
        <p className="text-sm text-[#878a99] mt-1">阿里云 ECS / RDS / SSL 续费到期提醒</p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
        <StatCard
          icon="error"
          label="紧急（30天内）"
          value={urgent.length}
          iconBg={urgent.length > 0 ? 'bg-[#f06548]/15' : 'bg-[#0ab39c]/15'}
          iconColor={urgent.length > 0 ? 'text-[#f06548]' : 'text-[#0ab39c]'}
          valueColor={urgent.length > 0 ? 'text-[#f06548]' : 'text-[#0ab39c]'}
          bottomBorder={urgent.length > 0 ? 'border-b-2 border-[#f06548]' : ''}
        />
        <StatCard
          icon="warning"
          label="预警（60天内）"
          value={warning.length}
          iconBg={warning.length > 0 ? 'bg-[#f7b84b]/15' : 'bg-[#0ab39c]/15'}
          iconColor={warning.length > 0 ? 'text-[#f7b84b]' : 'text-[#0ab39c]'}
          valueColor={warning.length > 0 ? 'text-[#f7b84b]' : 'text-[#0ab39c]'}
          bottomBorder={warning.length > 0 ? 'border-b-2 border-[#f7b84b]' : ''}
        />
        <StatCard
          icon="dns"
          label="ECS 实例"
          value={ecsCount}
          iconBg="bg-[#2ca07a]/15"
          iconColor="text-[#2ca07a]"
          valueColor="text-[#495057]"
        />
        <StatCard
          icon="database"
          label="RDS 实例"
          value={rdsCount}
          iconBg="bg-[#0ab39c]/15"
          iconColor="text-[#0ab39c]"
          valueColor="text-[#495057]"
        />
        <StatCard
          icon="lock"
          label="SSL 证书"
          value={sslCount}
          iconBg="bg-[#f7b84b]/15"
          iconColor="text-[#f7b84b]"
          valueColor="text-[#495057]"
        />
      </div>

      {/* Urgent Alert */}
      {urgent.length > 0 && (
        <div className="bg-[#f06548]/8 border border-[#f06548]/25 rounded-[10px] p-4 flex items-start gap-3"
          style={{ backgroundColor: 'rgba(240,101,72,0.06)' }}>
          <span className="material-symbols-outlined text-[#f06548] text-[20px] mt-0.5 flex-shrink-0">notification_important</span>
          <div>
            <div className="text-sm font-semibold text-[#f06548] mb-1">
              {urgent.length} 个资源将在 30 天内到期
            </div>
            <div className="text-[12px] text-[#f06548]/80">
              {urgent.map((i) => `${i.name}（${i.days_left}天）`).join('、')}
            </div>
          </div>
        </div>
      )}

      {/* Table */}
      <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
        <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center justify-between">
          <h2 className="text-base font-semibold text-[#495057]">到期列表</h2>
          <div className="flex">
            {([
              { key: 'all', label: '全部', count: items.length },
              { key: 'ecs', label: 'ECS', count: ecsCount },
              { key: 'rds', label: 'RDS', count: rdsCount },
              { key: 'ssl', label: 'SSL', count: sslCount },
            ] as const).map((tab, idx, arr) => (
              <button
                key={tab.key}
                onClick={() => setFilter(tab.key)}
                className={`px-3 py-1.5 text-[12px] font-medium border transition-colors ${
                  idx === 0 ? 'rounded-l' : ''
                }${idx === arr.length - 1 ? 'rounded-r' : ''
                }${idx > 0 ? 'border-l-0' : ''} ${
                  filter === tab.key
                    ? 'bg-[#2ca07a] border-[#2ca07a] text-white'
                    : 'bg-white border-[#ced4da] text-[#878a99] hover:text-[#495057] hover:border-[#2ca07a]'
                }`}
              >
                {tab.label} <span className="ml-0.5 opacity-70">{tab.count}</span>
              </button>
            ))}
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-[#f8f9fa]">
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">类型</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">名称</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec] hidden md:table-cell">所属账号</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">规格 / 引擎</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">计费方式</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">到期日期</th>
                <th className="text-right text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">剩余天数</th>
              </tr>
            </thead>
            <tbody>
              {items.filter(i => filter === 'all' || i.type === filter).map((item, idx) => {
                const isUrgent = item.days_left >= 0 && item.days_left <= 30
                const isWarning = item.days_left > 30 && item.days_left <= 60
                return (
                  <tr
                    key={`${item.type}-${item.id}`}
                    className={`transition-colors ${idx < items.length - 1 ? 'border-b border-[#f2f4f7]' : ''} ${
                      isUrgent
                        ? 'bg-[#f06548]/4 hover:bg-[#f06548]/8'
                        : isWarning
                        ? 'bg-[#f7b84b]/4 hover:bg-[#f7b84b]/8'
                        : 'hover:bg-[#f8f9fa]'
                    }`}
                    style={isUrgent ? { backgroundColor: 'rgba(240,101,72,0.03)' } :
                      isWarning ? { backgroundColor: 'rgba(247,184,75,0.03)' } : {}}
                  >
                    <td className="py-3.5 px-5">
                      <span
                        className={`text-[11px] py-0.5 px-2 rounded font-semibold uppercase ${
                          item.type === 'ecs'
                            ? 'bg-[#2ca07a]/10 text-[#2ca07a]'
                            : item.type === 'ssl'
                            ? 'bg-[#f7b84b]/10 text-[#c98a1a]'
                            : 'bg-[#0ab39c]/10 text-[#0ab39c]'
                        }`}
                      >
                        {item.type}
                      </span>
                    </td>
                    <td className="py-3.5 px-5">
                      <div className="font-medium text-[#495057] text-sm">{item.name}</div>
                      <div className="text-[11px] font-mono text-[#878a99] mt-0.5">{item.id}</div>
                    </td>
                    <td className="py-3.5 px-5 hidden md:table-cell">
                      {item.account_name ? (
                        <span className="text-[12px] text-[#878a99]">{item.account_name}</span>
                      ) : (
                        <span className="text-[12px] text-[#ced4da]">—</span>
                      )}
                    </td>
                    <td className="py-3.5 px-5 text-[#878a99] text-xs font-mono">
                      {item.type === 'ssl' ? `${item.spec} · ${item.engine}` : item.type === 'rds' ? item.engine : item.spec}
                    </td>
                    <td className="py-3.5 px-5 text-[#878a99] text-sm">{item.charge_type}</td>
                    <td className="py-3.5 px-5 text-[#878a99] font-mono text-sm">{item.expire_date}</td>
                    <td
                      className={`py-3.5 px-5 text-right font-bold text-xl ${
                        item.days_left < 0 ? 'text-[#f06548]' : isUrgent ? 'text-[#f06548]' : isWarning ? 'text-[#f7b84b]' : 'text-[#0ab39c]'
                      }`}
                    >
                      {item.days_left >= 0 ? `${item.days_left}` : item.days_left > -9999 ? `${item.days_left}` : '—'}
                      <span className="text-xs font-normal ml-1 text-[#878a99]">{item.days_left > -9999 ? '天' : ''}</span>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>

          {items.filter(i => filter === 'all' || i.type === filter).length === 0 && (
            <div className="py-12 text-center">
              <span className="material-symbols-outlined text-3xl text-[#ced4da] mb-2 block">receipt_long</span>
              <p className="text-[#878a99] text-sm">暂无到期记录</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function StatCard({
  icon,
  label,
  value,
  iconBg,
  iconColor,
  valueColor,
  bottomBorder = '',
}: {
  icon: string
  label: string
  value: number
  iconBg: string
  iconColor: string
  valueColor: string
  bottomBorder?: string
}) {
  return (
    <div className={`bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5 ${bottomBorder}`}>
      <div className="flex items-center gap-3">
        <div className={`w-12 h-12 rounded-full ${iconBg} flex items-center justify-center flex-shrink-0`}>
          <span className={`material-symbols-outlined text-[20px] ${iconColor}`}>{icon}</span>
        </div>
        <div>
          <div className={`text-2xl font-bold ${valueColor}`}>{value}</div>
          <div className="text-[11px] text-[#878a99] mt-0.5">{label}</div>
        </div>
      </div>
    </div>
  )
}
