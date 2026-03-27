import { create } from 'zustand'

interface SettingsState {
  platformName: string
  platformSubtitle: string
  logoUrl: string
  setPlatformName: (name: string) => void
  setPlatformSubtitle: (subtitle: string) => void
  setLogoUrl: (url: string) => void
}

const STORAGE_KEY = 'opsboard-settings'

function loadSettings() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) return JSON.parse(raw)
  } catch { /* ignore */ }
  return {}
}

function saveSettings(state: Partial<SettingsState>) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({
    platformName: state.platformName,
    platformSubtitle: state.platformSubtitle,
    logoUrl: state.logoUrl,
  }))
}

const saved = loadSettings()

export const useSettingsStore = create<SettingsState>((set) => ({
  platformName: saved.platformName ?? 'OpsBoard',
  platformSubtitle: saved.platformSubtitle ?? '运维监控管理平台',
  logoUrl: saved.logoUrl ?? '/logo.svg',
  setPlatformName: (name) => set((s) => {
    const next = { ...s, platformName: name }
    saveSettings(next)
    return next
  }),
  setPlatformSubtitle: (subtitle) => set((s) => {
    const next = { ...s, platformSubtitle: subtitle }
    saveSettings(next)
    return next
  }),
  setLogoUrl: (url) => set((s) => {
    const next = { ...s, logoUrl: url }
    saveSettings(next)
    return next
  }),
}))
