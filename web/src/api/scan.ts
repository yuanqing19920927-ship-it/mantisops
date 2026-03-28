import api from './client'

export interface ScanTemplate {
  id: number
  port: number
  name: string
  enabled: boolean
  sort_order: number
}

export async function getScanTemplates(): Promise<ScanTemplate[]> {
  const { data } = await api.get('/scan-templates')
  return data || []
}

export async function createScanTemplate(port: number, name: string): Promise<void> {
  await api.post('/scan-templates', { port, name })
}

export async function updateScanTemplate(id: number, body: { port: number; name: string; enabled: boolean }): Promise<void> {
  await api.put(`/scan-templates/${id}`, body)
}

export async function deleteScanTemplate(id: number): Promise<void> {
  await api.delete(`/scan-templates/${id}`)
}

export async function startScan(hostIds: string[]): Promise<{ task_id: string }> {
  const { data } = await api.post('/probes/scan', { host_ids: hostIds })
  return data
}
