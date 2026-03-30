import type { NetworkSubnet } from '../../api/network'
import { useNetworkStore } from '../../stores/networkStore'

function formatTime(ts: string | null): string {
  if (!ts) return '从未'
  const d = new Date(ts)
  const now = new Date()
  const diff = Math.floor((now.getTime() - d.getTime()) / 1000)
  if (diff < 60) return `${diff} 秒前`
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  return `${Math.floor(diff / 86400)} 天前`
}

interface SubnetCardProps {
  subnet: NetworkSubnet
  onClick: () => void
}

function SubnetCard({ subnet, onClick }: SubnetCardProps) {
  const rate =
    subnet.total_hosts > 0
      ? Math.round((subnet.alive_hosts / subnet.total_hosts) * 100)
      : 0

  const barColor =
    rate > 80
      ? 'bg-[#2ca07a]'
      : rate > 50
      ? 'bg-[#f59e0b]'
      : 'bg-[#ef4444]'

  const rateTextColor =
    rate > 80
      ? 'text-[#2ca07a]'
      : rate > 50
      ? 'text-[#f59e0b]'
      : 'text-[#ef4444]'

  return (
    <button
      onClick={onClick}
      className="glass-card p-4 rounded-xl text-left hover:border-[#2ca07a]/40 transition-all duration-200 group w-full"
    >
      {/* CIDR + gateway */}
      <div className="flex items-start justify-between mb-3">
        <div>
          <p className="text-sm font-bold text-on-surface font-mono">{subnet.cidr}</p>
          {subnet.name && (
            <p className="text-xs text-on-surface-variant mt-0.5">{subnet.name}</p>
          )}
          {subnet.gateway && (
            <p className="text-xs text-on-surface-variant">
              网关：<span className="font-mono">{subnet.gateway}</span>
            </p>
          )}
        </div>
        <span className="material-symbols-outlined text-[#adb5bd] group-hover:text-[#2ca07a] transition-colors text-xl">
          arrow_forward
        </span>
      </div>

      {/* Counts */}
      <div className="flex items-baseline gap-1 mb-2">
        <span className="text-xl font-bold text-on-surface">
          {subnet.alive_hosts}
        </span>
        <span className="text-xs text-on-surface-variant">
          / {subnet.total_hosts} 台在线
        </span>
        <span className={`text-xs font-semibold ml-auto ${rateTextColor}`}>
          {rate}%
        </span>
      </div>

      {/* Progress bar */}
      <div className="h-1.5 w-full rounded-full bg-[rgba(255,255,255,0.08)] overflow-hidden mb-3">
        <div
          className={`h-full rounded-full transition-all ${barColor}`}
          style={{ width: `${rate}%` }}
        />
      </div>

      {/* Last scan */}
      <p className="text-[11px] text-on-surface-variant">
        上次扫描：{formatTime(subnet.last_scan)}
      </p>
    </button>
  )
}

export default function SubnetOverview() {
  const subnets = useNetworkStore((s) => s.subnets)
  const setActiveTab = useNetworkStore((s) => s.setActiveTab)

  if (subnets.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-48 text-[#6c757d]">
        <span className="material-symbols-outlined text-4xl mb-2 text-[#adb5bd]">
          lan
        </span>
        <p className="text-sm">暂无网段数据</p>
        <p className="text-xs mt-1 text-[#adb5bd]">执行扫描后将显示网段信息</p>
      </div>
    )
  }

  function handleCardClick(_subnet: NetworkSubnet) {
    // Switch to device list tab (tab index 1)
    setActiveTab(1)
  }

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
      {subnets.map((subnet) => (
        <SubnetCard
          key={subnet.id}
          subnet={subnet}
          onClick={() => handleCardClick(subnet)}
        />
      ))}
    </div>
  )
}
