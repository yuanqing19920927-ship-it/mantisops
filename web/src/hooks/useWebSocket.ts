import { useEffect, useRef } from 'react'
import { useServerStore } from '../stores/serverStore'
import { useAlertStore } from '../stores/alertStore'
import { useAIStore } from '../stores/aiStore'
import type { DeployProgress, CloudSyncProgress } from '../types/onboarding'

let globalWs: WebSocket | null = null
let refCount = 0

export function useWebSocket() {
  const updateMetrics = useServerStore((s) => s.updateMetrics)
  const updateMetricsRef = useRef(updateMetrics)
  updateMetricsRef.current = updateMetrics

  const addAlert = useAlertStore((s) => s.addEvent)
  const addAlertRef = useRef(addAlert)
  addAlertRef.current = addAlert

  const resolveAlert = useAlertStore((s) => s.resolveEvent)
  const resolveAlertRef = useRef(resolveAlert)
  resolveAlertRef.current = resolveAlert

  const silenceAlert = useAlertStore((s) => s.silenceEvent)
  const silenceAlertRef = useRef(silenceAlert)
  silenceAlertRef.current = silenceAlert

  const appendChunk = useAIStore((s) => s.appendStreamChunk)
  const appendChunkRef = useRef(appendChunk)
  appendChunkRef.current = appendChunk

  const finalizeStream = useAIStore((s) => s.finalizeStream)
  const finalizeStreamRef = useRef(finalizeStream)
  finalizeStreamRef.current = finalizeStream

  const setStreamError = useAIStore((s) => s.setStreamError)
  const setStreamErrorRef = useRef(setStreamError)
  setStreamErrorRef.current = setStreamError

  const addGenReport = useAIStore((s) => s.addGeneratingReport)
  const addGenReportRef = useRef(addGenReport)
  addGenReportRef.current = addGenReport

  const removeGenReport = useAIStore((s) => s.removeGeneratingReport)
  const removeGenReportRef = useRef(removeGenReport)
  removeGenReportRef.current = removeGenReport

  useEffect(() => {
    refCount++
    if (refCount > 1) return () => { refCount-- }

    let reconnectTimer: ReturnType<typeof setTimeout>
    let disposed = false

    function connect() {
      if (disposed) return
      const token = localStorage.getItem('token')
      if (!token) return // 未登录不连接
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${protocol}//${window.location.host}/ws?token=${encodeURIComponent(token)}`)
      globalWs = ws;
      (window as any).__mantisops_ws = ws

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          if (msg.type === 'metrics' && msg.host_id && msg.data) {
            updateMetricsRef.current(msg.host_id, msg.data)
          }
          if (msg.type === 'alert' && msg.data) {
            addAlertRef.current(msg.data)
          }
          if (msg.type === 'alert_resolved' && msg.data) {
            resolveAlertRef.current(msg.data.id)
          }
          if (msg.type === 'alert_acked' && msg.data) {
            silenceAlertRef.current(msg.data.id, msg.data.acked_by)
          }
          if (msg.type === 'deploy_progress') {
            const detail: DeployProgress = msg
            window.dispatchEvent(new CustomEvent<DeployProgress>('deploy_progress', { detail }))
          }
          if (msg.type === 'cloud_sync_progress') {
            const detail: CloudSyncProgress = msg
            window.dispatchEvent(new CustomEvent<CloudSyncProgress>('cloud_sync_progress', { detail }))
          }
          if (msg.type === 'log' && msg.data) {
            window.dispatchEvent(new CustomEvent('ws_log', { detail: msg.data }))
          }
          if (msg.type === 'nas_metrics' && msg.nas_id && msg.data) {
            window.dispatchEvent(new CustomEvent('nas_metrics', { detail: { nas_id: msg.nas_id, data: msg.data } }))
          }
          if (msg.type === 'nas_status' && msg.nas_id) {
            window.dispatchEvent(new CustomEvent('nas_status', { detail: { nas_id: msg.nas_id, status: msg.status } }))
          }
          if (msg.type === 'scan_progress' && msg.data) {
            window.dispatchEvent(new CustomEvent('scan_progress', { detail: msg.data }))
          }
          if (msg.type === 'scan_complete' && msg.data) {
            window.dispatchEvent(new CustomEvent('scan_complete', { detail: msg.data }))
          }
          if (msg.type === 'ai_chat_chunk') {
            if (msg.done) {
              finalizeStreamRef.current()
            } else {
              appendChunkRef.current(msg.content || '')
            }
          }
          if (msg.type === 'ai_chat_error') {
            setStreamErrorRef.current(msg.message_id, msg.error || 'Unknown error')
          }
          if (msg.type === 'ai_report_generating') {
            addGenReportRef.current(msg.report_id)
          }
          if (msg.type === 'ai_report_completed' || msg.type === 'ai_report_failed') {
            removeGenReportRef.current(msg.report_id)
            window.dispatchEvent(new CustomEvent(msg.type, { detail: msg }))
          }
        } catch {
          // ignore
        }
      }

      ws.onclose = () => {
        globalWs = null
        if (!disposed) {
          reconnectTimer = setTimeout(connect, 3000)
        }
      }

      ws.onerror = () => {
        ws.close()
      }
    }

    connect()

    return () => {
      refCount--
      if (refCount <= 0) {
        disposed = true
        clearTimeout(reconnectTimer)
        globalWs?.close()
        globalWs = null;
        (window as any).__mantisops_ws = null
        refCount = 0
      }
    }
  }, [])
}

export function sendWsMessage(msg: Record<string, unknown>) {
  const ws = (window as any).__mantisops_ws as WebSocket | null
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(msg))
  }
}
