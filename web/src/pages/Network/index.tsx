import { useEffect, useState } from 'react'
import { useNetworkStore } from '../../stores/networkStore'
import { useAuthStore } from '../../stores/authStore'
import DeviceList from './DeviceList'
import SubnetOverview from './SubnetOverview'
import TopologyGraph from './TopologyGraph'
import ScanDialog from './ScanDialog'

const TABS = [
  { label: '拓扑图', icon: 'device_hub' },
  { label: '设备列表', icon: 'devices_other' },
  { label: '网段概览', icon: 'lan' },
]

export default function Network() {
  const activeTab = useNetworkStore((s) => s.activeTab)
  const setActiveTab = useNetworkStore((s) => s.setActiveTab)
  const fetchDevices = useNetworkStore((s) => s.fetchDevices)
  const fetchSubnets = useNetworkStore((s) => s.fetchSubnets)
  const fetchTopology = useNetworkStore((s) => s.fetchTopology)
  const scanStatus = useNetworkStore((s) => s.scanStatus)
  const fetchScanStatus = useNetworkStore((s) => s.fetchScanStatus)

  const role = useAuthStore((s) => s.role)
  const isAdmin = role === 'admin'

  const [scanOpen, setScanOpen] = useState(false)

  useEffect(() => {
    fetchSubnets()
    fetchDevices()
    fetchTopology()
    fetchScanStatus()
  }, [fetchSubnets, fetchDevices, fetchTopology, fetchScanStatus])

  const isScanning =
    scanStatus?.status === 'running' || scanStatus?.status === 'scanning'

  return (
    <div className="p-4 md:p-6 space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between flex-wrap gap-3">
        <div>
          <h1 className="text-xl font-bold text-on-surface">网络拓扑</h1>
          <p className="text-xs text-on-surface-variant mt-0.5">
            管理局域网设备、网段与拓扑关系
          </p>
        </div>

        {isAdmin && (
          <button
            onClick={() => setScanOpen(true)}
            className={`flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
              isScanning
                ? 'bg-[rgba(44,160,122,0.15)] text-[#2ca07a] border border-[#2ca07a]/30 cursor-default'
                : 'bg-[#2ca07a] text-white hover:bg-[#259068]'
            }`}
          >
            {isScanning ? (
              <>
                <span className="material-symbols-outlined text-base animate-spin">
                  progress_activity
                </span>
                扫描中 {Math.round(scanStatus?.progress ?? 0)}%
              </>
            ) : (
              <>
                <span className="material-symbols-outlined text-base">radar</span>
                扫描网络
              </>
            )}
          </button>
        )}
      </div>

      {/* Tabs */}
      <div className="glass-card rounded-xl overflow-hidden">
        {/* Tab bar */}
        <div className="flex border-b border-[rgba(255,255,255,0.08)]">
          {TABS.map((tab, idx) => (
            <button
              key={idx}
              onClick={() => setActiveTab(idx)}
              className={`flex items-center gap-2 px-5 py-3.5 text-sm font-medium transition-colors border-b-2 -mb-px ${
                activeTab === idx
                  ? 'text-[#2ca07a] border-[#2ca07a]'
                  : 'text-on-surface-variant border-transparent hover:text-on-surface'
              }`}
            >
              <span className="material-symbols-outlined text-base">{tab.icon}</span>
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab content */}
        <div className="p-5">
          {activeTab === 0 && <TopologyGraph />}
          {activeTab === 1 && <DeviceList />}
          {activeTab === 2 && <SubnetOverview />}
        </div>
      </div>

      {/* Scan dialog */}
      <ScanDialog open={scanOpen} onClose={() => setScanOpen(false)} />
    </div>
  )
}
