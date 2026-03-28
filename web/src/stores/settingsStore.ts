import { create } from 'zustand'
import api from '../api/client'

interface SettingsState {
  platformName: string
  platformSubtitle: string
  logoUrl: string
  loaded: boolean
  setPlatformName: (name: string) => void
  setPlatformSubtitle: (subtitle: string) => void
  setLogoUrl: (url: string) => void
  fetchSettings: () => Promise<void>
  saveSettings: (name: string, subtitle: string, logo: string) => Promise<void>
}

const STORAGE_KEY = 'mantisops-settings'

function loadCache() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) return JSON.parse(raw)
  } catch { /* ignore */ }
  return {}
}

function saveCache(state: { platformName: string; platformSubtitle: string; logoUrl: string }) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({
    platformName: state.platformName,
    platformSubtitle: state.platformSubtitle,
    logoUrl: state.logoUrl,
  }))
}

function syncTabMeta(name: string, logoUrl: string) {
  document.title = name
  const link = document.querySelector<HTMLLinkElement>("link[rel='icon']")
  if (link) link.href = logoUrl
}

// Load cached values for instant display before API responds
const cached = loadCache()
syncTabMeta(cached.platformName ?? 'MantisOps', cached.logoUrl ?? '/logo.svg')

export const useSettingsStore = create<SettingsState>((set) => ({
  platformName: cached.platformName ?? 'MantisOps',
  platformSubtitle: cached.platformSubtitle ?? '运维监控管理平台',
  logoUrl: cached.logoUrl ?? '/logo.svg',
  loaded: false,

  setPlatformName: (name) => set((s) => {
    const next = { ...s, platformName: name }
    saveCache(next)
    syncTabMeta(name, s.logoUrl)
    return next
  }),
  setPlatformSubtitle: (subtitle) => set((s) => {
    const next = { ...s, platformSubtitle: subtitle }
    saveCache(next)
    return next
  }),
  setLogoUrl: (url) => set((s) => {
    const next = { ...s, logoUrl: url }
    saveCache(next)
    syncTabMeta(s.platformName, url)
    return next
  }),

  fetchSettings: async () => {
    try {
      // Use plain fetch — this endpoint is public (no JWT required)
      const res = await fetch('/api/v1/settings')
      if (!res.ok) throw new Error('fetch settings failed')
      const data = await res.json()
      const name = data.platform_name || 'MantisOps'
      const subtitle = data.platform_subtitle || '运维监控管理平台'
      const logo = data.logo_url || '/logo.svg'
      set({ platformName: name, platformSubtitle: subtitle, logoUrl: logo, loaded: true })
      saveCache({ platformName: name, platformSubtitle: subtitle, logoUrl: logo })
      syncTabMeta(name, logo)
    } catch {
      // API not available, use cached values
      set({ loaded: true })
    }
  },

  saveSettings: async (name: string, subtitle: string, logo: string) => {
    const finalName = name.trim() || 'MantisOps'
    const finalSubtitle = subtitle.trim() || '运维监控管理平台'
    const finalLogo = logo.trim() || '/logo.svg'
    await api.put('/settings', {
      platform_name: finalName,
      platform_subtitle: finalSubtitle,
      logo_url: finalLogo,
    })
    set({ platformName: finalName, platformSubtitle: finalSubtitle, logoUrl: finalLogo })
    saveCache({ platformName: finalName, platformSubtitle: finalSubtitle, logoUrl: finalLogo })
    syncTabMeta(finalName, finalLogo)
  },
}))
