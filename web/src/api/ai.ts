import api from './client'

// Types
export interface AIReport {
  id: number
  report_type: string
  title: string
  summary: string
  content?: string
  period_start: number
  period_end: number
  status: string
  error_message?: string
  trigger_type: string
  provider: string
  model: string
  token_usage: number
  generation_time_ms: number
  created_at: string
  updated_at: string
}

export interface AIConversation {
  id: number
  title: string
  user: string
  provider: string
  model: string
  message_count: number
  last_message_at: number | null
  created_at: string
}

export interface AIMessage {
  id: number
  conversation_id: number
  role: string
  content: string
  status: string
  error_message?: string
  request_id?: string
  prompt_tokens?: number
  completion_tokens?: number
  created_at: string
}

export interface AISchedule {
  id: number
  report_type: string
  enabled: boolean
  cron_expr: string
  last_run_at: number | null
  next_run_at: number | null
}

export interface ProviderInfo {
  name: string
  configured: boolean
  active: boolean
}

// Report API
export const listReports = (params?: { type?: string; status?: string; limit?: number; offset?: number }) =>
  api.get('/ai/reports', { params }).then(r => r.data)

export const getReport = (id: number) =>
  api.get(`/ai/reports/${id}`).then(r => r.data)

export const generateReport = (data: { report_type: string; period_start?: number; period_end?: number; force?: boolean }) =>
  api.post('/ai/reports/generate', data).then(r => r.data)

export const deleteReport = (id: number) =>
  api.delete(`/ai/reports/${id}`)

export const downloadReport = async (id: number, title: string) => {
  const { data } = await api.get(`/ai/reports/${id}/download`, { responseType: 'blob' })
  const url = URL.createObjectURL(data)
  const a = document.createElement('a')
  a.href = url
  a.download = `${title}.md`
  a.click()
  URL.revokeObjectURL(url)
}

export const latestReport = () =>
  api.get('/ai/reports/latest').then(r => r.data)

// Conversation API
export const listConversations = (params?: { limit?: number; offset?: number }) =>
  api.get('/ai/conversations', { params }).then(r => r.data)

export const createConversation = () =>
  api.post('/ai/conversations').then(r => r.data)

export const getConversation = (id: number) =>
  api.get(`/ai/conversations/${id}`).then(r => r.data)

export const deleteConversation = (id: number) =>
  api.delete(`/ai/conversations/${id}`)

export const sendMessage = (convId: number, data: { content: string; request_id: string }) =>
  api.post(`/ai/conversations/${convId}/messages`, data).then(r => r.data)

// Settings API
export const getAISettings = () => api.get('/ai/settings').then(r => r.data)
export const updateAISettings = (data: Record<string, unknown>) => api.put('/ai/settings', data).then(r => r.data)
export const listProviders = () => api.get('/ai/providers').then(r => r.data) as Promise<ProviderInfo[]>
export const testProvider = (data: Record<string, unknown>) => api.post('/ai/providers/test', data).then(r => r.data)
export const listSchedules = () => api.get('/ai/schedules').then(r => r.data) as Promise<AISchedule[]>
export const updateSchedule = (id: number, data: { enabled?: boolean; cron_expr?: string }) =>
  api.put(`/ai/schedules/${id}`, data).then(r => r.data)
