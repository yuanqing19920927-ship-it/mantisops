import { create } from 'zustand'
import type { Server, MetricsPayload } from '../types'
import { getDashboard } from '../api/client'

interface ServerState {
  servers: Server[]
  metrics: Record<string, MetricsPayload>
  loading: boolean
  fetchDashboard: () => Promise<void>
  updateMetrics: (hostId: string, data: MetricsPayload) => void
}

export const useServerStore = create<ServerState>((set) => ({
  servers: [],
  metrics: {},
  loading: true,
  fetchDashboard: async () => {
    set({ loading: true })
    try {
      const data = await getDashboard()
      set({ servers: data.servers || [], loading: false })
    } catch {
      set({ loading: false })
    }
  },
  updateMetrics: (hostId, data) =>
    set((state) => ({
      metrics: { ...state.metrics, [hostId]: data },
    })),
}))
