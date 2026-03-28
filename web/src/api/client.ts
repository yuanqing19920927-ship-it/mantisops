import axios from 'axios'
import type { DashboardData, Server, ServerGroup, ProbeResult } from '../types'

const api = axios.create({ baseURL: '/api/v1' })

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('username')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

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

export async function updateServerName(id: string, displayName: string): Promise<void> {
  await api.put(`/servers/${id}/name`, { display_name: displayName })
}

// Probes
export interface ProbeRule {
  id?: number
  server_id: number | null
  name: string
  host: string
  port: number
  protocol: 'tcp' | 'http' | 'https'
  url: string
  expect_status: number
  expect_body: string
  interval_sec: number
  timeout_sec: number
  enabled: boolean
  source?: 'manual' | 'scan'
}

export async function getProbes(): Promise<ProbeRule[]> {
  const { data } = await api.get('/probes')
  return data || []
}

export async function createProbe(rule: ProbeRule): Promise<ProbeRule> {
  const { data } = await api.post('/probes', rule)
  return data
}

export async function updateProbe(id: number, rule: ProbeRule): Promise<ProbeRule> {
  const { data } = await api.put(`/probes/${id}`, rule)
  return data
}

export async function deleteProbe(id: number): Promise<void> {
  await api.delete(`/probes/${id}`)
}

export async function getProbeStatus(): Promise<ProbeResult[]> {
  const { data } = await api.get('/probes/status')
  return data || []
}

// Assets
export interface AssetInfo {
  id?: number
  server_id: number
  name: string
  category: string
  description: string
  tech_stack: string
  path: string
  port: string
  status: string
  extra_info: string
}

export async function getAssets(): Promise<AssetInfo[]> {
  const { data } = await api.get('/assets')
  return data || []
}

export async function createAsset(asset: AssetInfo): Promise<AssetInfo> {
  const { data } = await api.post('/assets', asset)
  return data
}

export async function updateAsset(id: number, asset: AssetInfo): Promise<AssetInfo> {
  const { data } = await api.put(`/assets/${id}`, asset)
  return data
}

export async function deleteAsset(id: number): Promise<void> {
  await api.delete(`/assets/${id}`)
}

// Billing
export interface BillingItem {
  type: string
  id: string
  name: string
  engine: string
  spec: string
  charge_type: string
  expire_date: string
  days_left: number
  status: string
  account_id: number
  account_name: string
}

export async function getBilling(): Promise<BillingItem[]> {
  const { data } = await api.get('/billing')
  return data || []
}

// Databases (RDS)
export interface RDSInfo {
  host_id: string
  name: string
  engine: string
  spec: string
  endpoint: string
  account_id: number
  account_name: string
  metrics: Record<string, number>
}

export async function getDatabases(): Promise<RDSInfo[]> {
  const { data } = await api.get('/databases')
  return data || []
}

export async function getDatabase(id: string): Promise<RDSInfo> {
  const { data } = await api.get(`/databases/${id}`)
  return data
}

// Groups
export async function getGroups(): Promise<ServerGroup[]> {
  const { data } = await api.get('/groups')
  return data || []
}

export async function createGroup(name: string) {
  const { data } = await api.post('/groups', { name })
  return data
}

export async function updateGroup(id: number, body: { name?: string; sort_order?: number }) {
  await api.put(`/groups/${id}`, body)
}

export async function deleteGroup(id: number) {
  await api.delete(`/groups/${id}`)
}

export async function setServerGroup(hostId: string, groupId: number | null) {
  await api.put(`/servers/${hostId}/group`, { group_id: groupId })
}

export async function batchSortGroups(items: { id: number; sort_order: number }[]) {
  await api.put('/groups-sort', { items })
}

export async function batchSortServers(items: { host_id: string; sort_order: number }[]) {
  await api.put('/servers-sort', { items })
}

// Alerts
export async function getAlertStats() {
  const { data } = await api.get('/alerts/stats')
  return data
}

export async function getAlertEvents(params: { status?: string; silenced?: boolean; limit?: number }) {
  const { data } = await api.get('/alerts/events', { params })
  return data || []
}

export default api
