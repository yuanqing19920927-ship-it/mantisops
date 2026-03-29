import { useEffect, useState, useCallback } from 'react'
import { useAuthStore } from '../../stores/authStore'
import api, { deleteProbe, type ProbeRule } from '../../api/client'
import type { ProbeResult } from '../../types'

interface Props {
  serverId: number  // servers.id (numeric)
  serverIp: string  // primary IP for default host value
}

export function ProbeSection({ serverId, serverIp }: Props) {
  const role = useAuthStore((s) => s.role)
  const canWrite = role === 'admin' || role === 'operator'
  const [rules, setRules] = useState<ProbeRule[]>([])
  const [results, setResults] = useState<ProbeResult[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [deleteId, setDeleteId] = useState<number | null>(null)

  // New rule form
  const [protocol, setProtocol] = useState<'tcp' | 'http' | 'https'>('tcp')
  const [name, setName] = useState('')
  const [host, setHost] = useState('')
  const [port, setPort] = useState('')
  const [url, setUrl] = useState('')
  const [expectStatus, setExpectStatus] = useState('200')
  const [expectBody, setExpectBody] = useState('')
  const [saving, setSaving] = useState(false)

  const fetchRules = useCallback(async () => {
    try {
      const data = await api.get(`/probes?server_id=${serverId}`).then(r => r.data)
      setRules(data || [])
    } catch { /* ignore */ }
  }, [serverId])

  const fetchStatus = useCallback(async () => {
    try {
      const data = await api.get(`/probes/status?server_id=${serverId}`).then(r => r.data)
      setResults(data || [])
    } catch { /* ignore */ }
  }, [serverId])

  useEffect(() => {
    fetchRules()
    fetchStatus()
    const timer = setInterval(fetchStatus, 10000)
    return () => clearInterval(timer)
  }, [fetchRules, fetchStatus])

  const resultMap = new Map(results.map(r => [r.rule_id, r]))

  const resetForm = () => {
    setProtocol('tcp')
    setName('')
    setHost(serverIp)
    setPort('')
    setUrl('')
    setExpectStatus('200')
    setExpectBody('')
  }

  const handleAdd = async () => {
    setSaving(true)
    try {
      const rule: Partial<ProbeRule> = {
        server_id: serverId,
        name,
        protocol,
        host: protocol === 'tcp' ? host : '',
        port: protocol === 'tcp' ? Number(port) : 0,
        url: protocol !== 'tcp' ? url : '',
        expect_status: protocol !== 'tcp' ? Number(expectStatus) : 0,
        expect_body: protocol !== 'tcp' ? expectBody : '',
        enabled: true,
      }
      await api.post('/probes', rule)
      setShowAdd(false)
      resetForm()
      fetchRules()
    } catch (err) {
      console.error('[probe] create:', err)
    }
    setSaving(false)
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteProbe(id)
      setDeleteId(null)
      fetchRules()
    } catch (err) {
      console.error('[probe] delete:', err)
    }
  }

  const sslBadge = (days: number | undefined) => {
    if (days === undefined || days < 0) return null
    const color = days > 60 ? '#0ab39c' : days > 30 ? '#f7b84b' : '#f06548'
    return (
      <span className="text-[10px] font-medium px-1.5 py-0.5 rounded" style={{ background: `${color}20`, color }}>
        SSL {days}天
      </span>
    )
  }

  return (
    <div className="bg-white overflow-hidden mb-5" style={{ borderRadius: 10, boxShadow: '0 1px 3px rgba(0,0,0,0.05)' }}>
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-4" style={{ borderBottom: '1px solid #f3f4f6' }}>
        <div className="flex items-center gap-3">
          <h5 className="mb-0 font-semibold" style={{ fontSize: 15, color: '#212529' }}>端口检测</h5>
          {rules.length > 0 && (
            <span className="text-[11px] px-2 py-0.5 rounded-full bg-[#2ca07a]/10 text-[#2ca07a] font-semibold">
              {rules.length}
            </span>
          )}
        </div>
        {canWrite && (
          <button
            onClick={() => { resetForm(); setHost(serverIp); setShowAdd(true) }}
            className="flex items-center gap-1 px-3 py-1.5 text-[12px] font-medium text-white rounded-lg transition-all hover:shadow-md"
            style={{ background: 'linear-gradient(135deg, #2ca07a, #0ab39c)' }}
          >
            <span className="material-symbols-outlined" style={{ fontSize: 16 }}>add</span>
            添加规则
          </button>
        )}
      </div>

      {/* Rules grid */}
      {rules.length === 0 ? (
        <div className="py-10 text-center">
          <span className="material-symbols-outlined text-3xl text-[#ced4da] mb-2 block">sensors</span>
          <p className="text-[#878a99] text-sm">暂无探测规则</p>
          <p className="text-[#adb5bd] text-[11px] mt-1">点击「添加规则」或在配置中开启自动端口检测</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3 p-4">
          {rules.map(rule => {
            const r = resultMap.get(rule.id!)
            const isUp = r?.status === 'up'
            const isDown = r?.status === 'down'
            return (
              <div
                key={rule.id}
                className="group relative rounded-lg border p-3 transition-all hover:shadow-md"
                style={{
                  borderColor: isDown ? 'rgba(240,101,72,0.4)' : '#e9ebec',
                  background: isDown ? 'rgba(240,101,72,0.03)' : 'white',
                }}
              >
                {/* Delete button */}
                {canWrite && (
                  <button
                    onClick={() => setDeleteId(rule.id!)}
                    className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 p-0.5 rounded hover:bg-[rgba(240,101,72,0.1)] text-[#878a99] hover:text-[#f06548] transition-all"
                  >
                    <span className="material-symbols-outlined" style={{ fontSize: 15 }}>close</span>
                  </button>
                )}

                {/* Name + protocol */}
                <div className="flex items-center gap-2 mb-2">
                  <span className="text-[13px] font-semibold text-[#495057] truncate">{rule.name}</span>
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-[#f3f6f9] text-[#878a99] font-medium uppercase shrink-0">
                    {rule.protocol}
                  </span>
                  {rule.source === 'scan' && (
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-[#2ca07a]/10 text-[#2ca07a] font-medium shrink-0">
                      自动
                    </span>
                  )}
                </div>

                {/* Address */}
                <div className="text-[11px] text-[#878a99] font-mono truncate mb-2">
                  {rule.protocol === 'tcp' ? `${rule.host}:${rule.port}` : rule.url}
                </div>

                {/* Status + latency + SSL */}
                <div className="flex items-center gap-2">
                  <span className="inline-flex items-center gap-1">
                    <span
                      className="inline-block w-2 h-2 rounded-full"
                      style={{
                        background: isUp ? '#0ab39c' : isDown ? '#f06548' : '#ced4da',
                        boxShadow: isUp ? '0 0 5px rgba(10,179,156,0.6)' : isDown ? '0 0 5px rgba(240,101,72,0.6)' : 'none',
                      }}
                    />
                    <span className="text-[11px] font-medium" style={{ color: isUp ? '#0ab39c' : isDown ? '#f06548' : '#878a99' }}>
                      {isUp ? 'UP' : isDown ? 'DOWN' : '-'}
                    </span>
                  </span>
                  {r?.latency_ms !== undefined && r.latency_ms > 0 && (
                    <span className="text-[10px] text-[#878a99]">{r.latency_ms}ms</span>
                  )}
                  {rule.protocol === 'https' && sslBadge(r?.ssl_expiry_days)}
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* Add Rule Dialog */}
      {showAdd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setShowAdd(false)}>
          <div className="bg-white rounded-xl shadow-xl w-[440px] max-w-[90vw] overflow-hidden" onClick={e => e.stopPropagation()}>
            <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
              <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
                <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">sensors</span>
              </div>
              <h3 className="text-sm font-semibold text-[#495057]">添加探测规则</h3>
            </div>

            <div className="px-5 py-5 space-y-4">
              {/* Protocol selector */}
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">协议</label>
                <div className="flex gap-2">
                  {(['tcp', 'http', 'https'] as const).map(p => (
                    <button
                      key={p}
                      onClick={() => setProtocol(p)}
                      className={`text-[12px] px-3 py-1.5 rounded-lg font-medium transition-colors ${
                        protocol === p ? 'bg-[#2ca07a] text-white' : 'bg-[#f3f6f9] text-[#878a99] hover:text-[#495057]'
                      }`}
                    >
                      {p.toUpperCase()}
                    </button>
                  ))}
                </div>
              </div>

              {/* Name */}
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">服务名称</label>
                <input type="text" value={name} onChange={e => setName(e.target.value)} placeholder="如：Web 服务"
                  className="w-full border border-[#e9ebec] rounded-lg px-3 py-2 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] transition-colors" />
              </div>

              {/* TCP fields */}
              {protocol === 'tcp' && (
                <div className="grid grid-cols-3 gap-3">
                  <div className="col-span-2">
                    <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">主机 IP</label>
                    <input type="text" value={host} onChange={e => setHost(e.target.value)}
                      className="w-full border border-[#e9ebec] rounded-lg px-3 py-2 text-sm text-[#495057] font-mono focus:outline-none focus:border-[#2ca07a] transition-colors" />
                  </div>
                  <div>
                    <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">端口</label>
                    <input type="number" value={port} onChange={e => setPort(e.target.value)} placeholder="80"
                      className="w-full border border-[#e9ebec] rounded-lg px-3 py-2 text-sm text-[#495057] font-mono focus:outline-none focus:border-[#2ca07a] transition-colors" />
                  </div>
                </div>
              )}

              {/* HTTP/HTTPS fields */}
              {protocol !== 'tcp' && (
                <>
                  <div>
                    <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">URL</label>
                    <input type="text" value={url} onChange={e => setUrl(e.target.value)} placeholder={`${protocol}://example.com`}
                      className="w-full border border-[#e9ebec] rounded-lg px-3 py-2 text-sm text-[#495057] font-mono focus:outline-none focus:border-[#2ca07a] transition-colors" />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">期望状态码</label>
                      <input type="number" value={expectStatus} onChange={e => setExpectStatus(e.target.value)} placeholder="200"
                        className="w-full border border-[#e9ebec] rounded-lg px-3 py-2 text-sm text-[#495057] font-mono focus:outline-none focus:border-[#2ca07a] transition-colors" />
                    </div>
                    <div>
                      <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">关键字匹配</label>
                      <input type="text" value={expectBody} onChange={e => setExpectBody(e.target.value)} placeholder="可选"
                        className="w-full border border-[#e9ebec] rounded-lg px-3 py-2 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] transition-colors" />
                    </div>
                  </div>
                </>
              )}
            </div>

            <div className="px-5 py-3 border-t border-[#e9ebec] flex justify-end gap-2">
              <button onClick={() => setShowAdd(false)}
                className="text-[12px] px-4 py-2 border border-[#ced4da] text-[#878a99] rounded-lg hover:bg-[#f8f9fa] transition-colors">
                取消
              </button>
              <button onClick={handleAdd} disabled={saving || !name || (protocol === 'tcp' ? !host || !port : !url)}
                className="text-[12px] px-4 py-2 bg-[#2ca07a] text-white rounded-lg hover:bg-[#248a69] transition-colors disabled:opacity-50">
                {saving ? '添加中...' : '添加'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete confirm */}
      {deleteId !== null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setDeleteId(null)}>
          <div className="bg-white rounded-xl shadow-xl w-[360px] max-w-[90vw] p-6" onClick={e => e.stopPropagation()}>
            <div className="flex items-center gap-2 mb-3">
              <span className="material-symbols-outlined text-[#f06548]" style={{ fontSize: 20 }}>warning</span>
              <h5 className="text-[15px] font-semibold text-[#495057]">确认删除</h5>
            </div>
            <p className="text-[13px] text-[#878a99] mb-5">删除后该端口将不再被监控，确定要删除吗？</p>
            <div className="flex justify-end gap-2">
              <button onClick={() => setDeleteId(null)} className="px-4 py-2 text-[12px] text-[#878a99] hover:text-[#495057] transition-colors">取消</button>
              <button onClick={() => handleDelete(deleteId)} className="px-4 py-2 text-[12px] font-medium text-white bg-[#f06548] hover:bg-[#d9534f] rounded-lg transition-colors">删除</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
