import { create } from 'zustand'
import api from '../api/client'

interface AuthState {
  token: string | null
  username: string | null
  role: string | null
  displayName: string | null
  mustChangePwd: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  checkAuth: () => Promise<boolean>
  changePassword: (oldPwd: string, newPwd: string) => Promise<void>
}

export const useAuthStore = create<AuthState>((set, get) => ({
  token: localStorage.getItem('token'),
  username: localStorage.getItem('username'),
  role: localStorage.getItem('role') || null,
  displayName: localStorage.getItem('displayName'),
  mustChangePwd: localStorage.getItem('mustChangePwd') === 'true',

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
    localStorage.setItem('role', data.role)
    localStorage.setItem('displayName', data.display_name || '')
    localStorage.setItem('mustChangePwd', String(data.must_change_pwd))
    set({
      token: data.token,
      username: data.username,
      role: data.role,
      displayName: data.display_name || '',
      mustChangePwd: !!data.must_change_pwd,
    })
  },

  logout: () => {
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    localStorage.removeItem('role')
    localStorage.removeItem('displayName')
    localStorage.removeItem('mustChangePwd')
    set({ token: null, username: null, role: null, displayName: null, mustChangePwd: false })
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
      const data = await res.json()
      set({
        role: data.role,
        displayName: data.display_name || '',
        mustChangePwd: !!data.must_change_pwd,
      })
      localStorage.setItem('role', data.role)
      localStorage.setItem('displayName', data.display_name || '')
      localStorage.setItem('mustChangePwd', String(data.must_change_pwd))
      return true
    } catch {
      return false
    }
  },

  changePassword: async (oldPwd: string, newPwd: string) => {
    const { data } = await api.put('/auth/password', {
      old_password: oldPwd,
      new_password: newPwd,
    })
    localStorage.setItem('token', data.token)
    localStorage.setItem('role', data.role)
    localStorage.setItem('displayName', data.display_name || '')
    localStorage.setItem('mustChangePwd', 'false')
    set({ token: data.token, role: data.role, displayName: data.display_name || '', mustChangePwd: false })
  },
}))
