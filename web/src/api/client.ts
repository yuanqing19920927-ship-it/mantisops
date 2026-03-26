import axios from 'axios'
import type { DashboardData, Server, ProbeResult } from '../types'

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

// Probes
export interface ProbeRule {
  id?: number
  server_id: number
  name: string
  host: string
  port: number
  protocol: string
  interval_sec: number
  timeout_sec: number
  enabled: boolean
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

export default api
