import api from './client'

export interface AuditLog {
  id: number
  timestamp: string
  username: string
  action: string
  resource_type: string
  resource_id: string
  resource_name: string
  detail: string
  ip_address: string
  user_agent: string
}

export interface RuntimeLog {
  id: number
  timestamp: string
  level: string
  module: string
  source: string
  message_preview: string
  message?: string
}

export interface LogPage<T> {
  data: T[] | null
  total: number
  page?: number
  page_size?: number
}

export interface LogQuery {
  start?: string
  end?: string
  level?: string
  module?: string
  source?: string
  keyword?: string
  username?: string
  action?: string
  resource_type?: string
  page?: number
  page_size?: number
}

export async function getAuditLogs(q: LogQuery): Promise<LogPage<AuditLog>> {
  const { data } = await api.get('/logs/audit', { params: q })
  return data
}

export async function getRuntimeLogs(q: LogQuery): Promise<LogPage<RuntimeLog>> {
  const { data } = await api.get('/logs/runtime', { params: q })
  return data
}

export async function getLogSources(): Promise<string[]> {
  const { data } = await api.get('/logs/sources')
  return data || []
}

export async function getLogStats(start?: string, end?: string): Promise<Record<string, number>> {
  const { data } = await api.get('/logs/stats', { params: { start, end } })
  return data || {}
}

export async function exportLogs(params: LogQuery & { type: string; format: string }): Promise<void> {
  const { data, headers } = await api.get('/logs/export', {
    params,
    responseType: 'blob',
  })
  const disposition = headers['content-disposition'] || ''
  const match = disposition.match(/filename=([^\s;]+)/)
  const filename = match ? match[1] : `${params.type}-logs.${params.format}`
  const url = URL.createObjectURL(data)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}
