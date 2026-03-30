import { useState } from 'react'
import { useNetworkStore } from '../../stores/networkStore'
import { useAuthStore } from '../../stores/authStore'
import { updateDevice, deleteDevice } from '../../api/network'
import type { NetworkDevice } from '../../api/network'

const DEVICE_TYPES = [
  { value: 'switch', label: '交换机' },
  { value: 'router', label: '路由器' },
  { value: 'ap', label: '无线 AP' },
  { value: 'firewall', label: '防火墙' },
  { value: 'printer', label: '打印机' },
  { value: 'server', label: '服务器' },
  { value: 'unknown', label: '未知' },
]

const DEVICE_TYPE_MAP: Record<string, string> = Object.fromEntries(
  DEVICE_TYPES.map((t) => [t.value, t.label])
)

function typeLabel(t: string): string {
  return DEVICE_TYPE_MAP[t] ?? t
}

function formatTime(ts: string | null): string {
  if (!ts) return '—'
  const d = new Date(ts)
  const now = new Date()
  const diff = Math.floor((now.getTime() - d.getTime()) / 1000)
  if (diff < 60) return `${diff}s 前`
  if (diff < 3600) return `${Math.floor(diff / 60)}m 前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h 前`
  return d.toLocaleDateString('zh-CN')
}

function StatusDot({ status }: { status: string }) {
  const isOnline = status === 'online' || status === 'up'
  return (
    <span
      className={`inline-block w-2 h-2 rounded-full ${
        isOnline ? 'bg-[#2ca07a]' : 'bg-[#6c757d]'
      }`}
    />
  )
}

interface DeviceRowProps {
  device: NetworkDevice
  isAdmin: boolean
  subnetCidr: string
  onTypeChange: (id: number, type: string) => void
  onDelete: (id: number) => void
}

function DeviceRow({ device, isAdmin, subnetCidr, onTypeChange, onDelete }: DeviceRowProps) {
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)

  async function handleTypeChange(newType: string) {
    setSaving(true)
    try {
      await updateDevice(device.id, { device_type: newType })
      onTypeChange(device.id, newType)
    } catch {
      // ignore
    } finally {
      setSaving(false)
      setEditing(false)
    }
  }

  async function handleDelete() {
    if (!confirm(`确认删除设备 ${device.ip}？`)) return
    try {
      await deleteDevice(device.id)
      onDelete(device.id)
    } catch {
      // ignore
    }
  }

  return (
    <tr className="border-b border-[rgba(255,255,255,0.06)] hover:bg-[rgba(255,255,255,0.03)] transition-colors">
      <td className="px-4 py-3">
        <StatusDot status={device.status} />
      </td>
      <td className="px-4 py-3 font-mono text-sm text-on-surface">{device.ip}</td>
      <td className="px-4 py-3 font-mono text-xs text-on-surface-variant">
        {device.mac || '—'}
      </td>
      <td className="px-4 py-3 text-sm text-on-surface-variant">
        {device.vendor || '—'}
      </td>
      <td className="px-4 py-3 text-sm">
        {isAdmin ? (
          editing ? (
            <select
              className="bg-[rgba(255,255,255,0.05)] border border-[rgba(255,255,255,0.15)] rounded px-2 py-0.5 text-xs text-on-surface focus:outline-none focus:border-[#2ca07a]"
              defaultValue={device.device_type || 'unknown'}
              disabled={saving}
              autoFocus
              onChange={(e) => handleTypeChange(e.target.value)}
              onBlur={() => setEditing(false)}
            >
              {DEVICE_TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          ) : (
            <button
              className="text-xs text-on-surface-variant hover:text-[#2ca07a] transition-colors flex items-center gap-1 group"
              onClick={() => setEditing(true)}
            >
              {typeLabel(device.device_type || 'unknown')}
              <span className="material-symbols-outlined text-[14px] opacity-0 group-hover:opacity-100 transition-opacity">
                edit
              </span>
            </button>
          )
        ) : (
          <span className="text-xs text-on-surface-variant">
            {typeLabel(device.device_type || 'unknown')}
          </span>
        )}
      </td>
      <td className="px-4 py-3 text-xs text-on-surface-variant">
        {device.model || '—'}
      </td>
      <td className="px-4 py-3 text-center">
        {device.snmp_supported ? (
          <span className="material-symbols-outlined text-[#2ca07a] text-base">check_circle</span>
        ) : (
          <span className="material-symbols-outlined text-[#6c757d] text-base">cancel</span>
        )}
      </td>
      <td className="px-4 py-3 text-xs font-mono text-on-surface-variant">
        {subnetCidr || '—'}
      </td>
      <td className="px-4 py-3 text-xs text-on-surface-variant">
        {formatTime(device.last_seen)}
      </td>
      {isAdmin && (
        <td className="px-4 py-3">
          <button
            onClick={handleDelete}
            className="text-[#6c757d] hover:text-[#ef4444] transition-colors"
            title="删除设备"
          >
            <span className="material-symbols-outlined text-base">delete</span>
          </button>
        </td>
      )}
    </tr>
  )
}

