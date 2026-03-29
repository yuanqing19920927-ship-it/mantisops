import api from './client'

export interface NasDevice {
  id: number
  name: string
  nas_type: 'synology' | 'fnos'
  host: string
  port: number
  ssh_user: string
  credential_id: number
  collect_interval: number
  status: 'online' | 'offline' | 'degraded' | 'unknown'
  last_seen: string | null
  system_info: string
  created_at: string
  updated_at: string
}

export interface NasCPU { usage_percent: number }
export interface NasMemory { total: number; used: number; usage_percent: number }
export interface NasNetwork { interface: string; rx_bytes_per_sec: number; tx_bytes_per_sec: number }
export interface NasRaid { array: string; raid_type: string; status: string; disks: string[]; rebuild_percent: number }
export interface NasVolume { mount: string; fs_type: string; total: number; used: number; usage_percent: number }
export interface NasDisk { name: string; model: string; size: number; temperature: number; power_on_hours: number; smart_healthy: boolean; reallocated_sectors: number }
export interface NasUPS { status: string; battery_percent: number; model: string }

export interface NasMetrics {
  nas_id: number
  timestamp: string
  cpu?: NasCPU
  memory?: NasMemory
  networks?: NasNetwork[]
  raids?: NasRaid[]
  volumes?: NasVolume[]
  disks?: NasDisk[]
  ups?: NasUPS
}

export async function getNasDevices(): Promise<NasDevice[]> {
  const { data } = await api.get('/nas-devices')
  return data
}

export async function createNasDevice(req: { name: string; nas_type: string; host: string; port: number; ssh_user: string; credential_id?: number; password?: string; collect_interval: number }): Promise<{ id: number }> {
  const { data } = await api.post('/nas-devices', req)
  return data
}

export async function updateNasDevice(id: number, req: { name: string; nas_type: string; host: string; port: number; ssh_user: string; credential_id?: number; password?: string; collect_interval: number }): Promise<void> {
  await api.put(`/nas-devices/${id}`, req)
}

export async function deleteNasDevice(id: number): Promise<void> {
  await api.delete(`/nas-devices/${id}`)
}

export async function testNasConnection(req: { host: string; port: number; ssh_user: string; credential_id?: number; password?: string }): Promise<{ ok: boolean; error?: string; detected_type?: string; smart_available?: boolean }> {
  const { data } = await api.post('/nas-devices/test', req)
  return data
}

export async function getNasMetrics(id: number): Promise<NasMetrics> {
  const { data } = await api.get(`/nas-devices/${id}/metrics`)
  return data
}
