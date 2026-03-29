import { useEffect, useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '../../stores/authStore'
import { useAIStore } from '../../stores/aiStore'
import { listReports, generateReport, deleteReport, getPrompts, updatePrompts } from '../../api/ai'
import type { AIReport } from '../../api/ai'

const REPORT_TYPES: Record<string, string> = {
  daily: '日报',
  weekly: '周报',
  monthly: '月报',
  quarterly: '季度报告',
  yearly: '年度报告',
}

const TRIGGER_LABELS: Record<string, string> = {
  scheduled: '自动',
  manual: '手动',
}

const FILTER_TABS = [
  { key: '', label: '全部' },
  { key: 'daily', label: '日报' },
  { key: 'weekly', label: '周报' },
  { key: 'monthly', label: '月报' },
  { key: 'quarterly', label: '季度' },
  { key: 'yearly', label: '年度' },
] as const

function formatDate(dateStr: string): string {
  const d = new Date(dateStr)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function StatusBadge({ status }: { status: string }) {
  if (status === 'completed') {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-[rgba(10,179,156,0.1)] text-[#0ab39c]">
        <span className="w-1.5 h-1.5 rounded-full bg-[#0ab39c]" />
        已完成
      </span>
    )
  }
  if (status === 'generating') {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-[rgba(247,184,75,0.1)] text-[#f7b84b] animate-pulse">
        <span className="w-1.5 h-1.5 rounded-full bg-[#f7b84b]" />
        生成中
      </span>
    )
  }
  if (status === 'failed') {
    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-[rgba(240,101,72,0.1)] text-[#f06548]">
        <span className="w-1.5 h-1.5 rounded-full bg-[#f06548]" />
        失败
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-[rgba(135,138,153,0.1)] text-[#878a99]">
      {status}
    </span>
  )
}

export default function AIReports() {
  const navigate = useNavigate()
  const role = useAuthStore((s) => s.role)
  const canWrite = role === 'admin' || role === 'operator'
  const { reports, reportsTotal, generatingReportIds, setReports } = useAIStore()
  const [filter, setFilter] = useState('')
  const [showDialog, setShowDialog] = useState(false)
  const [genType, setGenType] = useState('daily')
  const [genLoading, setGenLoading] = useState(false)
  const [elapsedMap, setElapsedMap] = useState<Record<number, number>>({})
  const [deleteConfirm, setDeleteConfirm] = useState<number | null>(null)

  // Template editor state
  const [showTemplateEditor, setShowTemplateEditor] = useState(false)
  const [templateTab, setTemplateTab] = useState('daily')
  const [templates, setTemplates] = useState<Record<string, string>>({})
  const [defaults, setDefaults] = useState<Record<string, string>>({})
  const [templateSaving, setTemplateSaving] = useState(false)
  const [templateSaved, setTemplateSaved] = useState(false)

  const openTemplateEditor = async () => {
    try {
      const data = await getPrompts()
      setDefaults(data.defaults || {})
      // For each type, show custom value if set, otherwise show default
      const merged: Record<string, string> = {}
      for (const t of Object.keys(REPORT_TYPES)) {
        merged[t] = data.custom?.[t] || data.defaults?.[t] || ''
      }
      setTemplates(merged)
      setTemplateTab('daily')
      setTemplateSaved(false)
      setShowTemplateEditor(true)
    } catch (err) {
      console.error('[ai-reports] load prompts failed:', err)
    }
  }

  const handleTemplateSave = async () => {
    setTemplateSaving(true)
    try {
      // Only save if different from default; send empty string to reset to default
      const payload: Record<string, string> = {}
      payload[templateTab] = templates[templateTab] === defaults[templateTab] ? '' : templates[templateTab]
      await updatePrompts(payload)
      setTemplateSaved(true)
      setTimeout(() => setTemplateSaved(false), 2000)
    } catch (err) {
      console.error('[ai-reports] save prompt:', err)
    } finally {
      setTemplateSaving(false)
    }
  }

  const fetchReports = useCallback(async () => {
    try {
      const params: Record<string, string | number> = { limit: 50 }
      if (filter) params.type = filter
      const res = await listReports(params)
      setReports(res.reports ?? [], res.total ?? 0)
    } catch (err) {
      console.error('[ai-reports] fetch failed:', err)
    }
  }, [filter, setReports])

  useEffect(() => {
    fetchReports()
  }, [fetchReports])

  // Listen for ai_report_completed custom event
  useEffect(() => {
    const handler = () => { fetchReports() }
    window.addEventListener('ai_report_completed', handler)
    return () => window.removeEventListener('ai_report_completed', handler)
  }, [fetchReports])

  // Elapsed time timer for generating reports
  useEffect(() => {
    if (generatingReportIds.length === 0) return
    const interval = setInterval(() => {
      setElapsedMap((prev) => {
        const next = { ...prev }
        for (const id of generatingReportIds) {
          next[id] = (next[id] ?? 0) + 1
        }
        return next
      })
    }, 1000)
    return () => clearInterval(interval)
  }, [generatingReportIds])

  const handleGenerate = async () => {
    setGenLoading(true)
    try {
      await generateReport({ report_type: genType })
      setShowDialog(false)
      fetchReports()
    } catch (err) {
      console.error('[ai-reports] generate failed:', err)
    } finally {
      setGenLoading(false)
    }
  }

  // Count by type
  const countByType = (type: string) => {
    if (!type) return reportsTotal
    return reports.filter((r) => r.report_type === type).length
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteReport(id)
      setDeleteConfirm(null)
      fetchReports()
    } catch (err) {
      console.error('[ai-reports] delete failed:', err)
    }
  }

  const isGenerating = (report: AIReport) =>
    report.status === 'generating' || generatingReportIds.includes(report.id)

  return (
    <div className="flex flex-col gap-5 pb-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h4 className="text-[18px] font-semibold text-[var(--color-on-surface)] mb-0">AI 报告</h4>
        {canWrite && (
          <div className="flex items-center gap-2">
            <button
              onClick={openTemplateEditor}
              className="flex items-center gap-1.5 px-3 py-2 text-[13px] font-medium text-[var(--color-on-surface-variant)] border border-[var(--color-outline-variant)] rounded-lg transition-all hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
            >
              <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>edit_note</span>
              编辑模板
            </button>
            <button
              onClick={() => setShowDialog(true)}
              className="flex items-center gap-1.5 px-4 py-2 text-[13px] font-medium text-white rounded-lg transition-all hover:shadow-lg"
              style={{ background: 'linear-gradient(135deg, #2ca07a, #0ab39c)' }}
            >
              <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>auto_awesome</span>
              生成报告
            </button>
          </div>
        )}
      </div>

      {/* Filter Tabs */}
      <div className="flex items-center gap-1 p-1 bg-[var(--color-surface-container)] rounded-lg w-fit">
        {FILTER_TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setFilter(tab.key)}
            className={`px-3 py-1.5 text-[12px] font-medium rounded-md transition-all ${
              filter === tab.key
                ? 'bg-white text-[var(--color-primary)] shadow-sm'
                : 'text-[var(--color-on-surface-variant)] hover:text-[var(--color-on-surface)]'
            }`}
          >
            {tab.label}
            <span className="ml-1 text-[11px] opacity-60">{countByType(tab.key)}</span>
          </button>
        ))}
      </div>

      {/* Report Cards */}
      {reports.length === 0 ? (
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-16 text-center">
          <div className="w-14 h-14 rounded-full bg-[rgba(44,160,122,0.1)] flex items-center justify-center mx-auto mb-4">
            <span className="material-symbols-outlined text-[var(--color-primary)] text-3xl">description</span>
          </div>
          <p className="text-[var(--color-on-surface)] text-[15px] mb-1">暂无 AI 报告</p>
          <p className="text-[var(--color-on-surface-variant)] text-[12px]">点击「生成报告」创建第一份 AI 分析报告</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {reports.map((report) => {
            const generating = isGenerating(report)
            return (
              <div
                key={report.id}
                onClick={() => !generating && navigate(`/ai-reports/${report.id}`)}
                className={`group bg-[var(--color-surface)] rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-5 transition-all ${
                  generating ? 'opacity-80' : 'cursor-pointer hover:shadow-[0_4px_12px_rgba(56,65,74,0.12)] hover:-translate-y-0.5'
                }`}
              >
                {/* Card Header */}
                <div className="flex items-start justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium bg-[var(--color-primary-subtle)] text-[var(--color-primary)]">
                      {REPORT_TYPES[report.report_type] ?? report.report_type}
                    </span>
                    <StatusBadge status={generating ? 'generating' : report.status} />
                  </div>
                  <div className="flex items-center gap-1.5">
                    <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium text-[var(--color-on-surface-variant)] bg-[var(--color-surface-container)]">
                      {TRIGGER_LABELS[report.trigger_type] ?? report.trigger_type}
                    </span>
                    {!generating && canWrite && (
                      <button
                        onClick={(e) => { e.stopPropagation(); setDeleteConfirm(report.id) }}
                        className="opacity-0 group-hover:opacity-100 p-0.5 rounded hover:bg-[rgba(240,101,72,0.1)] text-[var(--color-on-surface-variant)] hover:text-[#f06548] transition-all"
                        title="删除报告"
                      >
                        <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>delete</span>
                      </button>
                    )}
                  </div>
                </div>

                {/* Title */}
                <h6 className="text-[14px] font-semibold text-[var(--color-on-surface)] mb-2 line-clamp-1">
                  {report.title || '生成中...'}
                </h6>

                {/* Summary */}
                {generating ? (
                  <div className="flex items-center gap-2 py-4">
                    <svg className="animate-spin h-5 w-5 text-[var(--color-primary)]" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    <span className="text-[12px] text-[var(--color-on-surface-variant)]">
                      生成中... {elapsedMap[report.id] ? `${elapsedMap[report.id]}s` : ''}
                    </span>
                  </div>
                ) : (
                  <p className="text-[12px] text-[var(--color-on-surface-variant)] leading-relaxed line-clamp-3 mb-3">
                    {report.summary || (report.status === 'failed' ? report.error_message : '暂无摘要')}
                  </p>
                )}

                {/* Footer */}
                {!generating && (
                  <div className="flex items-center justify-between pt-3 border-t border-[var(--color-outline-variant)]">
                    <div className="flex items-center gap-3 text-[11px] text-[var(--color-on-surface-variant)]">
                      {report.provider && (
                        <span className="flex items-center gap-0.5">
                          <span className="material-symbols-outlined" style={{ fontSize: '13px' }}>smart_toy</span>
                          {report.model || report.provider}
                        </span>
                      )}
                      {report.token_usage > 0 && (
                        <span className="flex items-center gap-0.5">
                          <span className="material-symbols-outlined" style={{ fontSize: '13px' }}>token</span>
                          {report.token_usage.toLocaleString()}
                        </span>
                      )}
                      {report.generation_time_ms > 0 && (
                        <span className="flex items-center gap-0.5">
                          <span className="material-symbols-outlined" style={{ fontSize: '13px' }}>timer</span>
                          {formatDuration(report.generation_time_ms)}
                        </span>
                      )}
                    </div>
                    <span className="text-[11px] text-[var(--color-on-surface-variant)]">
                      {formatDate(report.created_at)}
                    </span>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* Generate Report Dialog */}
      {showDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm" onClick={() => setShowDialog(false)}>
          <div
            className="bg-white rounded-xl shadow-xl w-full max-w-md mx-4 p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <h5 className="text-[16px] font-semibold text-[var(--color-on-surface)] mb-4 flex items-center gap-2">
              <span className="material-symbols-outlined text-[var(--color-primary)]" style={{ fontSize: '20px' }}>auto_awesome</span>
              生成 AI 报告
            </h5>

            <label className="block text-[12px] font-medium text-[var(--color-on-surface-variant)] mb-1.5">
              报告类型
            </label>
            <select
              value={genType}
              onChange={(e) => setGenType(e.target.value)}
              className="w-full px-3 py-2 text-[13px] bg-[var(--color-surface-variant)] border border-[var(--color-outline-variant)] rounded-lg text-[var(--color-on-surface)] focus:outline-none focus:border-[var(--color-primary)] transition-colors"
            >
              {Object.entries(REPORT_TYPES).map(([key, label]) => (
                <option key={key} value={key}>{label}</option>
              ))}
            </select>

            <div className="flex items-center justify-end gap-3 mt-6">
              <button
                onClick={() => setShowDialog(false)}
                className="px-4 py-2 text-[13px] text-[var(--color-on-surface-variant)] hover:text-[var(--color-on-surface)] transition-colors"
              >
                取消
              </button>
              <button
                onClick={handleGenerate}
                disabled={genLoading}
                className="flex items-center gap-1.5 px-4 py-2 text-[13px] font-medium text-white rounded-lg transition-all hover:shadow-lg disabled:opacity-50"
                style={{ background: 'linear-gradient(135deg, #2ca07a, #0ab39c)' }}
              >
                {genLoading && (
                  <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                  </svg>
                )}
                生成
              </button>
            </div>
          </div>
        </div>
      )}
      {/* Delete Confirm Dialog */}
      {deleteConfirm !== null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm" onClick={() => setDeleteConfirm(null)}>
          <div className="bg-white rounded-xl shadow-xl w-full max-w-sm mx-4 p-6" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center gap-2 mb-3">
              <span className="material-symbols-outlined text-[#f06548]" style={{ fontSize: '20px' }}>warning</span>
              <h5 className="text-[16px] font-semibold text-[var(--color-on-surface)]">确认删除</h5>
            </div>
            <p className="text-[13px] text-[var(--color-on-surface-variant)] mb-5">
              删除后无法恢复，确定要删除这份报告吗？
            </p>
            <div className="flex items-center justify-end gap-3">
              <button onClick={() => setDeleteConfirm(null)} className="px-4 py-2 text-[13px] text-[var(--color-on-surface-variant)] hover:text-[var(--color-on-surface)] transition-colors">
                取消
              </button>
              <button
                onClick={() => handleDelete(deleteConfirm)}
                className="px-4 py-2 text-[13px] font-medium text-white bg-[#f06548] hover:bg-[#d9534f] rounded-lg transition-colors"
              >
                删除
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Template Editor Dialog */}
      {showTemplateEditor && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm" onClick={() => setShowTemplateEditor(false)}>
          <div
            className="bg-white rounded-xl shadow-xl w-full max-w-3xl mx-4 max-h-[85vh] flex flex-col"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Header */}
            <div className="px-6 py-4 border-b border-[var(--color-outline-variant)] flex items-center justify-between shrink-0">
              <div className="flex items-center gap-2">
                <span className="material-symbols-outlined text-[var(--color-primary)]" style={{ fontSize: '20px' }}>edit_note</span>
                <div>
                  <h5 className="text-[16px] font-semibold text-[var(--color-on-surface)]">编辑报告模板</h5>
                  <p className="text-[11px] text-[var(--color-on-surface-variant)]">自定义 AI 提示词模板，末尾会自动附加运维数据</p>
                </div>
              </div>
              <button onClick={() => setShowTemplateEditor(false)} className="p-1 rounded-lg hover:bg-[var(--color-surface-container)] transition-colors">
                <span className="material-symbols-outlined text-[var(--color-on-surface-variant)]" style={{ fontSize: '20px' }}>close</span>
              </button>
            </div>

            {/* Tabs */}
            <div className="px-6 pt-4 shrink-0">
              <div className="flex items-center gap-1 p-1 bg-[var(--color-surface-container)] rounded-lg w-fit">
                {Object.entries(REPORT_TYPES).map(([key, label]) => (
                  <button
                    key={key}
                    onClick={() => { setTemplateTab(key); setTemplateSaved(false) }}
                    className={`px-3 py-1.5 text-[12px] font-medium rounded-md transition-all ${
                      templateTab === key
                        ? 'bg-white text-[var(--color-primary)] shadow-sm'
                        : 'text-[var(--color-on-surface-variant)] hover:text-[var(--color-on-surface)]'
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>

            {/* Editor */}
            <div className="px-6 py-4 flex-1 min-h-0 overflow-y-auto">
              <textarea
                value={templates[templateTab] ?? ''}
                onChange={(e) => setTemplates((prev) => ({ ...prev, [templateTab]: e.target.value }))}
                rows={16}
                className="w-full h-full min-h-[320px] border border-[var(--color-outline-variant)] rounded-lg px-4 py-3 text-[13px] text-[var(--color-on-surface)] focus:outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/15 transition-colors font-mono leading-relaxed resize-y"
              />
            </div>

            {/* Footer */}
            <div className="px-6 py-3 border-t border-[var(--color-outline-variant)] flex items-center justify-between shrink-0">
              <div className="flex items-center gap-3">
                <button
                  onClick={() => setTemplates((prev) => ({ ...prev, [templateTab]: defaults[templateTab] ?? '' }))}
                  className="text-[12px] text-[var(--color-on-surface-variant)] hover:text-[var(--color-primary)] transition-colors"
                >
                  恢复默认
                </button>
                {templateSaved && (
                  <span className="text-[12px] text-[#0ab39c] flex items-center gap-1">
                    <span className="material-symbols-outlined" style={{ fontSize: 14 }}>check_circle</span>
                    已保存
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setShowTemplateEditor(false)}
                  className="px-4 py-2 text-[13px] text-[var(--color-on-surface-variant)] hover:text-[var(--color-on-surface)] transition-colors"
                >
                  关闭
                </button>
                <button
                  disabled={templateSaving}
                  onClick={handleTemplateSave}
                  className="flex items-center gap-1.5 px-4 py-2 text-[13px] font-medium text-white rounded-lg transition-all hover:shadow-lg disabled:opacity-50"
                  style={{ background: 'linear-gradient(135deg, #2ca07a, #0ab39c)' }}
                >
                  <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>save</span>
                  {templateSaving ? '保存中...' : '保存模板'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
