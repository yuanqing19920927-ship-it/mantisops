import { useEffect, useRef } from 'react'
import { useServerStore } from '../stores/serverStore'
import { useAlertStore } from '../stores/alertStore'
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
