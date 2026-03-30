import api from './client'

export interface NetworkSubnet {
  id: number
  cidr: string
  name: string
  gateway: string
  total_hosts: number
  alive_hosts: number
  last_scan: string | null
}

export interface NetworkDevice {
  id: number
  ip: string
  mac: string
  vendor: string
  device_type: string
  hostname: string
  snmp_supported: boolean
  snmp_credential_id: number
  sys_descr: string
  sys_name: string
  sys_object_id: string
  model: string
  subnet_id: number | null
  status: string
  last_seen: string | null
  first_seen: string
  server_id: number
}

export interface NetworkLink {
  id: number
  source_id: number
  target_id: number
  source_port: string
  target_port: string
  protocol: string
  bandwidth: string
  last_seen: string
}

export interface TopologyData {
  nodes: NetworkDevice[]
  edges: NetworkLink[]
}

export interface ScanStatus {
  status: string
  current_subnet: string
  progress: number
  started_at: string | null
  error: string
}

export async function listSubnets(): Promise<NetworkSubnet[]> {
  const { data } = await api.get('/network/subnets')
  return data
}

export async function listDevices(subnetId?: number): Promise<NetworkDevice[]> {
  const params = subnetId !== undefined ? { subnet_id: subnetId } : {}
  const { data } = await api.get('/network/devices', { params })
  return data
}

export async function getDevice(id: number): Promise<NetworkDevice> {
  const { data } = await api.get(`/network/devices/${id}`)
  return data
}

export async function getTopology(): Promise<TopologyData> {
  const { data } = await api.get('/network/topology')
  return data
}

export async function startScan(subnets: string[]): Promise<void> {
  await api.post('/network/scan', { subnets })
}

export async function getScanStatus(): Promise<ScanStatus> {
  const { data } = await api.get('/network/scan/status')
  return data
}

export async function cancelScan(): Promise<void> {
  await api.post('/network/scan/cancel')
}

export async function updateDevice(
  id: number,
  req: { device_type?: string; hostname?: string }
): Promise<void> {
  await api.put(`/network/devices/${id}`, req)
}

export async function deleteDevice(id: number): Promise<void> {
  await api.delete(`/network/devices/${id}`)
}
