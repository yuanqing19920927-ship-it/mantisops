import { useEffect, useRef } from 'react'
import { useServerStore } from '../stores/serverStore'

export function useWebSocket() {
  const wsRef = useRef<WebSocket | null>(null)
  const updateMetrics = useServerStore((s) => s.updateMetrics)

  useEffect(() => {
    let reconnectTimer: ReturnType<typeof setTimeout>

    function connect() {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${protocol}//${window.location.host}/ws`)
      wsRef.current = ws

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          if (msg.type === 'metrics' && msg.host_id && msg.data) {
            updateMetrics(msg.host_id, msg.data)
          }
        } catch {
          // ignore parse errors
        }
      }

      ws.onclose = () => {
        reconnectTimer = setTimeout(connect, 3000)
      }

      ws.onerror = () => {
        ws.close()
      }
    }

    connect()

    return () => {
      clearTimeout(reconnectTimer)
      wsRef.current?.close()
    }
  }, [updateMetrics])
}
