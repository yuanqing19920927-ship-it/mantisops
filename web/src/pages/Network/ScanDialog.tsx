import { useState, useEffect } from 'react'
import { startScan, cancelScan } from '../../api/network'
import { useNetworkStore } from '../../stores/networkStore'

interface ScanDialogProps {
  open: boolean
  onClose: () => void
}

function isValidCidr(cidr: string): boolean {
  const trimmed = cidr.trim()
  const match = trimmed.match(/^(\d{1,3}\.){3}\d{1,3}\/(\d+)$/)
  if (!match) return false
  const prefix = parseInt(trimmed.split('/')[1], 10)
  if (prefix < 24 || prefix > 32) return false
  const parts = trimmed.split('/')[0].split('.').map(Number)
  return parts.every((p) => p >= 0 && p <= 255)
}

function countHosts(cidr: string): number {
  const prefix = parseInt(cidr.trim().split('/')[1], 10)
  return Math.pow(2, 32 - prefix)
}

function estimateSeconds(cidrs: string[]): number {
  const total = cidrs.reduce((sum, c) => sum + countHosts(c), 0)
  // total IPs * 10ms / concurrency(256) / 1000 = seconds
  return Math.ceil((total * 10) / 256 / 1000)
}

export default function ScanDialog({ open, onClose }: ScanDialogProps) {
  const [input, setInput] = useState('')
  const [errors, setErrors] = useState<string[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [cancelling, setCancelling] = useState(false)

  const scanStatus = useNetworkStore((s) => s.scanStatus)
  const fetchScanStatus = useNetworkStore((s) => s.fetchScanStatus)

  const isScanning =
    scanStatus?.status === 'running' || scanStatus?.status === 'scanning'

  // Poll scan status while dialog is open
  useEffect(() => {
    if (!open) return
    fetchScanStatus()
    const t = setInterval(fetchScanStatus, 2000)
    return () => clearInterval(t)
  }, [open, fetchScanStatus])

  if (!open) return null

  const parsedCidrs = input
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)

  const validCidrs = parsedCidrs.filter(isValidCidr)
  const estimate = validCidrs.length > 0 ? estimateSeconds(validCidrs) : 0

  function validate(): boolean {
    const errs: string[] = []
    parsedCidrs.forEach((c) => {
      if (!isValidCidr(c)) {
        errs.push(`"${c}" 不是有效的 CIDR（需 /24 ~ /32）`)
      }
    })
    if (parsedCidrs.length === 0) {
      errs.push('请输入至少一个网段')
    }
    setErrors(errs)
    return errs.length === 0
  }

  async function handleScan() {
    if (!validate()) return
    setSubmitting(true)
    try {
      await startScan(validCidrs)
      await fetchScanStatus()
    } catch (e: any) {
      setErrors([e?.response?.data?.error || '启动扫描失败'])
    } finally {
      setSubmitting(false)
    }
  }

  async function handleCancel() {
    setCancelling(true)
    try {
      await cancelScan()
      await fetchScanStatus()
    } catch {
      // ignore
    } finally {
      setCancelling(false)
    }
  }

  function handleClose() {
    if (isScanning) return
    setInput('')
    setErrors([])
    onClose()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="glass-card w-full max-w-md mx-4 p-6 rounded-xl">
        {/* Header */}
        <div className="flex items-center justify-between mb-5">
          <div className="flex items-center gap-2">
            <span className="material-symbols-outlined text-[#2ca07a]">radar</span>
            <h2 className="text-base font-semibold text-on-surface">网络扫描</h2>
          </div>
          {!isScanning && (
            <button
              onClick={handleClose}
              className="text-on-surface-variant hover:text-on-surface transition-colors"
            >
              <span className="material-symbols-outlined text-xl">close</span>
            </button>
          )}
        </div>

        {/* Warning */}
        <div className="flex gap-2 p-3 rounded-lg bg-[rgba(245,158,11,0.1)] border border-[rgba(245,158,11,0.2)] mb-4">
          <span className="material-symbols-outlined text-[#f59e0b] text-base shrink-0 mt-0.5">
            warning
          </span>
          <p className="text-xs text-[#f59e0b]">
            网络扫描会向目标网段发送 ICMP/ARP 探测包，请确认您有权限扫描该网段。
          </p>
        </div>

        {isScanning ? (
          /* Scanning progress view */
          <div className="space-y-4">
            <div>
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs text-on-surface-variant">
                  {scanStatus?.current_subnet
                    ? `正在扫描：${scanStatus.current_subnet}`
                    : '准备中...'}
                </span>
                <span className="text-xs font-semibold text-on-surface">
                  {Math.round(scanStatus?.progress ?? 0)}%
                </span>
              </div>
              <div className="h-2 w-full rounded-full bg-[#e9ecef] overflow-hidden">
                <div
                  className="h-full rounded-full bg-[#2ca07a] transition-all duration-500"
                  style={{ width: `${scanStatus?.progress ?? 0}%` }}
                />
              </div>
            </div>

            <div className="flex gap-3 pt-1">
              <button
                onClick={handleCancel}
                disabled={cancelling}
                className="flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg border border-[#ef4444] text-[#ef4444] text-sm font-medium hover:bg-[rgba(239,68,68,0.08)] transition-colors disabled:opacity-50"
              >
                {cancelling ? (
                  <span className="material-symbols-outlined text-base animate-spin">
                    progress_activity
                  </span>
                ) : (
                  <span className="material-symbols-outlined text-base">stop_circle</span>
                )}
                取消扫描
              </button>
              <button
                onClick={handleClose}
                className="px-4 py-2 rounded-lg text-sm text-on-surface-variant hover:text-on-surface transition-colors"
              >
                后台运行
              </button>
            </div>
          </div>
        ) : (
          /* Input view */
          <div className="space-y-4">
            <div>
              <label className="block text-xs font-medium text-on-surface-variant mb-1.5">
                扫描网段（逗号分隔，前缀需 ≥ /24）
              </label>
              <textarea
                className="w-full rounded-lg border border-[rgba(255,255,255,0.12)] bg-[rgba(255,255,255,0.05)] text-on-surface text-sm px-3 py-2 focus:outline-none focus:border-[#2ca07a] resize-none placeholder-[#6c757d]"
                rows={3}
                placeholder="192.168.10.0/24, 192.168.20.0/24"
                value={input}
                onChange={(e) => {
                  setInput(e.target.value)
                  setErrors([])
                }}
              />
              {errors.length > 0 && (
                <ul className="mt-1 space-y-0.5">
                  {errors.map((e, i) => (
                    <li key={i} className="text-xs text-[#ef4444]">
                      {e}
                    </li>
                  ))}
                </ul>
              )}
            </div>

            {validCidrs.length > 0 && (
              <div className="text-xs text-on-surface-variant">
                预计扫描 {validCidrs.reduce((s, c) => s + countHosts(c), 0).toLocaleString()} 个 IP，约需{' '}
                <span className="text-on-surface font-medium">
                  {estimate >= 60
                    ? `${Math.floor(estimate / 60)} 分 ${estimate % 60} 秒`
                    : `${estimate} 秒`}
                </span>
              </div>
            )}

            <div className="flex gap-3 pt-1">
              <button
                onClick={handleClose}
                className="px-4 py-2 rounded-lg text-sm text-on-surface-variant hover:text-on-surface transition-colors"
              >
                取消
              </button>
              <button
                onClick={handleScan}
                disabled={submitting}
                className="flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg bg-[#2ca07a] text-white text-sm font-medium hover:bg-[#259068] transition-colors disabled:opacity-50"
              >
                {submitting ? (
                  <span className="material-symbols-outlined text-base animate-spin">
                    progress_activity
                  </span>
                ) : (
                  <span className="material-symbols-outlined text-base">radar</span>
                )}
                开始扫描
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
