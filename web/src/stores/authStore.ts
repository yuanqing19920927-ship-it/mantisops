import { create } from 'zustand'

interface AuthState {
  token: string | null
  username: string | null
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  checkAuth: () => Promise<boolean>
}

export const useAuthStore = create<AuthState>((set, get) => ({
  token: localStorage.getItem('token'),
  username: localStorage.getItem('username'),

  login: async (username: string, password: string) => {
    const res = await fetch('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      throw new Error(data.error || '登录失败')
    }
    const data = await res.json()
    localStorage.setItem('token', data.token)
    localStorage.setItem('username', data.username)
    set({ token: data.token, username: data.username })
  },

  logout: () => {
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    set({ token: null, username: null })
  },

  checkAuth: async () => {
    const token = get().token
    if (!token) return false
    try {
      const res = await fetch('/api/v1/auth/me', {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) {
        get().logout()
        return false
      }
      return true
    } catch {
      return false
    }
  },
}))
