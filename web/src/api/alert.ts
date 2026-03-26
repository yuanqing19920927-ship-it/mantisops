import api from './client'
import type { AlertRule, AlertEvent, AlertStats, NotificationChannel, AlertNotificationDetail } from '../types'

// Rules
export async function getAlertRules(): Promise<AlertRule[]> {
  const { data } = await api.get('/alerts/rules')
  return data || []
}
export async function createAlertRule(rule: Partial<AlertRule>): Promise<AlertRule> {
  const { data } = await api.post('/alerts/rules', rule)
  return data
}
export async function updateAlertRule(id: number, rule: Partial<AlertRule>): Promise<AlertRule> {
  const { data } = await api.put(`/alerts/rules/${id}`, rule)
  return data
}
export async function deleteAlertRule(id: number): Promise<void> {
  await api.delete(`/alerts/rules/${id}`)
}

// Events
export async function getAlertEvents(params?: {
  status?: string; silenced?: string; since?: string; until?: string; limit?: number; offset?: number
}): Promise<AlertEvent[]> {
  const { data } = await api.get('/alerts/events', { params })
  return data || []
}
export async function getAlertStats(): Promise<AlertStats> {
  const { data } = await api.get('/alerts/stats')
  return data
}
export async function ackAlertEvent(id: number): Promise<void> {
  await api.put(`/alerts/events/${id}/ack`)
}
export async function getEventNotifications(id: number): Promise<AlertNotificationDetail[]> {
  const { data } = await api.get(`/alerts/events/${id}/notifications`)
  return data || []
}

// Channels
export async function getAlertChannels(): Promise<NotificationChannel[]> {
  const { data } = await api.get('/alerts/channels')
  return data || []
}
export async function createAlertChannel(ch: Partial<NotificationChannel>): Promise<NotificationChannel> {
  const { data } = await api.post('/alerts/channels', ch)
  return data
}
export async function updateAlertChannel(id: number, ch: Partial<NotificationChannel>): Promise<NotificationChannel> {
  const { data } = await api.put(`/alerts/channels/${id}`, ch)
  return data
}
export async function deleteAlertChannel(id: number): Promise<void> {
  await api.delete(`/alerts/channels/${id}`)
}
export async function testAlertChannel(id: number): Promise<void> {
  await api.post(`/alerts/channels/${id}/test`)
}
