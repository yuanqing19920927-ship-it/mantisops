import { useServerStore } from '../../stores/serverStore'
import { useSettingsStore } from '../../stores/settingsStore'
import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { StatusBadge } from '../../components/StatusBadge'
import { timeSince } from '../../utils/format'
import { getAISettings, updateAISettings, testProvider, listSchedules, updateSchedule } from '../../api/ai'
import type { AISchedule } from '../../api/ai'

const REPORT_TYPE_LABELS: Record<string, string> = {
  daily: '日报', weekly: '周报', monthly: '月报', quarterly: '季度报告', yearly: '年度报告',
}

export default function Settings() {
  const { servers, metrics, fetchDashboard } = useServerStore()
  const { platformName } = useSettingsStore()

  // AI config state
  const [aiProvider, setAiProvider] = useState('ollama')
  const [aiApiKey, setAiApiKey] = useState('')
  const [aiBaseUrl, setAiBaseUrl] = useState('')
  const [aiReportModel, setAiReportModel] = useState('')
  const [aiChatModel, setAiChatModel] = useState('')
  const [aiSchedules, setAiSchedules] = useState<AISchedule[]>([])
  const [aiLoaded, setAiLoaded] = useState(false)
  const [aiSaving, setAiSaving] = useState(false)
  const [aiSaved, setAiSaved] = useState(false)
  const [aiTesting, setAiTesting] = useState(false)
  const [aiTestResult, setAiTestResult] = useState<{ ok: boolean; error?: string } | null>(null)

  const fetchAI = useCallback(async () => {
    try {
      const [settings, schedules] = await Promise.all([getAISettings(), listSchedules()])
      const provider = settings.active_provider || 'ollama'
      setAiProvider(provider)
      const provCfg = settings[provider] || {}
      setAiApiKey(provCfg.api_key || '')
      setAiBaseUrl(provCfg.base_url || provCfg.host || '')
      setAiReportModel(provCfg.report_model || '')
      setAiChatModel(provCfg.chat_model || '')
      setAiSchedules(schedules || [])
    } catch { /* AI not enabled, use defaults */ }
    setAiLoaded(true)
  }, [])

  useEffect(() => { fetchDashboard(); fetchAI() }, [fetchDashboard, fetchAI])

  const onlineCount = servers.filter((s) => s.status === 'online').length

  return (
    <div className="flex flex-col gap-5">

      {/* Header */}
      <div className="mb-1">
        <h1 className="text-[22px] font-semibold text-[#495057]">系统信息</h1>
        <p className="text-sm text-[#878a99] mt-1">Settings</p>
      </div>

      {/* System Overview Card */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-6">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 rounded-full bg-[#2ca07a]/15 flex items-center justify-center flex-shrink-0">
              <span className="material-symbols-outlined text-[#2ca07a] text-[20px]">deployed_code</span>
            </div>
            <div>
              <div className="text-sm font-semibold text-[#495057]">{platformName}</div>
              <div className="text-[12px] text-[#878a99] mt-0.5">前端版本 <span className="font-mono text-[#495057]">v1.0.0</span></div>
            </div>
          </div>
          <div className="mt-5 grid grid-cols-2 gap-3">
            <div className="bg-[#f8f9fa] rounded-[8px] p-3 text-center">
              <div className="text-2xl font-bold text-[#0ab39c]">{onlineCount}</div>
              <div className="text-[11px] text-[#878a99] mt-0.5">代理在线</div>
            </div>
            <div className="bg-[#f8f9fa] rounded-[8px] p-3 text-center">
              <div className="text-2xl font-bold text-[#495057]">{servers.length}</div>
              <div className="text-[11px] text-[#878a99] mt-0.5">总代理数</div>
            </div>
          </div>
        </div>
      </div>

      {/* Agent list */}
      <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
        <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
            <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">hub</span>
          </div>
          <h2 className="text-base font-semibold text-[#495057]">已注册代理</h2>
          <span className="text-[11px] px-2 py-0.5 rounded-full bg-[#2ca07a]/10 text-[#2ca07a] font-semibold">
            {servers.length}
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-[#f8f9fa]">
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">主机名</th>
                <th className="hidden sm:table-cell text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">主机 ID</th>
                <th className="hidden sm:table-cell text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">版本</th>
                <th className="hidden md:table-cell text-center text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">Docker</th>
                <th className="hidden md:table-cell text-center text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">GPU</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">最后心跳</th>
                <th className="text-center text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">状态</th>
                <th className="text-center text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">操作</th>
              </tr>
            </thead>
            <tbody>
              {servers.map((s, idx) => {
                const m = metrics[s.host_id]
                const hasDocker = m?.containers !== undefined
                const hasGpu = !!s.gpu_model
                return (
                  <tr
                    key={s.host_id}
                    className={`hover:bg-[#f8f9fa] transition-colors ${idx < servers.length - 1 ? 'border-b border-[#f2f4f7]' : ''}`}
                  >
                    <td className="px-5 py-3.5">
                      <span className="text-[#495057] font-medium text-sm">{s.display_name || s.hostname}</span>
                    </td>
                    <td className="hidden sm:table-cell px-5 py-3.5">
                      <span className="text-[11px] font-mono bg-[#f3f6f9] text-[#495057] px-2 py-0.5 rounded">
                        {s.host_id}
                      </span>
                    </td>
                    <td className="hidden sm:table-cell px-5 py-3.5">
                      <span className="text-[12px] text-[#878a99]">{s.agent_version || '-'}</span>
                    </td>
                    <td className="hidden md:table-cell px-5 py-3.5 text-center">
                      <span className={`text-[11px] px-2 py-0.5 rounded font-medium ${hasDocker ? 'bg-[#0ab39c]/10 text-[#0ab39c]' : 'bg-[#878a99]/10 text-[#878a99]'}`}>
                        {hasDocker ? '采集中' : '未开启'}
                      </span>
                    </td>
                    <td className="hidden md:table-cell px-5 py-3.5 text-center">
                      <span className={`text-[11px] px-2 py-0.5 rounded font-medium ${hasGpu ? 'bg-[#0ab39c]/10 text-[#0ab39c]' : 'bg-[#878a99]/10 text-[#878a99]'}`}>
                        {hasGpu ? s.gpu_model : '无'}
                      </span>
                    </td>
                    <td className="px-5 py-3.5">
                      <span className="text-[12px] text-[#878a99]">{s.last_seen ? timeSince(s.last_seen) : '-'}</span>
                    </td>
                    <td className="px-5 py-3.5 text-center">
                      <StatusBadge status={s.status} label={s.status === 'online' ? '在线' : '离线'} />
                    </td>
                    <td className="px-5 py-3.5 text-center">
                      <Link to={`/servers/${s.host_id}`}
                        className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded transition-colors">
                        详情
                      </Link>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>

          {servers.length === 0 && (
            <div className="py-12 text-center">
              <span className="material-symbols-outlined text-3xl text-[#ced4da] mb-2 block">devices_off</span>
              <p className="text-[#878a99] text-sm">暂无已注册的代理</p>
            </div>
          )}
        </div>
      </div>

      {/* ── AI 配置 ── */}
      {aiLoaded && (
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
          <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
            <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
              <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">smart_toy</span>
            </div>
            <div>
              <h2 className="text-base font-semibold text-[#495057]">AI 配置</h2>
              <p className="text-[11px] text-[#878a99] mt-0.5">配置 AI 分析引擎的提供商、模型和定时报告</p>
            </div>
          </div>
          <div className="p-5 space-y-5">
            {/* Provider selector */}
            <div>
              <label className="block text-[12px] font-medium text-[#878a99] mb-2">活跃提供商</label>
              <div className="flex gap-2">
                {(['claude', 'openai', 'ollama'] as const).map((p) => (
                  <button
                    key={p}
                    onClick={() => { setAiProvider(p); setAiTestResult(null) }}
                    className={`text-[13px] px-4 py-2 rounded-lg transition-colors font-medium ${
                      aiProvider === p
                        ? 'bg-[#2ca07a] text-white'
                        : 'bg-[#f3f6f9] text-[#878a99] hover:text-[#495057]'
                    }`}
                  >
                    {p === 'claude' ? 'Claude' : p === 'openai' ? 'OpenAI' : 'Ollama'}
                  </button>
                ))}
              </div>
            </div>

            {/* Provider-specific fields */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              {(aiProvider === 'claude' || aiProvider === 'openai') && (
                <div className="sm:col-span-2">
                  <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">API Key</label>
                  <input
                    type="password"
                    value={aiApiKey}
                    onChange={(e) => setAiApiKey(e.target.value)}
                    placeholder={aiProvider === 'claude' ? 'sk-ant-...' : 'sk-...'}
                    className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors font-mono"
                  />
                </div>
              )}
              {(aiProvider === 'openai' || aiProvider === 'ollama') && (
                <div className="sm:col-span-2">
                  <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">
                    {aiProvider === 'ollama' ? 'Ollama Host' : 'Base URL'}
                  </label>
                  <input
                    type="text"
                    value={aiBaseUrl}
                    onChange={(e) => setAiBaseUrl(e.target.value)}
                    placeholder={aiProvider === 'ollama' ? 'http://192.168.10.69:11434' : 'https://api.openai.com/v1'}
                    className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors font-mono"
                  />
                </div>
              )}
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">报告模型</label>
                <input
                  type="text"
                  value={aiReportModel}
                  onChange={(e) => setAiReportModel(e.target.value)}
                  placeholder={aiProvider === 'claude' ? 'claude-sonnet-4-20250514' : aiProvider === 'openai' ? 'gpt-4o' : 'qwen2.5:7b'}
                  className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors font-mono"
                />
              </div>
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">对话模型</label>
                <input
                  type="text"
                  value={aiChatModel}
                  onChange={(e) => setAiChatModel(e.target.value)}
                  placeholder={aiProvider === 'claude' ? 'claude-haiku-4-5-20251001' : aiProvider === 'openai' ? 'gpt-4o-mini' : 'qwen2.5:7b'}
                  className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors font-mono"
                />
              </div>
            </div>

            {/* Test + Save buttons */}
            <div className="flex items-center gap-3 flex-wrap">
              <button
                onClick={async () => {
                  setAiTesting(true); setAiTestResult(null)
                  try {
                    const model = aiReportModel || (aiProvider === 'claude' ? 'claude-sonnet-4-20250514' : aiProvider === 'openai' ? 'gpt-4o' : 'qwen2.5:7b')
                    const host = aiBaseUrl || (aiProvider === 'ollama' ? 'http://192.168.10.69:11434' : 'https://api.openai.com/v1')
                    const payload: Record<string, string> = { provider: aiProvider, model }
                    if (aiProvider === 'claude' || aiProvider === 'openai') payload.api_key = aiApiKey
                    if (aiProvider === 'openai') payload.host = host
                    if (aiProvider === 'ollama') payload.host = host
                    const res = await testProvider(payload)
                    setAiTestResult({ ok: res.ok, error: res.error })
                  } catch (e: any) {
                    setAiTestResult({ ok: false, error: e.response?.data?.error || e.message })
                  }
                  setAiTesting(false)
                }}
                disabled={aiTesting}
                className="flex items-center gap-1.5 px-3 py-2 text-[12px] border border-[#ced4da] text-[#495057] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded-lg transition-colors disabled:opacity-50"
              >
                {aiTesting ? (
                  <span className="w-3 h-3 border border-[#2ca07a]/40 border-t-[#2ca07a] rounded-full animate-spin flex-shrink-0" />
                ) : (
                  <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>network_check</span>
                )}
                {aiTesting ? '测试中...' : '测试连接'}
              </button>
              <button
                onClick={async () => {
                  setAiSaving(true); setAiSaved(false)
                  try {
                    const payload: Record<string, unknown> = { active_provider: aiProvider }
                    if (aiProvider === 'claude') {
                      payload.claude = { api_key: aiApiKey, report_model: aiReportModel, chat_model: aiChatModel }
                    } else if (aiProvider === 'openai') {
                      payload.openai = { api_key: aiApiKey, base_url: aiBaseUrl, report_model: aiReportModel, chat_model: aiChatModel }
                    } else if (aiProvider === 'ollama') {
                      payload.ollama = { host: aiBaseUrl, report_model: aiReportModel, chat_model: aiChatModel }
                    }
                    await updateAISettings(payload)
                    setAiSaved(true)
                    setTimeout(() => setAiSaved(false), 3000)
                  } catch { /* ignore */ }
                  setAiSaving(false)
                }}
                disabled={aiSaving}
                className="flex items-center gap-1.5 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded-lg transition-colors disabled:opacity-50"
              >
                <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>save</span>
                {aiSaving ? '保存中...' : '保存 AI 配置'}
              </button>
              {aiSaved && (
                <span className="text-[12px] text-[#0ab39c] flex items-center gap-1">
                  <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>check_circle</span>
                  已保存
                </span>
              )}
            </div>

            {/* Test result */}
            {aiTestResult && (
              <div className={`px-3 py-2 rounded-[6px] text-[12px] ${aiTestResult.ok ? 'bg-[#0ab39c]/8 text-[#0ab39c] border border-[#0ab39c]/20' : 'bg-[#f06548]/8 text-[#f06548] border border-[#f06548]/20'}`}>
                {aiTestResult.ok ? '✓ 连接成功' : `✗ ${aiTestResult.error || '连接失败'}`}
              </div>
            )}

            {/* Scheduled reports */}
            {aiSchedules.length > 0 && (
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-2">定时报告</label>
                <div className="space-y-2">
                  {aiSchedules.map((s) => (
                    <div key={s.id} className="flex items-center justify-between px-3 py-2.5 bg-[#f8f9fa] rounded-lg">
                      <div>
                        <span className="text-sm font-medium text-[#495057]">{REPORT_TYPE_LABELS[s.report_type] || s.report_type}</span>
                        <span className="text-[11px] text-[#878a99] ml-2 font-mono">{s.cron_expr}</span>
                      </div>
                      <button
                        onClick={async () => {
                          await updateSchedule(s.id, { enabled: !s.enabled })
                          fetchAI()
                        }}
                        className={`relative w-10 h-5 rounded-full transition-colors ${s.enabled ? 'bg-[#2ca07a]' : 'bg-[#ced4da]'}`}
                      >
                        <span className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform ${s.enabled ? 'left-5' : 'left-0.5'}`} />
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      )}

    </div>
  )
}