export default function DeviceList() {
  const devices = useNetworkStore((s) => s.devices)
  const subnets = useNetworkStore((s) => s.subnets)
  const loading = useNetworkStore((s) => s.loading)
  const updateDeviceInList = useNetworkStore((s) => s.updateDeviceInList)
  const removeDeviceFromList = useNetworkStore((s) => s.removeDeviceFromList)
  const role = useAuthStore((s) => s.role)
  const isAdmin = role === 'admin'

  const [filterSubnet, setFilterSubnet] = useState<string>('all')
  const [filterType, setFilterType] = useState<string>('all')
  const [filterStatus, setFilterStatus] = useState<string>('all')
  const [search, setSearch] = useState('')

  const subnetMap: Record<number, string> = {}
  subnets.forEach((s) => { subnetMap[s.id] = s.cidr })

  const filtered = devices.filter((d) => {
    if (filterSubnet !== 'all' && String(d.subnet_id) !== filterSubnet) return false
    if (filterType !== 'all' && (d.device_type || 'unknown') !== filterType) return false
    if (filterStatus !== 'all') {
      const isOnline = d.status === 'online' || d.status === 'up'
      if (filterStatus === 'online' && !isOnline) return false
      if (filterStatus === 'offline' && isOnline) return false
    }
    if (search) {
      const q = search.toLowerCase()
      return (
        d.ip.toLowerCase().includes(q) ||
        d.mac.toLowerCase().includes(q) ||
        d.vendor.toLowerCase().includes(q) ||
        d.model.toLowerCase().includes(q) ||
        d.hostname.toLowerCase().includes(q)
      )
    }
    return true
  })

  if (loading) {
    return (
      <div className="flex items-center justify-center h-40 text-on-surface-variant">
        <span className="material-symbols-outlined text-3xl animate-spin mr-2">
          progress_activity
        </span>
        加载中...
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Filters */}
      <div className="flex flex-wrap gap-3 items-center">
        {/* Search */}
        <div className="flex items-center gap-2 bg-[rgba(255,255,255,0.05)] border border-[rgba(255,255,255,0.1)] rounded-lg px-3 py-1.5 flex-1 min-w-[200px] max-w-xs">
          <span className="material-symbols-outlined text-[#6c757d] text-base">search</span>
          <input
            type="text"
            placeholder="搜索 IP / MAC / 厂商 / 型号"
            className="bg-transparent text-sm text-on-surface placeholder-[#6c757d] flex-1 focus:outline-none"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          {search && (
            <button onClick={() => setSearch('')} className="text-[#6c757d] hover:text-on-surface">
              <span className="material-symbols-outlined text-base">close</span>
            </button>
          )}
        </div>

        {/* Subnet filter */}
        <select
          className="bg-[rgba(255,255,255,0.05)] border border-[rgba(255,255,255,0.1)] rounded-lg px-3 py-1.5 text-sm text-on-surface focus:outline-none focus:border-[#2ca07a]"
          value={filterSubnet}
          onChange={(e) => setFilterSubnet(e.target.value)}
        >
          <option value="all">全部网段</option>
          {subnets.map((s) => (
            <option key={s.id} value={String(s.id)}>
              {s.cidr}
            </option>
          ))}
        </select>

        {/* Type filter */}
        <select
          className="bg-[rgba(255,255,255,0.05)] border border-[rgba(255,255,255,0.1)] rounded-lg px-3 py-1.5 text-sm text-on-surface focus:outline-none focus:border-[#2ca07a]"
          value={filterType}
          onChange={(e) => setFilterType(e.target.value)}
        >
          <option value="all">全部类型</option>
          {DEVICE_TYPES.map((t) => (
            <option key={t.value} value={t.value}>
              {t.label}
            </option>
          ))}
        </select>

        {/* Status filter */}
        <select
          className="bg-[rgba(255,255,255,0.05)] border border-[rgba(255,255,255,0.1)] rounded-lg px-3 py-1.5 text-sm text-on-surface focus:outline-none focus:border-[#2ca07a]"
          value={filterStatus}
          onChange={(e) => setFilterStatus(e.target.value)}
        >
          <option value="all">全部状态</option>
          <option value="online">在线</option>
          <option value="offline">离线</option>
        </select>

        <span className="text-xs text-on-surface-variant ml-auto">
          共 {filtered.length} 台
          {filtered.length !== devices.length && ` / ${devices.length}`}
        </span>
      </div>

      {/* Table */}
      {filtered.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-40 text-[#6c757d]">
          <span className="material-symbols-outlined text-4xl mb-2 text-[#adb5bd]">
            devices_other
          </span>
          <p className="text-sm">暂无设备数据</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-[rgba(255,255,255,0.08)]">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-[rgba(255,255,255,0.08)] bg-[rgba(255,255,255,0.03)]">
                <th className="px-4 py-3 text-xs font-semibold text-on-surface-variant w-8">状态</th>
                <th className="px-4 py-3 text-xs font-semibold text-on-surface-variant">IP 地址</th>
                <th className="px-4 py-3 text-xs font-semibold text-on-surface-variant">MAC</th>
                <th className="px-4 py-3 text-xs font-semibold text-on-surface-variant">厂商</th>
                <th className="px-4 py-3 text-xs font-semibold text-on-surface-variant">类型</th>
                <th className="px-4 py-3 text-xs font-semibold text-on-surface-variant">型号</th>
                <th className="px-4 py-3 text-xs font-semibold text-on-surface-variant text-center">SNMP</th>
                <th className="px-4 py-3 text-xs font-semibold text-on-surface-variant">网段</th>
                <th className="px-4 py-3 text-xs font-semibold text-on-surface-variant">最后在线</th>
                {isAdmin && <th className="px-4 py-3 w-8" />}
              </tr>
            </thead>
            <tbody>
              {filtered.map((device) => (
                <DeviceRow
                  key={device.id}
                  device={device}
                  isAdmin={isAdmin}
                  subnetCidr={device.subnet_id ? (subnetMap[device.subnet_id] ?? '') : ''}
                  onTypeChange={(id, type) =>
                    updateDeviceInList(id, { device_type: type })
                  }
                  onDelete={removeDeviceFromList}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
