import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { getReport, downloadReport } from '../../api/ai'
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

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export default function ReportDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [report, setReport] = useState<AIReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!id) return
    setLoading(true)
    getReport(Number(id))
      .then((data) => setReport(data))
      .catch((err) => {
        console.error('[report-detail] fetch failed:', err)
        setError(err?.response?.status === 404 ? '报告不存在' : '加载失败，请稍后重试')
      })
      .finally(() => setLoading(false))
  }, [id])

  const handleExport = () => {
    if (!report) return
    downloadReport(report.id, report.title)
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-32">
        <svg className="animate-spin h-8 w-8 text-[var(--color-primary)]" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
      </div>
    )
  }

  if (error || !report) {
    return (
      <div className="flex flex-col items-center justify-center py-32">
        <div className="w-14 h-14 rounded-full bg-[rgba(240,101,72,0.1)] flex items-center justify-center mb-4">
          <span className="material-symbols-outlined text-[#f06548] text-3xl">error</span>
        </div>
        <p className="text-[var(--color-on-surface)] text-[15px] mb-2">{error || '报告不存在'}</p>
        <button
          onClick={() => navigate('/ai-reports')}
          className="text-[13px] text-[var(--color-primary)] hover:underline"
        >
          返回报告列表
        </button>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5 pb-8">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <button
            onClick={() => navigate('/ai-reports')}
            className="mt-0.5 flex items-center gap-1 text-[13px] text-[var(--color-on-surface-variant)] hover:text-[var(--color-primary)] transition-colors"
          >
            <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>arrow_back</span>
            返回
          </button>
          <div>
            <div className="flex items-center gap-2 mb-1">
              <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium bg-[var(--color-primary-subtle)] text-[var(--color-primary)]">
                {REPORT_TYPES[report.report_type] ?? report.report_type}
              </span>
              {report.status === 'failed' && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-[rgba(240,101,72,0.1)] text-[#f06548]">
                  <span className="w-1.5 h-1.5 rounded-full bg-[#f06548]" />
                  失败
                </span>
              )}
            </div>
            <h4 className="text-[18px] font-semibold text-[var(--color-on-surface)]">{report.title}</h4>
          </div>
        </div>
        <button
          onClick={handleExport}
          className="flex items-center gap-1.5 px-4 py-2 text-[13px] font-medium text-white rounded-lg transition-all hover:shadow-lg shrink-0"
          style={{ background: 'linear-gradient(135deg, #2ca07a, #0ab39c)' }}
        >
          <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>download</span>
          导出 Markdown
        </button>
      </div>

      {/* Meta Info Bar */}
      <div className="flex flex-wrap items-center gap-4 px-5 py-3 bg-[var(--color-surface-container)] rounded-[10px] text-[12px] text-[var(--color-on-surface-variant)]">
        {report.provider && (
          <span className="flex items-center gap-1">
            <span className="material-symbols-outlined" style={{ fontSize: '15px' }}>smart_toy</span>
            {report.provider}
          </span>
        )}
        {report.model && (
          <span className="flex items-center gap-1">
            <span className="material-symbols-outlined" style={{ fontSize: '15px' }}>memory</span>
            {report.model}
          </span>
        )}
        {report.token_usage > 0 && (
          <span className="flex items-center gap-1">
            <span className="material-symbols-outlined" style={{ fontSize: '15px' }}>token</span>
            {report.token_usage.toLocaleString()} tokens
          </span>
        )}
        {report.generation_time_ms > 0 && (
          <span className="flex items-center gap-1">
            <span className="material-symbols-outlined" style={{ fontSize: '15px' }}>timer</span>
            {formatDuration(report.generation_time_ms)}
          </span>
        )}
        <span className="flex items-center gap-1">
          <span className="material-symbols-outlined" style={{ fontSize: '15px' }}>schedule</span>
          {TRIGGER_LABELS[report.trigger_type] ?? report.trigger_type}
        </span>
      </div>

      {/* Content */}
      {report.status === 'failed' ? (
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-6">
          <div className="flex items-center gap-2 text-[#f06548] mb-2">
            <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>error</span>
            <span className="text-[14px] font-medium">生成失败</span>
          </div>
          <p className="text-[13px] text-[var(--color-on-surface-variant)]">
            {report.error_message || '未知错误'}
          </p>
        </div>
      ) : (
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-6 md:p-8">
          <div className="ai-report-content max-w-none">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {report.content ?? ''}
            </ReactMarkdown>
          </div>
        </div>
      )}
    </div>
  )
}
