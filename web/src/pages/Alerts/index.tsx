import { useEffect, useState, useCallback } from 'react'
import {
  getAlertRules, createAlertRule, updateAlertRule, deleteAlertRule,
  getAlertEvents, getAlertStats, ackAlertEvent, getEventNotifications,
  getAlertChannels, createAlertChannel, updateAlertChannel, deleteAlertChannel, testAlertChannel,
} from '../../api/alert'
import type { AlertRule, AlertEvent, AlertStats, NotificationChannel, AlertNotificationDetail } from '../../types'

// ── Constants ──────────────────────────────────────────────

const RULE_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: 'server_offline', label: '服务器离线' },
  { value: 'probe_down', label: '端口异常' },
  { value: 'cpu', label: 'CPU 使用率' },
  { value: 'memory', label: '内存使用率' },
  { value: 'disk', label: '磁盘使用率' },
  { value: 'container', label: '容器异常' },
  { value: 'gpu_temp', label: 'GPU 温度' },
  { value: 'gpu_memory', label: 'GPU 显存' },
  { value: 'network_rx', label: '网络入站' },
  { value: 'network_tx', label: '网络出站' },
]

const OPERATOR_OPTIONS = ['>', '>=', '<', '<=', '==', '!=']

const LEVEL_OPTIONS: { value: string; label: string; color: string }[] = [
  { value: 'critical', label: '严重', color: 'text-error' },
  { value: 'warning', label: '警告', color: 'text-warning' },
  { value: 'info', label: '通知', color: 'text-primary' },
]

const RESOLVE_TYPE_LABELS: Record<string, string> = {
  auto: '自动恢复',
  target_gone: '目标消失',
  rule_disabled: '规则禁用',
  rule_deleted: '规则删除',
}

const LEVEL_EMOJI: Record<string, string> = {
  critical: '🔴',
  warning: '🟡',
  info: '🔵',
}

const CHANNEL_TYPE_OPTIONS = [
  { value: 'dingtalk', label: '钉钉机器人' },
  { value: 'webhook', label: 'Webhook' },
]

// ── Helper functions ───────────────────────────────────────

