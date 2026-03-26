import { useEffect, useState } from 'react'
import { getProbes, getProbeStatus, createProbe, deleteProbe, type ProbeRule } from '../../api/client'
import { StatusBadge } from '../../components/StatusBadge'
import type { ProbeResult } from '../../types'

export default function Probes() {
  const [rules, setRules] = useState<ProbeRule[]>([])
  const [results, setResults] = useState<ProbeResult[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ name: '', host: '', port: '', server_id: 1 })

  const load = async () => {
    const [r, s] = await Promise.all([getProbes(), getProbeStatus()])
    setRules(r)
    setResults(s)
  }

  useEffect(() => { load() }, [])
  useEffect(() => {
    const timer = setInterval(async () => {
      const s = await getProbeStatus()
      setResults(s)
    }, 10000)
    return () => clearInterval(timer)
  }, [])

  const getResult = (ruleId: number) => results.find((r) => r.rule_id === ruleId)

  const handleAdd = async () => {
    if (!form.name || !form.host || !form.port) return
    await createProbe({
      server_id: form.server_id, name: form.name, host: form.host,
      port: parseInt(form.port), protocol: 'tcp', interval_sec: 30,
      timeout_sec: 5, enabled: true,
    })
    setForm({ name: '', host: '', port: '', server_id: 1 })
    setShowAdd(false)
    load()
  }

  const handleDelete = async (id: number) => {
    await deleteProbe(id)
    load()
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>端口监控</h1>
        <button onClick={() => setShowAdd(!showAdd)}
          className="px-4 py-2 rounded-lg text-sm text-white"
          style={{ backgroundColor: 'var(--accent)' }}>
          + 添加规则
        </button>
      </div>

      {showAdd && (
        <div className="rounded-xl p-4 mb-6" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
          <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
            <input placeholder="名称" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="px-3 py-2 rounded-lg text-sm" style={{ backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border)' }} />
            <input placeholder="主机 IP" value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })}
              className="px-3 py-2 rounded-lg text-sm" style={{ backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border)' }} />
            <input placeholder="端口" value={form.port} onChange={(e) => setForm({ ...form, port: e.target.value })}
              className="px-3 py-2 rounded-lg text-sm" style={{ backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border)' }} />
            <button onClick={handleAdd} className="px-4 py-2 rounded-lg text-sm text-white" style={{ backgroundColor: 'var(--success)' }}>
              保存
            </button>
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        {rules.map((rule) => {
          const result = getResult(rule.id!)
          return (
            <div key={rule.id} className="rounded-xl p-4" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
              <div className="flex items-center justify-between mb-2">
                <span className="font-medium" style={{ color: 'var(--text-primary)' }}>{rule.name}</span>
                <StatusBadge status={result?.status === 'up' ? 'up' : 'down'} />
              </div>
              <div className="text-sm mb-2" style={{ color: 'var(--text-secondary)' }}>
                {rule.host}:{rule.port}
              </div>
              {result && (
                <div className="text-xs" style={{ color: 'var(--text-secondary)' }}>
                  延迟: {result.latency_ms.toFixed(1)}ms
                  {result.error && <span style={{ color: 'var(--danger)' }}> | {result.error}</span>}
                </div>
              )}
              <button onClick={() => handleDelete(rule.id!)}
                className="mt-2 text-xs px-2 py-1 rounded" style={{ color: 'var(--danger)' }}>
                删除
              </button>
            </div>
          )
        })}
      </div>

      {rules.length === 0 && (
        <div className="text-center py-12" style={{ color: 'var(--text-secondary)' }}>
          暂无探测规则，点击"添加规则"开始监控
        </div>
      )}
    </div>
  )
}
