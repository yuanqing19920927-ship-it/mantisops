import api from './client'

export interface UserInfo {
  id: number
  username: string
  display_name: string
  role: string
  enabled: boolean
  must_change_pwd: boolean
  created_at: string
}

export interface PermissionItem {
  res_type: string
  res_id: string
}

export const getUsers = () => api.get('/users').then(r => r.data || [])
export const getUser = (id: number) => api.get(`/users/${id}`).then(r => r.data)
export const createUser = (body: { username: string; password: string; display_name: string; role: string }) =>
  api.post('/users', body).then(r => r.data)
export const updateUser = (id: number, body: { display_name: string; role: string; enabled: boolean }) =>
  api.put(`/users/${id}`, body).then(r => r.data)
export const deleteUser = (id: number) => api.delete(`/users/${id}`)
export const resetPassword = (id: number, password: string) =>
  api.put(`/users/${id}/reset-pwd`, { password })
export const getUserPermissions = (id: number) =>
  api.get(`/users/${id}/permissions`).then(r => r.data?.permissions || [])
export const setUserPermissions = (id: number, permissions: PermissionItem[]) =>
  api.put(`/users/${id}/permissions`, { permissions })
