import axios from 'axios'
import type { DashboardData, Server } from '../types'

const api = axios.create({ baseURL: '/api/v1' })

export async function getDashboard(): Promise<DashboardData> {
  const { data } = await api.get('/dashboard')
  return data
}

export async function getServers(): Promise<Server[]> {
  const { data } = await api.get('/servers')
  return data
}

export async function getServer(id: string): Promise<Server> {
  const { data } = await api.get(`/servers/${id}`)
  return data
}

export default api
