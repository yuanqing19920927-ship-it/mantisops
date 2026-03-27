import { create } from 'zustand'

interface ThemeState {
  theme: 'light' | 'dark'
  toggle: () => void
}

function applyTheme(theme: 'light' | 'dark') {
  const root = document.documentElement
  if (theme === 'dark') {
    root.classList.add('dark')
  } else {
    root.classList.remove('dark')
  }
}

const initial = (localStorage.getItem('theme') as 'light' | 'dark') || 'light'
applyTheme(initial)

export const useThemeStore = create<ThemeState>((set) => ({
  theme: initial,
  toggle: () =>
    set((state) => {
      const next = state.theme === 'dark' ? 'light' : 'dark'
      localStorage.setItem('theme', next)
      applyTheme(next)
      return { theme: next }
    }),
}))
