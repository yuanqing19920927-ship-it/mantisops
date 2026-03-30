import { create } from 'zustand'
import {
  listDevices,
  listSubnets,
  getTopology,
  getScanStatus,
  type NetworkDevice,
  type NetworkSubnet,
  type TopologyData,
  type ScanStatus,
} from '../api/network'

interface NetworkState {
  devices: NetworkDevice[]
  subnets: NetworkSubnet[]
  topology: TopologyData | null
  scanStatus: ScanStatus | null
  loading: boolean
  activeTab: number
  fetchDevices: (subnetId?: number) => Promise<void>
  fetchSubnets: () => Promise<void>
  fetchTopology: () => Promise<void>
  fetchScanStatus: () => Promise<void>
  updateDeviceInList: (id: number, changes: Partial<NetworkDevice>) => void
  removeDeviceFromList: (id: number) => void
  setActiveTab: (tab: number) => void
  setScanStatus: (status: ScanStatus | null) => void
}

export const useNetworkStore = create<NetworkState>((set) => ({
  devices: [],
  subnets: [],
  topology: null,
  scanStatus: null,
  loading: false,
  activeTab: 0,

  fetchDevices: async (subnetId?: number) => {
    set({ loading: true })
    try {
      const devices = await listDevices(subnetId)
      set({ devices: devices || [] })
    } finally {
      set({ loading: false })
    }
  },

  fetchSubnets: async () => {
    try {
      const subnets = await listSubnets()
      set({ subnets: subnets || [] })
    } catch {
      // ignore
    }
  },

  fetchTopology: async () => {
    try {
      const topology = await getTopology()
      set({ topology })
    } catch {
      // ignore
    }
  },

  fetchScanStatus: async () => {
    try {
      const scanStatus = await getScanStatus()
      set({ scanStatus })
    } catch {
      // ignore
    }
  },

  updateDeviceInList: (id, changes) =>
    set((state) => ({
      devices: state.devices.map((d) =>
        d.id === id ? { ...d, ...changes } : d
      ),
    })),

  removeDeviceFromList: (id) =>
    set((state) => ({
      devices: state.devices.filter((d) => d.id !== id),
    })),

  setActiveTab: (tab) => set({ activeTab: tab }),

  setScanStatus: (scanStatus) => set({ scanStatus }),
}))
