import { create } from 'zustand'
import type { Server, ServerGroup, MetricsPayload } from '../types'
import { getDashboard } from '../api/client'

interface ServerState {
  servers: Server[]
  groups: ServerGroup[]
  metrics: Record<string, MetricsPayload>
  loading: boolean
  fetchDashboard: () => Promise<void>
  updateMetrics: (hostId: string, data: MetricsPayload) => void
}

export const useServerStore = create<ServerState>((set) => ({
  servers: [],
  groups: [],
  metrics: {},
  loading: true,
  fetchDashboard: async () => {
    set({ loading: true })
    try {
      const data = await getDashboard()
      const update: Partial<ServerState> = { servers: data.servers || [], loading: false }
      // 预填充缓存的指标快照（阿里云服务器无需等 WebSocket）
      if (data.metrics) {
        update.metrics = { ...data.metrics }
      }
      if (data.groups) {
        update.groups = data.groups
      }
      set(update)
    } catch {
      set({ loading: false })
    }
  },
  updateMetrics: (hostId, data) =>
    set((state) => ({
      metrics: { ...state.metrics, [hostId]: data },
    })),
}))
