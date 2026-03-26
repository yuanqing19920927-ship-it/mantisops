import { useEffect, useState } from 'react'
import { getProbes, getProbeStatus, createProbe, deleteProbe, type ProbeRule } from '../../api/client'
import { StatusBadge } from '../../components/StatusBadge'
import type { ProbeResult } from '../../types'

export default function Probes() {
  const [rules, setRules] = useState<ProbeRule[]>([])
  const [results, setResults] = useState<ProbeResult[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({
    name: '', host: '', port: '', server_id: 1,
    protocol: 'tcp' as 'tcp' | 'http' | 'https',
    url: '', expect_status: '200', expect_body: '',
  })

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
    if (form.protocol === 'tcp') {
      if (!form.name || !form.host || !form.port) return
    } else {
      if (!form.name || !form.url) return
    }
    await createProbe({
      server_id: form.protocol === 'tcp' ? form.server_id : null,
      name: form.name,
      host: form.protocol === 'tcp' ? form.host : '',
      port: form.protocol === 'tcp' ? parseInt(form.port) : 0,
      protocol: form.protocol,
      url: form.protocol !== 'tcp' ? form.url : '',
      expect_status: form.protocol !== 'tcp' ? parseInt(form.expect_status) || 0 : 0,
      expect_body: form.protocol !== 'tcp' ? form.expect_body : '',
      interval_sec: 30, timeout_sec: 5, enabled: true,
    })
    setForm({ name: '', host: '', port: '', server_id: 1, protocol: 'tcp', url: '', expect_status: '200', expect_body: '' })
    setShowAdd(false)
    load()
  }

  const handleDelete = async (id: number) => {
    if (!window.confirm('确定要删除此探测规则吗？此操作不可撤销。')) return
    try {
      await deleteProbe(id)
      load()
    } catch {
      alert('删除失败，请重试')
    }
  }

  // Derived stats
  const upCount = rules.filter((r) => {
    const res = getResult(r.id!)
    return res?.status === 'up'
  }).length

  const downCount = rules.filter((r) => {
    const res = getResult(r.id!)
    return res && res.status !== 'up'
  }).length

  const avgLatency = results.length > 0
    ? results.reduce((sum, r) => sum + r.latency_ms, 0) / results.length
    : 0

  const protocolOptions: Array<'tcp' | 'http' | 'https'> = ['tcp', 'http', 'https']

  return (
    <div>
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
        <div>
          <h1 className="font-headline text-2xl font-bold text-on-surface">探测管理中心</h1>
          <p className="text-sm text-on-surface-variant mt-1">端口连通性探测与服务可用性监控</p>
        </div>
        <button
          onClick={() => setShowAdd(!showAdd)}
          className="bg-gradient-to-br from-primary to-primary-container text-on-primary-container px-6 py-2.5 rounded-lg font-medium shadow-lg shadow-primary/20 flex items-center gap-2 hover:opacity-90 transition-opacity"
        >
          <span className="material-symbols-outlined text-lg">add_circle</span>
          添加新规则
        </button>
      </div>

      {/* Stats Row */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <div className="bg-surface-container p-4 sm:p-6 rounded-xl border-l-4 border-primary">
          <div className="flex items-center gap-3 mb-2">
            <span className="material-symbols-outlined text-primary text-xl">account_tree</span>
            <span className="text-sm text-on-surface-variant">总探测任务数</span>
          </div>
          <div className="text-2xl font-bold text-on-surface">{rules.length}</div>
        </div>

        <div className="bg-surface-container p-4 sm:p-6 rounded-xl border-l-4 border-tertiary">
          <div className="flex items-center gap-3 mb-2">
            <span className="material-symbols-outlined text-tertiary text-xl">bolt</span>
            <span className="text-sm text-on-surface-variant">正常运行</span>
          </div>
          <div className="text-2xl font-bold text-tertiary">{upCount}</div>
        </div>

        <div className="bg-surface-container p-4 sm:p-6 rounded-xl border-l-4 border-error">
          <div className="flex items-center gap-3 mb-2">
            <span className="material-symbols-outlined text-error text-xl">warning</span>
            <span className="text-sm text-on-surface-variant">异常告警</span>
          </div>
          <div className="text-2xl font-bold text-error">{downCount}</div>
        </div>

        <div className="bg-surface-container p-4 sm:p-6 rounded-xl border-l-4 border-on-secondary-container">
          <div className="flex items-center gap-3 mb-2">
            <span className="material-symbols-outlined text-on-secondary-container text-xl">speed</span>
            <span className="text-sm text-on-surface-variant">平均响应延迟</span>
          </div>
          <div className="text-2xl font-bold text-on-surface">{(avgLatency ?? 0).toFixed(1)} <span className="text-sm font-normal text-on-surface-variant">ms</span></div>
        </div>
      </div>

      {/* Add Form */}
      {showAdd && (
        <div className="bg-surface-container rounded-xl p-6 mb-8 border border-outline-variant/10">
          <div className="flex items-center gap-2 mb-4">
            <div className="w-1 h-6 bg-primary rounded-full" />
            <h2 className="font-headline text-xl font-bold text-on-surface">新建探测规则</h2>
          </div>

          {/* Protocol Selector */}
          <div className="flex gap-1 mb-4 bg-surface-container-lowest rounded-lg p-1 w-fit">
            {protocolOptions.map((p) => (
              <button
                key={p}
                onClick={() => setForm({ ...form, protocol: p })}
                className={`px-4 py-1.5 rounded-md text-xs font-bold uppercase transition-colors ${
                  form.protocol === p
                    ? 'bg-primary text-on-primary shadow-sm'
                    : 'text-on-surface-variant hover:text-on-surface'
                }`}
              >
                {p}
              </button>
            ))}
          </div>

          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <input
              placeholder="服务名称"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="bg-surface-container-lowest border-none rounded-lg px-4 py-2 text-sm focus:ring-1 focus:ring-primary/30 text-on-surface placeholder:text-on-surface-variant outline-none"
            />
            {form.protocol === 'tcp' ? (
              <>
                <input
                  placeholder="主机 IP / 域名"
                  value={form.host}
                  onChange={(e) => setForm({ ...form, host: e.target.value })}
                  className="bg-surface-container-lowest border-none rounded-lg px-4 py-2 text-sm focus:ring-1 focus:ring-primary/30 text-on-surface placeholder:text-on-surface-variant outline-none"
                />
                <input
                  placeholder="端口"
                  value={form.port}
                  onChange={(e) => setForm({ ...form, port: e.target.value })}
                  className="bg-surface-container-lowest border-none rounded-lg px-4 py-2 text-sm focus:ring-1 focus:ring-primary/30 text-on-surface placeholder:text-on-surface-variant outline-none"
                />
              </>
            ) : (
              <>
                <input
                  placeholder="URL（如 https://example.com）"
                  value={form.url}
                  onChange={(e) => setForm({ ...form, url: e.target.value })}
                  className="bg-surface-container-lowest border-none rounded-lg px-4 py-2 text-sm focus:ring-1 focus:ring-primary/30 text-on-surface placeholder:text-on-surface-variant outline-none"
                />
                <input
                  placeholder="期望状态码（默认200）"
                  value={form.expect_status}
                  onChange={(e) => setForm({ ...form, expect_status: e.target.value })}
                  className="bg-surface-container-lowest border-none rounded-lg px-4 py-2 text-sm focus:ring-1 focus:ring-primary/30 text-on-surface placeholder:text-on-surface-variant outline-none"
                />
              </>
            )}
            <div className="flex gap-2">
              <button
                onClick={handleAdd}
                className="flex-1 bg-gradient-to-br from-primary to-primary-container text-on-primary-container px-4 py-2 rounded-lg text-sm font-medium shadow-lg shadow-primary/20 hover:opacity-90 transition-opacity"
              >
                保存
              </button>
              <button
                onClick={() => setShowAdd(false)}
                className="px-4 py-2 rounded-lg text-sm text-on-surface-variant bg-surface-container-high hover:bg-surface-container transition-colors"
              >
                取消
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Section Title */}
      <div className="flex items-center gap-2 mb-6">
        <div className="w-1 h-6 bg-primary rounded-full" />
        <h2 className="font-headline text-xl font-bold text-on-surface">探测规则列表</h2>
      </div>

      {/* Probe Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 auto-rows-fr">
        {rules.map((rule) => {
          const result = getResult(rule.id!)
          const isDown = result && result.status !== 'up'
          const protocol = rule.protocol || 'tcp'

          return (
            <div
              key={rule.id}
              className={`bg-surface-container-low rounded-xl p-5 hover:bg-surface-container-high transition-colors group flex flex-col ${
                isDown ? 'border border-error/20' : ''
              }`}
            >
              {/* Card Header */}
              <div className="flex items-center justify-between mb-3">
                <span className="font-medium text-on-surface truncate">{rule.name}</span>
                <div className="flex items-center gap-2 flex-shrink-0">
                  {result?.ssl_expiry_days != null && (
                    <span className={`text-[10px] px-1.5 py-0.5 rounded font-bold ${
                      result.ssl_expiry_days > 60 ? 'bg-tertiary/20 text-tertiary' :
                      result.ssl_expiry_days > 30 ? 'bg-warning/20 text-warning' :
                      'bg-error/20 text-error'
                    }`}>
                      SSL {result.ssl_expiry_days}天
                    </span>
                  )}
                  <button
                    onClick={() => handleDelete(rule.id!)}
                    className="sm:opacity-0 sm:group-hover:opacity-100 transition-opacity text-error/60 hover:text-error p-1 rounded-lg hover:bg-error/20"
                  >
                    <span className="material-symbols-outlined text-base">delete</span>
                  </button>
                  <StatusBadge status={result?.status === 'up' ? 'up' : 'down'} label={result?.status === 'up' ? '正常' : '异常'} />
                </div>
              </div>

              {/* Address Badge */}
              <div className="mb-4">
                <span
                  className={`text-xs px-2 py-0.5 rounded font-mono ${
                    isDown
                      ? 'text-error bg-error/20'
                      : 'text-primary bg-primary/20'
                  }`}
                >
                  {protocol !== 'tcp' && rule.url ? rule.url : `${rule.host}:${rule.port}`}
                </span>
              </div>

              {/* Inner Stats */}
              <div className="grid grid-cols-2 gap-3 mb-3">
                <div className="bg-surface-container-lowest p-3 rounded-lg">
                  <div className="text-xs text-on-surface-variant mb-1">响应延迟</div>
                  <div className="text-sm font-semibold text-on-surface">
                    {result ? `${(result.latency_ms ?? 0).toFixed(1)} ms` : '-- ms'}
                  </div>
                </div>
                <div className="bg-surface-container-lowest p-3 rounded-lg relative group/status">
                  <div className="text-xs text-on-surface-variant mb-1">状态</div>
                  <div className={`text-sm font-semibold ${
                    result?.status === 'up' ? 'text-tertiary' : 'text-error'
                  }`}>
                    {result ? (result.status === 'up' ? '正常' : '异常') : '--'}
                  </div>
                  {result?.error && (
                    <div className="absolute left-0 right-0 top-full mt-1 z-10 hidden group-hover/status:block">
                      <div className="text-xs text-error bg-surface-container-highest rounded-lg px-3 py-2 shadow-xl border border-error/20 break-all">
                        {result.error}
                      </div>
                    </div>
                  )}
                </div>
              </div>

              {/* Spacer */}
              <div className="flex-1"></div>

              {/* Details link */}
              <div className="flex items-center justify-end mt-3">
                <span className="text-xs text-on-surface-variant opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer hover:text-primary">
                  详情 →
                </span>
              </div>
            </div>
          )
        })}

        {/* Add Card Placeholder */}
        {rules.length > 0 && (
          <button
            onClick={() => setShowAdd(true)}
            className="border-2 border-dashed border-outline-variant/20 hover:border-primary/40 rounded-xl p-5 flex flex-col items-center justify-center gap-2 transition-colors min-h-[200px]"
          >
            <span className="material-symbols-outlined text-3xl text-on-surface-variant/40">add_circle</span>
            <span className="text-sm text-on-surface-variant/60">添加新规则</span>
          </button>
        )}
      </div>

      {/* Empty State */}
      {rules.length === 0 && !showAdd && (
        <div className="text-center py-20">
          <span className="material-symbols-outlined text-5xl text-on-surface-variant/30 mb-4 block">radar</span>
          <p className="text-on-surface-variant text-lg mb-2">暂无探测规则</p>
          <p className="text-on-surface-variant/60 text-sm">点击「添加新规则」开始监控您的服务</p>
        </div>
      )}
    </div>
  )
}
