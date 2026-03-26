import { create } from 'zustand'
import type { AlertEvent, AlertStats } from '../types'
import { getAlertEvents, getAlertStats } from '../api/alert'

interface AlertState {
  firingEvents: AlertEvent[]
  firingCount: number
  stats: AlertStats | null
  fetchFiringEvents: () => Promise<void>
  fetchStats: () => Promise<void>
  addEvent: (event: AlertEvent) => void
  resolveEvent: (eventId: number) => void
  silenceEvent: (eventId: number, ackedBy: string) => void
}

export const useAlertStore = create<AlertState>((set) => ({
  firingEvents: [],
  firingCount: 0,
  stats: null,

  fetchFiringEvents: async () => {
    const events = await getAlertEvents({ status: 'firing', limit: 50 })
    set({ firingEvents: events, firingCount: events.length })
  },

  fetchStats: async () => {
    const stats = await getAlertStats()
    set({ stats, firingCount: stats.firing })
  },

  addEvent: (event) =>
    set((state) => ({
      firingEvents: [event, ...state.firingEvents],
      firingCount: state.firingCount + 1,
    })),

  resolveEvent: (eventId) =>
    set((state) => ({
      firingEvents: state.firingEvents.filter((e) => e.id !== eventId),
      firingCount: Math.max(0, state.firingCount - 1),
    })),

  silenceEvent: (eventId, ackedBy) =>
    set((state) => ({
      firingEvents: state.firingEvents.map((e) =>
        e.id === eventId ? { ...e, silenced: true, acked_by: ackedBy } : e
      ),
    })),
}))
