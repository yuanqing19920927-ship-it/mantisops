import { useServerStore } from '../../stores/serverStore'
import { useSettingsStore } from '../../stores/settingsStore'
import { useEffect, useState, useCallback } from 'react'
import { StatusBadge } from '../../components/StatusBadge'
import { timeSince } from '../../utils/format'
import { AddServerDialog } from '../../components/AddServerDialog'
import { AddCloudAccountDialog } from '../../components/AddCloudAccountDialog'
import {
  getManagedServers,
  getCloudAccounts,
  deleteManagedServer,
  retryDeploy,
  deployAgent,
  syncCloudAccount,
  deleteCloudAccount,
} from '../../api/onboarding'
import type { ManagedServer, CloudAccount } from '../../types/onboarding'
import { INSTALL_STATE_LABELS, SYNC_STATE_LABELS } from '../../types/onboarding'

export default function Settings() {
  const { servers, fetchDashboard } = useServerStore()
  const { platformName, platformSubtitle, logoUrl, setPlatformName, setPlatformSubtitle, setLogoUrl } = useSettingsStore()
  useEffect(() => { fetchDashboard() }, [fetchDashboard])

  const onlineCount = servers.filter((s) => s.status === 'online').length

  // Platform settings editing
  const [editName, setEditName] = useState(platformName)
  const [editSubtitle, setEditSubtitle] = useState(platformSubtitle)
  const [editLogo, setEditLogo] = useState(logoUrl)
  const [settingsSaved, setSettingsSaved] = useState(false)

  const handleSaveSettings = () => {
    setPlatformName(editName.trim() || 'MantisOps')
    setPlatformSubtitle(editSubtitle.trim() || '运维监控管理平台')
    setLogoUrl(editLogo.trim() || '/logo.svg')
    setSettingsSaved(true)
    setTimeout(() => setSettingsSaved(false), 2000)
  }

  const [showAddServer, setShowAddServer] = useState(false)
  const [showAddCloud, setShowAddCloud] = useState(false)
  const [managedServers, setManagedServers] = useState<ManagedServer[]>([])
  const [cloudAccounts, setCloudAccounts] = useState<CloudAccount[]>([])
  const [syncingId, setSyncingId] = useState<number | null>(null)

  const fetchManaged = useCallback(() => {
    getManagedServers().then(setManagedServers).catch((err) => console.error('[settings] fetch managed:', err))
  }, [])

  const fetchCloud = useCallback(() => {
    getCloudAccounts().then(setCloudAccounts).catch((err) => console.error('[settings] fetch cloud:', err))
  }, [])

  useEffect(() => {
    fetchManaged()
    fetchCloud()
    const timer = setInterval(() => {
      fetchManaged()
      fetchCloud()
    }, 15000)
    return () => clearInterval(timer)
  }, [fetchManaged, fetchCloud])

  const handleDeleteManaged = async (id: number) => {
    await deleteManagedServer(id)
    fetchManaged()
  }

  const handleRetryDeploy = async (id: number) => {
    await retryDeploy(id)
    fetchManaged()
  }

  const handleDeployAgent = async (id: number) => {
    await deployAgent(id)
    fetchManaged()
  }

  const handleSyncCloud = async (id: number) => {
    setSyncingId(id)
    try {
      await syncCloudAccount(id)
      fetchCloud()
    } finally {
      setSyncingId(null)
    }
  }

  const handleDeleteCloud = async (id: number) => {
    await deleteCloudAccount(id, false)
    fetchCloud()
  }

  const PROVIDER_LABELS: Record<string, string> = { aliyun: '阿里云' }

  return (
    <div className="flex flex-col gap-5">

      {/* Dialogs */}
      <AddServerDialog
        open={showAddServer}
        onClose={() => setShowAddServer(false)}
        onSuccess={() => { fetchDashboard(); fetchManaged() }}
      />
      <AddCloudAccountDialog
        open={showAddCloud}
        onClose={() => setShowAddCloud(false)}
        onSuccess={fetchCloud}
      />

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

      {/* Platform Settings */}
      <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
        <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
            <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">tune</span>
          </div>
          <h2 className="text-base font-semibold text-[#495057]">平台配置</h2>
        </div>
        <div className="p-5 grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">平台名称</label>
            <input
              type="text"
              value={editName}
              onChange={(e) => setEditName(e.target.value)}
              placeholder="MantisOps"
              className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
            />
          </div>
          <div>
            <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">平台副标题</label>
            <input
              type="text"
              value={editSubtitle}
              onChange={(e) => setEditSubtitle(e.target.value)}
              placeholder="运维监控管理平台"
              className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">Logo URL</label>
            <div className="flex items-center gap-3">
              <img src={editLogo || '/logo.svg'} alt="Preview" className="w-8 h-8 rounded object-contain bg-[#f8f9fa] p-1 flex-shrink-0" />
              <input
                type="text"
                value={editLogo}
                onChange={(e) => setEditLogo(e.target.value)}
                placeholder="/logo.svg"
                className="flex-1 border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white font-mono"
              />
            </div>
          </div>
          <div className="sm:col-span-2 flex items-center gap-3">
            <button
              onClick={handleSaveSettings}
              className="flex items-center gap-1.5 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors"
            >
              <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>save</span>
              保存配置
            </button>
            {settingsSaved && (
              <span className="text-[12px] text-[#0ab39c] flex items-center gap-1">
                <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>check_circle</span>
                已保存
              </span>
            )}
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
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">最后心跳</th>
                <th className="text-center text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">状态</th>
              </tr>
            </thead>
            <tbody>
              {servers.map((s, idx) => (
                <tr
                  key={s.host_id}
                  className={`hover:bg-[#f8f9fa] transition-colors ${idx < servers.length - 1 ? 'border-b border-[#f2f4f7]' : ''}`}
                >
                  <td className="px-5 py-3.5">
                    <span className="text-[#495057] font-medium text-sm">{s.hostname}</span>
                  </td>
                  <td className="hidden sm:table-cell px-5 py-3.5">
                    <span className="text-[11px] font-mono bg-[#f3f6f9] text-[#495057] px-2 py-0.5 rounded">
                      {s.host_id}
                    </span>
                  </td>
                  <td className="hidden sm:table-cell px-5 py-3.5">
                    <span className="text-[12px] text-[#878a99]">{s.agent_version || '-'}</span>
                  </td>
                  <td className="px-5 py-3.5">
                    <span className="text-[12px] text-[#878a99]">{s.last_seen ? timeSince(s.last_seen) : '-'}</span>
                  </td>
                  <td className="px-5 py-3.5 text-center">
                    <StatusBadge status={s.status} label={s.status === 'online' ? '在线' : '离线'} />
                  </td>
                </tr>
              ))}
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

      {/* ── 接入管理 ── */}
      <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
        <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
            <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">add_circle</span>
          </div>
          <h2 className="text-base font-semibold text-[#495057]">接入管理</h2>
        </div>
        <div className="p-5 grid grid-cols-1 sm:grid-cols-2 gap-4">
          {/* 本地服务器 */}
          <div className="border border-[#e9ebec] rounded-[8px] p-4 flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <span className="material-symbols-outlined text-[#495057] text-[20px]">computer</span>
              <span className="text-[14px] font-semibold text-[#495057]">本地服务器</span>
            </div>
            <p className="text-[12px] text-[#878a99] leading-relaxed">
              通过 SSH 远程安装 Agent 到目标机器，安装完成后自动上报监控数据。
            </p>
            <button
              onClick={() => setShowAddServer(true)}
              className="mt-auto flex items-center justify-center gap-1.5 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors"
            >
              <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>add</span>
              添加服务器
            </button>
          </div>

          {/* 云账号 */}
          <div className="border border-[#e9ebec] rounded-[8px] p-4 flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <span className="material-symbols-outlined text-[#495057] text-[20px]">cloud</span>
              <span className="text-[14px] font-semibold text-[#495057]">云账号</span>
            </div>
            <p className="text-[12px] text-[#878a99] leading-relaxed">
              接入阿里云 ECS / RDS，自动发现并监控云上实例，无需手动添加。
            </p>
            <button
              onClick={() => setShowAddCloud(true)}
              className="mt-auto flex items-center justify-center gap-1.5 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors"
            >
              <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>add</span>
              添加云账号
            </button>
          </div>
        </div>
      </div>

      {/* ── 托管服务器 ── */}
      <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
        <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
            <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">dns</span>
          </div>
          <h2 className="text-base font-semibold text-[#495057]">托管服务器</h2>
          <span className="text-[11px] px-2 py-0.5 rounded-full bg-[#2ca07a]/10 text-[#2ca07a] font-semibold">
            {managedServers.length}
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-[#f8f9fa]">
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">主机</th>
                <th className="hidden sm:table-cell text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">用户</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">安装状态</th>
                <th className="text-right text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">操作</th>
              </tr>
            </thead>
            <tbody>
              {managedServers.map((m, idx) => {
                const isFailed = m.install_state === 'failed'
                const isOnline = m.install_state === 'online'
                const isPending = !isFailed && !isOnline
                return (
                  <tr
                    key={m.id}
                    className={`hover:bg-[#f8f9fa] transition-colors ${idx < managedServers.length - 1 ? 'border-b border-[#f2f4f7]' : ''}`}
                  >
                    <td className="px-5 py-3.5">
                      <span className="text-[#495057] font-medium text-sm font-mono">{m.host}</span>
                      <span className="text-[11px] text-[#878a99] ml-1.5">:{m.ssh_port}</span>
                    </td>
                    <td className="hidden sm:table-cell px-5 py-3.5">
                      <span className="text-[12px] text-[#878a99]">{m.ssh_user}</span>
                    </td>
                    <td className="px-5 py-3.5">
                      <div className="flex items-center gap-2">
                        {isPending && (
                          <span className="w-3 h-3 border border-[#f7b84b]/40 border-t-[#f7b84b] rounded-full animate-spin flex-shrink-0" />
                        )}
                        <span
                          className="text-[11px] py-0.5 px-2 rounded font-medium"
                          style={{
                            backgroundColor: isOnline
                              ? 'rgba(10,179,156,0.08)'
                              : isFailed
                                ? 'rgba(240,101,72,0.08)'
                                : 'rgba(247,184,75,0.08)',
                            color: isOnline ? '#0ab39c' : isFailed ? '#f06548' : '#f7b84b',
                          }}
                        >
                          {INSTALL_STATE_LABELS[m.install_state] ?? m.install_state}
                        </span>
                        {isFailed && m.install_error && (
                          <span
                            className="text-[11px] text-[#f06548] truncate max-w-[160px]"
                            title={m.install_error}
                          >
                            {m.install_error}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex items-center justify-end gap-2">
                        {isOnline && (
                          <button
                            onClick={() => handleDeployAgent(m.id)}
                            className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded transition-colors"
                          >
                            重新部署
                          </button>
                        )}
                        {isFailed && (
                          <button
                            onClick={() => handleRetryDeploy(m.id)}
                            className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded transition-colors"
                          >
                            重试
                          </button>
                        )}
                        <button
                          onClick={() => handleDeleteManaged(m.id)}
                          className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#f06548] hover:text-[#f06548] rounded transition-colors"
                        >
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>

          {managedServers.length === 0 && (
            <div className="py-10 text-center">
              <span className="material-symbols-outlined text-3xl text-[#ced4da] mb-2 block">computer</span>
              <p className="text-[#878a99] text-sm">暂无托管服务器</p>
            </div>
          )}
        </div>
      </div>

      {/* ── 云账号 ── */}
      <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
        <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
            <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">cloud</span>
          </div>
          <h2 className="text-base font-semibold text-[#495057]">云账号</h2>
          <span className="text-[11px] px-2 py-0.5 rounded-full bg-[#2ca07a]/10 text-[#2ca07a] font-semibold">
            {cloudAccounts.length}
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-[#f8f9fa]">
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">名称</th>
                <th className="hidden sm:table-cell text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">提供商</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">同步状态</th>
                <th className="hidden sm:table-cell text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">上次同步</th>
                <th className="text-right text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">操作</th>
              </tr>
            </thead>
            <tbody>
              {cloudAccounts.map((a, idx) => {
                const isSynced = a.sync_state === 'synced'
                const isFailed = a.sync_state === 'failed'
                const isSyncing = a.sync_state === 'syncing'
                return (
                  <tr
                    key={a.id}
                    className={`hover:bg-[#f8f9fa] transition-colors ${idx < cloudAccounts.length - 1 ? 'border-b border-[#f2f4f7]' : ''}`}
                  >
                    <td className="px-5 py-3.5">
                      <span className="text-[#495057] font-medium text-sm">{a.name}</span>
                    </td>
                    <td className="hidden sm:table-cell px-5 py-3.5">
                      <span className="text-[12px] text-[#878a99]">{PROVIDER_LABELS[a.provider] ?? a.provider}</span>
                    </td>
                    <td className="px-5 py-3.5">
                      <div className="flex items-center gap-2">
                        {isSyncing && (
                          <span className="w-3 h-3 border border-[#2ca07a]/40 border-t-[#2ca07a] rounded-full animate-spin flex-shrink-0" />
                        )}
                        <span
                          className="text-[11px] py-0.5 px-2 rounded font-medium"
                          style={{
                            backgroundColor: isSynced
                              ? 'rgba(10,179,156,0.08)'
                              : isFailed
                                ? 'rgba(240,101,72,0.08)'
                                : 'rgba(247,184,75,0.08)',
                            color: isSynced ? '#0ab39c' : isFailed ? '#f06548' : '#f7b84b',
                          }}
                        >
                          {SYNC_STATE_LABELS[a.sync_state] ?? a.sync_state}
                        </span>
                        {isFailed && a.sync_error && (
                          <span
                            className="text-[11px] text-[#f06548] truncate max-w-[160px]"
                            title={a.sync_error}
                          >
                            {a.sync_error}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="hidden sm:table-cell px-5 py-3.5">
                      <span className="text-[12px] text-[#878a99]">
                        {a.last_synced_at ? timeSince(Math.floor(new Date(a.last_synced_at).getTime() / 1000)) : '-'}
                      </span>
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => handleSyncCloud(a.id)}
                          disabled={syncingId === a.id}
                          className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                          同步
                        </button>
                        <button
                          onClick={() => handleDeleteCloud(a.id)}
                          className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#f06548] hover:text-[#f06548] rounded transition-colors"
                        >
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>

          {cloudAccounts.length === 0 && (
            <div className="py-10 text-center">
              <span className="material-symbols-outlined text-3xl text-[#ced4da] mb-2 block">cloud_off</span>
              <p className="text-[#878a99] text-sm">暂无云账号</p>
            </div>
          )}
        </div>
      </div>

    </div>
  )
}