function formatTime(iso?: string): string {
  if (!iso) return '--'
  const d = new Date(iso)
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function ruleTypeLabel(type: string): string {
  return RULE_TYPE_OPTIONS.find((o) => o.value === type)?.label ?? type
}

function levelColor(level: string): string {
  return LEVEL_OPTIONS.find((o) => o.value === level)?.color ?? 'text-on-surface'
}

function maskUrl(url: string): string {
  try {
    const u = new URL(url)
    return `${u.protocol}//${u.host}/***`
  } catch {
    return url.length > 30 ? url.slice(0, 30) + '...' : url
  }
}

// ── Shared form styles ─────────────────────────────────────

const inputClass =
  'bg-surface-container-lowest border-none rounded-lg px-4 py-2 text-sm focus:ring-1 focus:ring-primary/30 text-on-surface placeholder:text-on-surface-variant outline-none'
const selectClass =
  'bg-surface-container-lowest border-none rounded-lg px-4 py-2 text-sm focus:ring-1 focus:ring-primary/30 text-on-surface outline-none appearance-none'

// ── Component ──────────────────────────────────────────────

export default function Alerts() {
  // Tab state
  const [tab, setTab] = useState<'events' | 'rules' | 'channels'>('events')

  // Stats
  const [stats, setStats] = useState<AlertStats | null>(null)

  // Events state
  const [events, setEvents] = useState<AlertEvent[]>([])
  const [eventFilter, setEventFilter] = useState('all')

  // Rules state
  const [rules, setRules] = useState<AlertRule[]>([])
  const [showRuleForm, setShowRuleForm] = useState(false)
  const [ruleForm, setRuleForm] = useState({
    name: '', type: 'cpu', target_id: '', operator: '>', threshold: 90, unit: '%', duration: 60, level: 'warning',
  })

  // Channels state
  const [channels, setChannels] = useState<NotificationChannel[]>([])
  const [showChannelForm, setShowChannelForm] = useState(false)
  const [channelForm, setChannelForm] = useState({ name: '', type: 'dingtalk', url: '', secret: '' })

  // Expanded event for notification details
  const [expandedEvent, setExpandedEvent] = useState<number | null>(null)
  const [notifications, setNotifications] = useState<AlertNotificationDetail[]>([])

  // Toast
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null)

  const showToast = (msg: string, ok: boolean) => {
    setToast({ msg, ok })
    setTimeout(() => setToast(null), 3000)
  }

  // ── Data loading ───────────────────────────────────────

  const loadStats = useCallback(async () => {
    try { setStats(await getAlertStats()) } catch { /* ignore */ }
  }, [])

  const loadEvents = useCallback(async () => {
    try {
      const params: Record<string, string> = {}
      if (eventFilter === 'firing') params.status = 'firing'
      else if (eventFilter === 'resolved') params.status = 'resolved'
      else if (eventFilter === 'silenced') params.silenced = 'true'
      setEvents(await getAlertEvents(params))
    } catch { /* ignore */ }
  }, [eventFilter])

  const loadRules = useCallback(async () => {
    try { setRules(await getAlertRules()) } catch { /* ignore */ }
  }, [])

  const loadChannels = useCallback(async () => {
    try { setChannels(await getAlertChannels()) } catch { /* ignore */ }
  }, [])

  useEffect(() => { loadStats() }, [loadStats])

  useEffect(() => {
    if (tab === 'events') loadEvents()
    else if (tab === 'rules') loadRules()
    else if (tab === 'channels') loadChannels()
  }, [tab, loadEvents, loadRules, loadChannels])

  // Auto-refresh events every 15s
  useEffect(() => {
    if (tab !== 'events') return
    const timer = setInterval(() => { loadEvents(); loadStats() }, 15000)
    return () => clearInterval(timer)
  }, [tab, loadEvents, loadStats])

  // ── Event handlers ─────────────────────────────────────

  const handleAck = async (id: number) => {
    try {
      await ackAlertEvent(id)
      loadEvents()
      loadStats()
    } catch {
      showToast('确认失败', false)
    }
  }

  const handleExpandEvent = async (id: number) => {
    if (expandedEvent === id) {
      setExpandedEvent(null)
      return
    }
    setExpandedEvent(id)
    try {
      setNotifications(await getEventNotifications(id))
    } catch {
      setNotifications([])
    }
  }

  // ── Rule handlers ──────────────────────────────────────

  const handleCreateRule = async () => {
    if (!ruleForm.name) return
    try {
      await createAlertRule({
        name: ruleForm.name,
        type: ruleForm.type,
        target_id: ruleForm.target_id,
        operator: ruleForm.operator,
        threshold: Number(ruleForm.threshold),
        unit: ruleForm.unit,
        duration: Number(ruleForm.duration),
        level: ruleForm.level,
        enabled: true,
      })
      setRuleForm({ name: '', type: 'cpu', target_id: '', operator: '>', threshold: 90, unit: '%', duration: 60, level: 'warning' })
      setShowRuleForm(false)
      loadRules()
      showToast('规则创建成功', true)
    } catch {
      showToast('创建失败', false)
    }
  }

  const handleToggleRule = async (rule: AlertRule) => {
    try {
      await updateAlertRule(rule.id!, { enabled: !rule.enabled })
      loadRules()
    } catch {
      showToast('更新失败', false)
    }
  }

  const handleDeleteRule = async (id: number) => {
    if (!window.confirm('确定要删除此告警规则吗？')) return
    try {
      await deleteAlertRule(id)
      loadRules()
      showToast('规则已删除', true)
    } catch {
      showToast('删除失败', false)
    }
  }

  // ── Channel handlers ───────────────────────────────────

  const handleCreateChannel = async () => {
    if (!channelForm.name || !channelForm.url) return
    const config: Record<string, string> = { url: channelForm.url }
    if (channelForm.type === 'dingtalk' && channelForm.secret) config.secret = channelForm.secret
    try {
      await createAlertChannel({
        name: channelForm.name,
        type: channelForm.type,
        config: JSON.stringify(config),
        enabled: true,
      })
      setChannelForm({ name: '', type: 'dingtalk', url: '', secret: '' })
      setShowChannelForm(false)
      loadChannels()
      showToast('渠道创建成功', true)
    } catch {
      showToast('创建失败', false)
    }
  }

  const handleToggleChannel = async (ch: NotificationChannel) => {
    try {
      await updateAlertChannel(ch.id!, { enabled: !ch.enabled })
      loadChannels()
    } catch {
      showToast('更新失败', false)
    }
  }

  const handleDeleteChannel = async (id: number) => {
    if (!window.confirm('确定要删除此通知渠道吗？')) return
    try {
      await deleteAlertChannel(id)
      loadChannels()
      showToast('渠道已删除', true)
    } catch {
      showToast('删除失败', false)
    }
  }

  const handleTestChannel = async (id: number) => {
    try {
      await testAlertChannel(id)
      showToast('测试消息已发送', true)
    } catch {
      showToast('测试失败，请检查配置', false)
    }
  }

  const parseChannelConfig = (configStr: string): Record<string, string> => {
    try { return JSON.parse(configStr) } catch { return {} }
  }

  // ── Render ─────────────────────────────────────────────

  return (
    <div>
      {/* Toast */}
      {toast && (
        <div className={`fixed top-6 right-6 z-50 px-5 py-3 rounded-xl text-sm font-medium shadow-xl transition-all ${
          toast.ok ? 'bg-tertiary/90 text-on-tertiary' : 'bg-error/90 text-on-error'
        }`}>
          {toast.msg}
        </div>
      )}

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
        <div>
          <h1 className="font-headline text-2xl font-bold text-on-surface">告警中心</h1>
          <p className="text-sm text-on-surface-variant mt-1">告警事件监控、规则管理与通知渠道配置</p>
        </div>
      </div>

      {/* Stats Row */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <div className="bg-surface-container p-4 sm:p-6 rounded-xl border-l-4 border-error">
          <div className="flex items-center gap-3 mb-2">
            <span className="material-symbols-outlined text-error text-xl">warning</span>
            <span className="text-sm text-on-surface-variant">当前触发中</span>
          </div>
          <div className="text-2xl font-bold text-error">{stats?.firing ?? 0}</div>
        </div>

        <div className="bg-surface-container p-4 sm:p-6 rounded-xl border-l-4 border-warning">
          <div className="flex items-center gap-3 mb-2">
            <span className="material-symbols-outlined text-warning text-xl">notification_add</span>
            <span className="text-sm text-on-surface-variant">今日触发</span>
          </div>
          <div className="text-2xl font-bold text-warning">{stats?.today_fired ?? 0}</div>
        </div>

        <div className="bg-surface-container p-4 sm:p-6 rounded-xl border-l-4 border-tertiary">
          <div className="flex items-center gap-3 mb-2">
            <span className="material-symbols-outlined text-tertiary text-xl">check_circle</span>
            <span className="text-sm text-on-surface-variant">今日恢复</span>
          </div>
          <div className="text-2xl font-bold text-tertiary">{stats?.today_resolved ?? 0}</div>
        </div>

        <div className="bg-surface-container p-4 sm:p-6 rounded-xl border-l-4 border-primary">
          <div className="flex items-center gap-3 mb-2">
            <span className="material-symbols-outlined text-primary text-xl">done_all</span>
            <span className="text-sm text-on-surface-variant">今日确认</span>
          </div>
          <div className="text-2xl font-bold text-primary">{stats?.today_silenced ?? 0}</div>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-1 mb-6 bg-surface-container rounded-xl p-1">
        {([
          { key: 'events' as const, label: '告警事件', icon: 'notifications' },
          { key: 'rules' as const, label: '告警规则', icon: 'rule' },
          { key: 'channels' as const, label: '通知渠道', icon: 'send' },
        ]).map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={`flex-1 flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg text-sm font-medium transition-all ${
              tab === t.key
                ? 'bg-primary/20 text-primary shadow-sm'
                : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'
            }`}
          >
            <span className="material-symbols-outlined text-lg">{t.icon}</span>
            {t.label}
          </button>
        ))}
      </div>

      {/* ━━━ Tab 1: 告警事件 ━━━ */}
      {tab === 'events' && (
        <div>
          {/* Filter buttons */}
          <div className="flex items-center gap-2 mb-6 flex-wrap">
            {([
              { key: 'all', label: '全部' },
              { key: 'firing', label: '触发中' },
              { key: 'resolved', label: '已恢复' },
              { key: 'silenced', label: '已静默' },
            ]).map((f) => (
              <button
                key={f.key}
                onClick={() => setEventFilter(f.key)}
                className={`px-4 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                  eventFilter === f.key
                    ? 'bg-primary/20 text-primary'
                    : 'text-on-surface-variant hover:bg-surface-container-high'
                }`}
              >
                {f.label}
              </button>
            ))}
          </div>

          {/* Events table */}
          <div className="bg-surface-container rounded-xl overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-outline-variant/10">
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium">级别</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium">告警名称</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium hidden md:table-cell">目标</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium hidden lg:table-cell">触发值</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium">状态</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium hidden sm:table-cell">触发时间</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {events.map((ev) => {
                    const isFiring = ev.status === 'firing'
                    const isSilenced = ev.silenced
                    const isExpanded = expandedEvent === ev.id

                    let rowClass = ''
                    if (isFiring && !isSilenced) rowClass = 'border-l-4 border-l-error bg-error/5'
                    else if (isFiring && isSilenced) rowClass = 'border-l-4 border-l-warning bg-warning/5'

                    return (
                      <tr key={ev.id} className="group">
                        <td colSpan={7} className="p-0">
                          <div
                            className={`flex flex-col ${rowClass} cursor-pointer hover:bg-surface-container-high/50 transition-colors`}
                            onClick={() => handleExpandEvent(ev.id)}
                          >
                            {/* Main row */}
                            <div className="flex items-center">
                              <div className="px-4 py-3 w-14 flex-shrink-0">{LEVEL_EMOJI[ev.level] ?? '⚪'}</div>
                              <div className="px-4 py-3 flex-1 min-w-0">
                                <span className="text-on-surface font-medium truncate block">{ev.rule_name}</span>
                              </div>
                              <div className="px-4 py-3 flex-1 min-w-0 hidden md:block">
                                <span className="text-on-surface-variant text-xs truncate block">{ev.target_label}</span>
                              </div>
                              <div className="px-4 py-3 w-24 hidden lg:block">
                                <span className="text-on-surface-variant">{ev.value}</span>
                              </div>
                              <div className="px-4 py-3 w-28">
                                {isFiring && !isSilenced && (
                                  <span className="text-xs px-2 py-0.5 rounded bg-error/20 text-error font-medium">触发中</span>
                                )}
                                {isFiring && isSilenced && (
                                  <span className="text-xs px-2 py-0.5 rounded bg-warning/20 text-warning font-medium">已确认</span>
                                )}
                                {ev.status === 'resolved' && (
                                  <span className="text-xs px-2 py-0.5 rounded bg-tertiary/20 text-tertiary font-medium">已恢复</span>
                                )}
                              </div>
                              <div className="px-4 py-3 w-36 hidden sm:block">
                                <span className="text-on-surface-variant text-xs">{formatTime(ev.fired_at)}</span>
                              </div>
                              <div className="px-4 py-3 w-24">
                                {isFiring && !isSilenced && (
                                  <button
                                    onClick={(e) => { e.stopPropagation(); handleAck(ev.id) }}
                                    className="text-xs px-3 py-1 rounded-lg bg-warning/20 text-warning hover:bg-warning/30 font-medium transition-colors"
                                  >
                                    确认
                                  </button>
                                )}
                              </div>
                            </div>

                            {/* Expanded: extra info */}
                            {isFiring && isSilenced && (
                              <div className="px-4 pb-2 text-xs text-on-surface-variant">
                                确认人: {ev.acked_by ?? '--'} | 确认时间: {formatTime(ev.acked_at)}
                              </div>
                            )}
                            {ev.status === 'resolved' && (
                              <div className="px-4 pb-2 text-xs text-on-surface-variant">
                                恢复时间: {formatTime(ev.resolved_at)} | 恢复方式: {RESOLVE_TYPE_LABELS[ev.resolve_type ?? ''] ?? ev.resolve_type ?? '--'}
                              </div>
                            )}

                            {/* Notification details */}
                            {isExpanded && (
                              <div className="px-4 pb-4">
                                <div className="bg-surface-container-lowest rounded-lg p-3 mt-1">
                                  <div className="text-xs font-medium text-on-surface-variant mb-2">通知详情</div>
                                  {notifications.length === 0 ? (
                                    <div className="text-xs text-on-surface-variant/60">暂无通知记录</div>
                                  ) : (
                                    <div className="space-y-1">
                                      {notifications.map((n, i) => (
                                        <div key={i} className="flex items-center gap-3 text-xs">
                                          <span className="text-on-surface-variant">{n.channel_name}</span>
                                          <span className="text-on-surface-variant/60">{n.channel_type}</span>
                                          <span className={n.status === 'sent' ? 'text-tertiary' : 'text-error'}>
                                            {n.status === 'sent' ? '已发送' : n.status === 'failed' ? '发送失败' : n.status}
                                          </span>
                                          {n.last_error && <span className="text-error/60 truncate">{n.last_error}</span>}
                                          <span className="text-on-surface-variant/40 ml-auto">{formatTime(n.sent_at)}</span>
                                        </div>
                                      ))}
                                    </div>
                                  )}
                                </div>
                              </div>
                            )}
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {events.length === 0 && (
              <div className="text-center py-16">
                <span className="material-symbols-outlined text-4xl text-on-surface-variant/30 mb-3 block">notifications_off</span>
                <p className="text-on-surface-variant">暂无告警事件</p>
              </div>
            )}
          </div>
        </div>
      )}

      {/* ━━━ Tab 2: 告警规则 ━━━ */}
      {tab === 'rules' && (
        <div>
          {/* Header with add button */}
          <div className="flex items-center justify-between mb-6">
            <div className="flex items-center gap-2">
              <div className="w-1 h-6 bg-primary rounded-full" />
              <h2 className="font-headline text-xl font-bold text-on-surface">告警规则列表</h2>
            </div>
            <button
              onClick={() => setShowRuleForm(!showRuleForm)}
              className="bg-gradient-to-br from-primary to-primary-container text-on-primary-container px-5 py-2 rounded-lg font-medium shadow-lg shadow-primary/20 flex items-center gap-2 hover:opacity-90 transition-opacity text-sm"
            >
              <span className="material-symbols-outlined text-lg">add_circle</span>
              添加规则
            </button>
          </div>

          {/* Rule form */}
          {showRuleForm && (
            <div className="bg-surface-container rounded-xl p-6 mb-6 border border-outline-variant/10">
              <div className="flex items-center gap-2 mb-4">
                <div className="w-1 h-6 bg-primary rounded-full" />
                <h3 className="font-headline text-lg font-bold text-on-surface">新建告警规则</h3>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-4">
                <input
                  placeholder="规则名称"
                  value={ruleForm.name}
                  onChange={(e) => setRuleForm({ ...ruleForm, name: e.target.value })}
                  className={inputClass}
                />
                <select
                  value={ruleForm.type}
                  onChange={(e) => setRuleForm({ ...ruleForm, type: e.target.value })}
                  className={selectClass}
                >
                  {RULE_TYPE_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
                <input
                  placeholder="目标 ID（可选, host_id 或 probe id）"
                  value={ruleForm.target_id}
                  onChange={(e) => setRuleForm({ ...ruleForm, target_id: e.target.value })}
                  className={inputClass}
                />
                <select
                  value={ruleForm.operator}
                  onChange={(e) => setRuleForm({ ...ruleForm, operator: e.target.value })}
                  className={selectClass}
                >
                  {OPERATOR_OPTIONS.map((o) => (
                    <option key={o} value={o}>{o}</option>
                  ))}
                </select>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <input
                  type="number"
                  placeholder="阈值"
                  value={ruleForm.threshold}
                  onChange={(e) => setRuleForm({ ...ruleForm, threshold: Number(e.target.value) })}
                  className={inputClass}
                />
                <input
                  type="number"
                  placeholder="持续时间（秒）"
                  value={ruleForm.duration}
                  onChange={(e) => setRuleForm({ ...ruleForm, duration: Number(e.target.value) })}
                  className={inputClass}
                />
                <select
                  value={ruleForm.level}
                  onChange={(e) => setRuleForm({ ...ruleForm, level: e.target.value })}
                  className={selectClass}
                >
                  {LEVEL_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
                <div className="flex gap-2">
                  <button
                    onClick={handleCreateRule}
                    className="flex-1 bg-gradient-to-br from-primary to-primary-container text-on-primary-container px-4 py-2 rounded-lg text-sm font-medium shadow-lg shadow-primary/20 hover:opacity-90 transition-opacity"
                  >
                    保存
                  </button>
                  <button
                    onClick={() => setShowRuleForm(false)}
                    className="px-4 py-2 rounded-lg text-sm text-on-surface-variant bg-surface-container-high hover:bg-surface-container transition-colors"
                  >
                    取消
                  </button>
                </div>
              </div>
            </div>
          )}

          {/* Rules table */}
          <div className="bg-surface-container rounded-xl overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-outline-variant/10">
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium">名称</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium">类型</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium hidden md:table-cell">目标</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium hidden sm:table-cell">条件</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium hidden lg:table-cell">持续</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium">级别</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium">启用</th>
                    <th className="text-left px-4 py-3 text-on-surface-variant font-medium">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {rules.map((rule) => (
                    <tr key={rule.id} className="border-b border-outline-variant/5 hover:bg-surface-container-high/50 transition-colors group">
                      <td className="px-4 py-3 text-on-surface font-medium">{rule.name}</td>
                      <td className="px-4 py-3 text-on-surface-variant">{ruleTypeLabel(rule.type)}</td>
                      <td className="px-4 py-3 text-on-surface-variant hidden md:table-cell">
                        <span className="text-xs font-mono">{rule.target_id || '全部'}</span>
                      </td>
                      <td className="px-4 py-3 text-on-surface-variant hidden sm:table-cell">
                        <span className="font-mono text-xs">{rule.operator} {rule.threshold}{rule.unit}</span>
                      </td>
                      <td className="px-4 py-3 text-on-surface-variant hidden lg:table-cell">{rule.duration}s</td>
                      <td className="px-4 py-3">
                        <span className={`text-xs px-2 py-0.5 rounded font-medium ${levelColor(rule.level)} ${
                          rule.level === 'critical' ? 'bg-error/20' : rule.level === 'warning' ? 'bg-warning/20' : 'bg-primary/20'
                        }`}>
                          {LEVEL_OPTIONS.find((o) => o.value === rule.level)?.label ?? rule.level}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <button
                          onClick={() => handleToggleRule(rule)}
                          className={`w-10 h-5 rounded-full transition-colors relative ${
                            rule.enabled ? 'bg-primary' : 'bg-surface-container-highest'
                          }`}
                        >
                          <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform ${
                            rule.enabled ? 'translate-x-5' : 'translate-x-0.5'
                          }`} />
                        </button>
                      </td>
                      <td className="px-4 py-3">
                        <button
                          onClick={() => handleDeleteRule(rule.id!)}
                          className="sm:opacity-0 sm:group-hover:opacity-100 transition-opacity text-error/60 hover:text-error p-1 rounded-lg hover:bg-error/20"
                        >
                          <span className="material-symbols-outlined text-base">delete</span>
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {rules.length === 0 && (
              <div className="text-center py-16">
                <span className="material-symbols-outlined text-4xl text-on-surface-variant/30 mb-3 block">rule</span>
                <p className="text-on-surface-variant">暂无告警规则</p>
                <p className="text-on-surface-variant/60 text-xs mt-1">点击「添加规则」开始配置</p>
              </div>
            )}
          </div>
        </div>
      )}

      {/* ━━━ Tab 3: 通知渠道 ━━━ */}
      {tab === 'channels' && (
        <div>
          {/* Header with add button */}
          <div className="flex items-center justify-between mb-6">
            <div className="flex items-center gap-2">
              <div className="w-1 h-6 bg-primary rounded-full" />
              <h2 className="font-headline text-xl font-bold text-on-surface">通知渠道管理</h2>
            </div>
            <button
              onClick={() => setShowChannelForm(!showChannelForm)}
              className="bg-gradient-to-br from-primary to-primary-container text-on-primary-container px-5 py-2 rounded-lg font-medium shadow-lg shadow-primary/20 flex items-center gap-2 hover:opacity-90 transition-opacity text-sm"
            >
              <span className="material-symbols-outlined text-lg">add_circle</span>
              添加渠道
            </button>
          </div>

          {/* Channel form */}
          {showChannelForm && (
            <div className="bg-surface-container rounded-xl p-6 mb-6 border border-outline-variant/10">
              <div className="flex items-center gap-2 mb-4">
                <div className="w-1 h-6 bg-primary rounded-full" />
                <h3 className="font-headline text-lg font-bold text-on-surface">新建通知渠道</h3>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <input
                  placeholder="渠道名称"
                  value={channelForm.name}
                  onChange={(e) => setChannelForm({ ...channelForm, name: e.target.value })}
                  className={inputClass}
                />
                <select
                  value={channelForm.type}
                  onChange={(e) => setChannelForm({ ...channelForm, type: e.target.value })}
                  className={selectClass}
                >
                  {CHANNEL_TYPE_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
                <input
                  placeholder="Webhook URL"
                  value={channelForm.url}
                  onChange={(e) => setChannelForm({ ...channelForm, url: e.target.value })}
                  className={inputClass}
                />
                {channelForm.type === 'dingtalk' && (
                  <input
                    placeholder="Secret（加签密钥，可选）"
                    value={channelForm.secret}
                    onChange={(e) => setChannelForm({ ...channelForm, secret: e.target.value })}
                    className={inputClass}
                  />
                )}
              </div>
              <div className="flex gap-2 mt-4">
                <button
                  onClick={handleCreateChannel}
                  className="bg-gradient-to-br from-primary to-primary-container text-on-primary-container px-6 py-2 rounded-lg text-sm font-medium shadow-lg shadow-primary/20 hover:opacity-90 transition-opacity"
                >
                  保存
                </button>
                <button
                  onClick={() => setShowChannelForm(false)}
                  className="px-4 py-2 rounded-lg text-sm text-on-surface-variant bg-surface-container-high hover:bg-surface-container transition-colors"
                >
                  取消
                </button>
              </div>
            </div>
          )}

          {/* Channel cards */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {channels.map((ch) => {
              const config = parseChannelConfig(ch.config)
              return (
                <div key={ch.id} className="bg-surface-container-low rounded-xl p-5 hover:bg-surface-container-high transition-colors group flex flex-col">
                  {/* Card header */}
                  <div className="flex items-center justify-between mb-3">
                    <div className="flex items-center gap-3">
                      <span className="material-symbols-outlined text-xl text-primary">
                        {ch.type === 'dingtalk' ? 'chat' : 'webhook'}
                      </span>
                      <span className="font-medium text-on-surface">{ch.name}</span>
                    </div>
                    <button
                      onClick={() => handleDeleteChannel(ch.id!)}
                      className="sm:opacity-0 sm:group-hover:opacity-100 transition-opacity text-error/60 hover:text-error p-1 rounded-lg hover:bg-error/20"
                    >
                      <span className="material-symbols-outlined text-base">delete</span>
                    </button>
                  </div>

                  {/* Type badge */}
                  <div className="mb-3">
                    <span className="text-xs px-2 py-0.5 rounded bg-primary/20 text-primary font-medium">
                      {CHANNEL_TYPE_OPTIONS.find((o) => o.value === ch.type)?.label ?? ch.type}
                    </span>
                  </div>

                  {/* Masked URL */}
                  <div className="bg-surface-container-lowest p-3 rounded-lg mb-4">
                    <div className="text-xs text-on-surface-variant mb-1">Webhook URL</div>
                    <div className="text-xs font-mono text-on-surface truncate">{maskUrl(config.url ?? '')}</div>
                  </div>

                  <div className="flex-1" />

                  {/* Bottom controls */}
                  <div className="flex items-center justify-between mt-2">
                    <button
                      onClick={() => handleToggleChannel(ch)}
                      className={`w-10 h-5 rounded-full transition-colors relative ${
                        ch.enabled ? 'bg-primary' : 'bg-surface-container-highest'
                      }`}
                    >
                      <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform ${
                        ch.enabled ? 'translate-x-5' : 'translate-x-0.5'
                      }`} />
                    </button>
                    <button
                      onClick={() => handleTestChannel(ch.id!)}
                      className="text-xs px-3 py-1.5 rounded-lg bg-tertiary/20 text-tertiary hover:bg-tertiary/30 font-medium transition-colors flex items-center gap-1"
                    >
                      <span className="material-symbols-outlined text-sm">send</span>
                      测试
                    </button>
                  </div>
                </div>
              )
            })}
          </div>

          {channels.length === 0 && !showChannelForm && (
            <div className="text-center py-16">
              <span className="material-symbols-outlined text-4xl text-on-surface-variant/30 mb-3 block">send</span>
              <p className="text-on-surface-variant">暂无通知渠道</p>
              <p className="text-on-surface-variant/60 text-xs mt-1">点击「添加渠道」配置告警通知</p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
