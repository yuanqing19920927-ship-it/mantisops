import { useEffect, useState } from 'react'
import { getProbes, getProbeStatus, createProbe, deleteProbe, type ProbeRule } from '../../api/client'
import { useAuthStore } from '../../stores/authStore'
import type { ProbeResult } from '../../types'

export default function Probes() {
  const role = useAuthStore((s) => s.role)
  const canEdit = role === 'admin' || role === 'operator'
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
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 className="text-[22px] font-semibold text-[#495057]">探测管理</h1>
          <p className="text-sm text-[#878a99] mt-1">端口连通性探测与服务可用性监控</p>
        </div>
        {canEdit && (
          <button
            onClick={() => setShowAdd(!showAdd)}
            className="inline-flex items-center gap-2 bg-[#2ca07a] hover:bg-[#259b73] text-white px-4 py-2 rounded-[6px] text-sm font-medium transition-colors shadow-sm"
          >
            <span className="material-symbols-outlined text-[16px]">add</span>
            添加探测规则
          </button>
        )}
      </div>

      {/* Stats Row */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        {/* Total */}
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-full bg-[#2ca07a]/15 flex items-center justify-center flex-shrink-0">
              <span className="material-symbols-outlined text-[#2ca07a] text-[20px]">account_tree</span>
            </div>
            <div>
              <div className="text-2xl font-bold text-[#495057]">{rules.length}</div>
              <div className="text-[12px] text-[#878a99]">总探测任务</div>
            </div>
          </div>
        </div>

        {/* Running OK */}
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-full bg-[#0ab39c]/15 flex items-center justify-center flex-shrink-0">
              <span className="material-symbols-outlined text-[#0ab39c] text-[20px]">check_circle</span>
            </div>
            <div>
              <div className="text-2xl font-bold text-[#0ab39c]">{upCount}</div>
              <div className="text-[12px] text-[#878a99]">正常运行</div>
            </div>
          </div>
        </div>

        {/* Alert */}
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5 border-b-2 border-[#f06548]">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-full bg-[#f06548]/15 flex items-center justify-center flex-shrink-0">
              <span className="material-symbols-outlined text-[#f06548] text-[20px]">warning</span>
            </div>
            <div>
              <div className="text-2xl font-bold text-[#f06548]">{downCount}</div>
              <div className="text-[12px] text-[#878a99]">异常告警</div>
            </div>
          </div>
        </div>

        {/* Avg Latency */}
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-full bg-[#f7b84b]/15 flex items-center justify-center flex-shrink-0">
              <span className="material-symbols-outlined text-[#f7b84b] text-[20px]">speed</span>
            </div>
            <div>
              <div className="text-2xl font-bold text-[#495057]">
                {(avgLatency ?? 0).toFixed(1)}
                <span className="text-sm font-normal text-[#878a99] ml-1">ms</span>
              </div>
              <div className="text-[12px] text-[#878a99]">平均响应延迟</div>
            </div>
          </div>
        </div>
      </div>

      {/* Add Form */}
      {showAdd && (
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-6 mb-6">
          <h2 className="text-base font-semibold text-[#495057] mb-4">新建探测规则</h2>

          {/* Protocol Selector */}
          <div className="flex gap-1 mb-4 bg-[#f3f6f9] rounded-[6px] p-1 w-fit">
            {protocolOptions.map((p) => (
              <button
                key={p}
                onClick={() => setForm({ ...form, protocol: p })}
                className={`px-4 py-1.5 rounded-[4px] text-xs font-bold uppercase transition-colors ${
                  form.protocol === p
                    ? 'bg-white text-[#2ca07a] shadow-sm'
                    : 'text-[#878a99] hover:text-[#495057]'
                }`}
              >
                {p}
              </button>
            ))}
          </div>

          <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
            <input
              placeholder="服务名称"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
            />
            {form.protocol === 'tcp' ? (
              <>
                <input
                  placeholder="主机 IP / 域名"
                  value={form.host}
                  onChange={(e) => setForm({ ...form, host: e.target.value })}
                  className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
                />
                <input
                  placeholder="端口"
                  value={form.port}
                  onChange={(e) => setForm({ ...form, port: e.target.value })}
                  className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
                />
              </>
            ) : (
              <>
                <input
                  placeholder="URL（如 https://example.com）"
                  value={form.url}
                  onChange={(e) => setForm({ ...form, url: e.target.value })}
                  className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
                />
                <input
                  placeholder="期望状态码（默认200）"
                  value={form.expect_status}
                  onChange={(e) => setForm({ ...form, expect_status: e.target.value })}
                  className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
                />
              </>
            )}
            <div className="flex gap-2">
              <button
                onClick={handleAdd}
                className="flex-1 bg-[#2ca07a] hover:bg-[#259b73] text-white px-4 py-2 rounded-[6px] text-sm font-medium transition-colors"
              >
                保存
              </button>
              <button
                onClick={() => setShowAdd(false)}
                className="px-4 py-2 rounded-[6px] text-sm text-[#878a99] bg-[#f3f6f9] hover:bg-[#e9ebec] transition-colors"
              >
                取消
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Probe Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
        {rules.map((rule) => {
          const result = getResult(rule.id!)
          const isDown = result && result.status !== 'up'
          const protocol = rule.protocol || 'tcp'

          return (
            <div
              key={rule.id}
              className={`bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5 flex flex-col group ${
                isDown ? 'border border-[#f06548]/40' : ''
              }`}
            >
              {/* Card Header */}
              <div className="flex items-start justify-between gap-2 mb-3">
                <div className="flex items-center gap-2 min-w-0">
                  <span
                    className={`w-3 h-3 rounded-full flex-shrink-0 ${
                      !result ? 'bg-[#adb5bd]' :
                      result.status === 'up' ? 'bg-[#0ab39c] animate-pulse' : 'bg-[#f06548]'
                    }`}
                    style={result?.status === 'up' ? { boxShadow: '0 0 0 3px rgba(10,179,156,0.2)' } :
                      result ? { boxShadow: '0 0 0 3px rgba(240,101,72,0.2)' } : {}}
                  />
                  <span className="font-medium text-[#495057] text-sm truncate">{rule.name}</span>
                </div>
                <div className="flex items-center gap-1 flex-shrink-0">
                  {result?.ssl_expiry_days != null && (
                    <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium ${
                      result.ssl_expiry_days > 60 ? 'bg-[#0ab39c]/10 text-[#0ab39c]' :
                      result.ssl_expiry_days > 30 ? 'bg-[#f7b84b]/10 text-[#f7b84b]' :
                      'bg-[#f06548]/10 text-[#f06548]'
                    }`}>
                      SSL {result.ssl_expiry_days}天
                    </span>
                  )}
                  <button
                    onClick={() => handleDelete(rule.id!)}
                    className="sm:opacity-0 sm:group-hover:opacity-100 transition-opacity text-[#878a99] hover:text-[#f06548] p-1 rounded hover:bg-[#f06548]/10"
                  >
                    <span className="material-symbols-outlined text-[14px]">delete</span>
                  </button>
                </div>
              </div>

              {/* Address Badge */}
              <div className="mb-4">
                <span
                  className={`text-[11px] px-2 py-1 rounded font-mono inline-block max-w-full truncate ${
                    isDown
                      ? 'bg-[#f06548]/10 text-[#f06548]'
                      : 'bg-[#495057]/8 text-[#495057]'
                  }`}
                  style={!isDown ? { backgroundColor: 'rgba(73,80,87,0.08)' } : {}}
                >
                  {protocol !== 'tcp' && rule.url ? rule.url : `${rule.host}:${rule.port}`}
                </span>
              </div>

              {/* Stats */}
              <div className="flex items-end justify-between mt-auto pt-3 border-t border-[#f3f6f9]">
                <div>
                  <div className="text-lg font-bold text-[#495057]">
                    {result ? `${(result.latency_ms ?? 0).toFixed(1)}` : '--'}
                    <span className="text-xs font-normal text-[#878a99] ml-1">ms</span>
                  </div>
                  <div className="text-[11px] text-[#878a99] mt-0.5">
                    {result ? (result.status === 'up' ? '运行正常' : result.error || '连接异常') : '等待检测'}
                  </div>
                </div>
                {/* Sparkline placeholder */}
                <div className="w-16 h-8 bg-[#f3f6f9] rounded flex items-end gap-px px-1 overflow-hidden">
                  {[40, 60, 45, 70, 55, 80, 50].map((h, i) => (
                    <div
                      key={i}
                      className={`flex-1 rounded-sm ${result?.status === 'up' ? 'bg-[#0ab39c]/40' : 'bg-[#f06548]/40'}`}
                      style={{ height: `${h}%` }}
                    />
                  ))}
                </div>
              </div>
            </div>
          )
        })}

        {/* Add Placeholder Card */}
        {rules.length > 0 && (
          <button
            onClick={() => setShowAdd(true)}
            className="border-2 border-dashed border-[#ced4da] hover:border-[#2ca07a] rounded-[10px] p-5 flex flex-col items-center justify-center gap-2 transition-colors min-h-[160px] text-[#878a99] hover:text-[#2ca07a]"
          >
            <span className="material-symbols-outlined text-3xl">add_circle</span>
            <span className="text-sm font-medium">添加新规则</span>
          </button>
        )}
      </div>

      {/* Empty State */}
      {rules.length === 0 && !showAdd && (
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-16 text-center">
          <span className="material-symbols-outlined text-5xl text-[#ced4da] mb-4 block">radar</span>
          <p className="text-[#495057] text-base font-medium mb-1">暂无探测规则</p>
          <p className="text-[#878a99] text-sm">点击「添加探测规则」开始监控您的服务</p>
        </div>
      )}
    </div>
  )
}
