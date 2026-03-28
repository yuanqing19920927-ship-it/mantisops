import api from './client'

export interface DiscoveredService {
  id: number
  host_id: string
  pid: number
  name: string
  cmd_line: string
  port: number
  protocol: string
  bind_addr: string
  status: string
  asset_id: number | null
  first_seen: string
  last_seen: string
}

export async function getDiscoveredServices(): Promise<Record<string, DiscoveredService[]>> {
  const { data } = await api.get('/assets/discovered')
  return data || {}
}
