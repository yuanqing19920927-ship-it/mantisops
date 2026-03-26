import { create } from 'zustand'

interface ThemeState {
  theme: 'light' | 'dark'
  toggle: () => void
}

export const useThemeStore = create<ThemeState>((set) => ({
  theme: (localStorage.getItem('theme') as 'light' | 'dark') || 'dark',
  toggle: () =>
    set((state) => {
      const next = state.theme === 'dark' ? 'light' : 'dark'
      localStorage.setItem('theme', next)
      return { theme: next }
    }),
}))
