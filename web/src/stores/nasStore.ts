import { create } from 'zustand'
import { getNasDevices, getNasMetrics, type NasDevice, type NasMetrics } from '../api/nas'

interface NasState {
  devices: NasDevice[]
  metrics: Record<number, NasMetrics>
  loading: boolean
  fetchDevices: () => Promise<void>
  updateMetrics: (nasId: number, data: NasMetrics) => void
  updateStatus: (nasId: number, status: string) => void
}

export const useNasStore = create<NasState>((set) => ({
  devices: [],
  metrics: {},
  loading: false,
  fetchDevices: async () => {
    set({ loading: true })
    try {
      const devices = await getNasDevices()
      set({ devices: devices || [] })
      // First-screen: batch load cached metrics for all devices
      const metricsMap: Record<number, NasMetrics> = {}
      await Promise.all(
        (devices || []).map(async (d) => {
          try {
            const m = await getNasMetrics(d.id)
            if (m && m.timestamp) metricsMap[d.id] = m
          } catch { /* device may have no metrics yet */ }
        })
      )
      set({ metrics: metricsMap })
    } finally {
      set({ loading: false })
    }
  },
  updateMetrics: (nasId, data) =>
    set((state) => ({
      metrics: { ...state.metrics, [nasId]: data },
    })),
  updateStatus: (nasId, status) =>
    set((state) => ({
      devices: state.devices.map((d) =>
        d.id === nasId ? { ...d, status: status as NasDevice['status'] } : d
      ),
    })),
}))
