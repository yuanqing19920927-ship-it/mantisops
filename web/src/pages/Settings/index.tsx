import { useServerStore } from '../../stores/serverStore'
import { useSettingsStore } from '../../stores/settingsStore'
import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
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
  getCredentials,
} from '../../api/onboarding'
import type { ManagedServer, CloudAccount, CredentialSummary } from '../../types/onboarding'
import { INSTALL_STATE_LABELS, SYNC_STATE_LABELS } from '../../types/onboarding'
import {
  getNasDevices,
  createNasDevice,
  updateNasDevice,
  deleteNasDevice,
  testNasConnection,
} from '../../api/nas'
import type { NasDevice } from '../../api/nas'
import { useAuthStore } from '../../stores/authStore'

export default function Settings() {
  const isAdmin = useAuthStore((s) => s.role === 'admin')
  const { servers, metrics, fetchDashboard } = useServerStore()
  const { platformName, platformSubtitle, logoUrl, saveSettings } = useSettingsStore()
  useEffect(() => { fetchDashboard() }, [fetchDashboard])

  const onlineCount = servers.filter((s) => s.status === 'online').length

  // Platform settings editing
  const [editName, setEditName] = useState(platformName)
  const [editSubtitle, setEditSubtitle] = useState(platformSubtitle)
  const [editLogo, setEditLogo] = useState(logoUrl)
  const [settingsSaved, setSettingsSaved] = useState(false)
  const [settingsSaving, setSettingsSaving] = useState(false)

  // Sync edit fields when store values change (e.g. after fetchSettings)
  useEffect(() => { setEditName(platformName) }, [platformName])
  useEffect(() => { setEditSubtitle(platformSubtitle) }, [platformSubtitle])
  useEffect(() => { setEditLogo(logoUrl) }, [logoUrl])

  const handleSaveSettings = async () => {
    setSettingsSaving(true)
    try {
      await saveSettings(editName, editSubtitle, editLogo)
      setSettingsSaved(true)
      setTimeout(() => setSettingsSaved(false), 2000)
    } catch (err) {
      console.error('[settings] save failed:', err)
    } finally {
      setSettingsSaving(false)
    }
  }

  const [showAddServer, setShowAddServer] = useState(false)
  const [showAddCloud, setShowAddCloud] = useState(false)
  const [managedServers, setManagedServers] = useState<ManagedServer[]>([])
  const [cloudAccounts, setCloudAccounts] = useState<CloudAccount[]>([])
  const [syncingId, setSyncingId] = useState<number | null>(null)
  const [retryTarget, setRetryTarget] = useState<ManagedServer | null>(null)

  // NAS state
  const [nasDevices, setNasDevices] = useState<NasDevice[]>([])
  const [showNasDialog, setShowNasDialog] = useState(false)
  const [editingNas, setEditingNas] = useState<NasDevice | null>(null)
  const [deleteNasTarget, setDeleteNasTarget] = useState<NasDevice | null>(null)
  const [credentials, setCredentials] = useState<CredentialSummary[]>([])

  // NAS form state
  const [nasForm, setNasForm] = useState({
    name: '',
    nas_type: 'synology' as 'synology' | 'fnos',
    host: '',
    port: 22,
    ssh_user: 'root',
    credential_id: 0,
    collect_interval: 60,
  })
  const [nasAuthMode, setNasAuthMode] = useState<'password' | 'credential'>('password')
  const [nasPassword, setNasPassword] = useState('')
  const [nasTestResult, setNasTestResult] = useState<{ ok: boolean; error?: string; detected_type?: string; smart_available?: boolean } | null>(null)
  const [nasTestLoading, setNasTestLoading] = useState(false)
  const [nasSaving, setNasSaving] = useState(false)

  const fetchManaged = useCallback(() => {
    getManagedServers().then(setManagedServers).catch((err) => console.error('[settings] fetch managed:', err))
  }, [])

  const fetchCloud = useCallback(() => {
    getCloudAccounts().then(setCloudAccounts).catch((err) => console.error('[settings] fetch cloud:', err))
  }, [])

  const fetchNas = useCallback(() => {
    getNasDevices().then(setNasDevices).catch((err) => console.error('[settings] fetch nas:', err))
  }, [])

  const fetchCredentials = useCallback(() => {
    getCredentials().then(setCredentials).catch((err) => console.error('[settings] fetch credentials:', err))
  }, [])

  useEffect(() => {
    fetchManaged()
    fetchCloud()
    fetchNas()
    fetchCredentials()
    const timer = setInterval(() => {
      fetchManaged()
      fetchCloud()
      fetchNas()
    }, 15000)
    return () => clearInterval(timer)
  }, [fetchManaged, fetchCloud, fetchNas, fetchCredentials])

  const sshCredentials = credentials.filter((c) => c.type === 'ssh_password' || c.type === 'ssh_key')

  const openNasAdd = () => {
    setEditingNas(null)
    setNasForm({ name: '', nas_type: 'synology', host: '', port: 22, ssh_user: 'root', credential_id: 0, collect_interval: 60 })
    setNasAuthMode('password')
    setNasPassword('')
    setNasTestResult(null)
    setShowNasDialog(true)
  }

  const openNasEdit = (d: NasDevice) => {
    setEditingNas(d)
    setNasForm({ name: d.name, nas_type: d.nas_type, host: d.host, port: d.port, ssh_user: d.ssh_user, credential_id: d.credential_id, collect_interval: d.collect_interval })
    setNasAuthMode('credential')
    setNasPassword('')
    setNasTestResult(null)
    setShowNasDialog(true)
  }

  const handleNasTest = async () => {
    if (!nasForm.host) return
    if (nasAuthMode === 'password' && !nasPassword) return
    if (nasAuthMode === 'credential' && !nasForm.credential_id) return
    setNasTestLoading(true)
    setNasTestResult(null)
    try {
      const payload: Record<string, unknown> = { host: nasForm.host, port: nasForm.port, ssh_user: nasForm.ssh_user }
      if (nasAuthMode === 'password') {
        payload.password = nasPassword
      } else {
        payload.credential_id = nasForm.credential_id
      }
      const result = await testNasConnection(payload as Parameters<typeof testNasConnection>[0])
      setNasTestResult(result)
    } catch (err) {
      setNasTestResult({ ok: false, error: String(err) })
    } finally {
      setNasTestLoading(false)
    }
  }

  const nasFormValid = nasForm.name && nasForm.host && (nasAuthMode === 'password' ? !!nasPassword : nasForm.credential_id > 0)

  const handleNasSave = async () => {
    if (!nasFormValid) return
    setNasSaving(true)
    try {
      const payload: Record<string, unknown> = { ...nasForm }
      if (nasAuthMode === 'password') {
        payload.password = nasPassword
        delete payload.credential_id
      }
      if (editingNas) {
        await updateNasDevice(editingNas.id, payload as Parameters<typeof updateNasDevice>[1])
      } else {
        await createNasDevice(payload as Parameters<typeof createNasDevice>[0])
      }
      setShowNasDialog(false)
      fetchNas()
      fetchCredentials() // 刷新凭据列表（可能自动创建了新凭据）
    } catch (err) {
      console.error('[settings] nas save:', err)
    } finally {
      setNasSaving(false)
    }
  }

  const handleNasDelete = async (id: number) => {
    await deleteNasDevice(id)
    setDeleteNasTarget(null)
    fetchNas()
  }

  const NAS_TYPE_LABELS: Record<string, string> = { synology: 'Synology', fnos: 'fnOS' }
  const NAS_STATUS_COLOR: Record<string, string> = {
    online: '#0ab39c',
    offline: '#f06548',
    degraded: '#f7b84b',
    unknown: '#adb5bd',
  }

  const handleDeleteManaged = async (id: number) => {
    await deleteManagedServer(id)
    fetchManaged()
  }

  const handleRetryDeploy = async (id: number) => {
    setRetryTarget(null)
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
            <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">Logo</label>
            <div className="flex items-center gap-3">
              <img src={editLogo || '/logo.svg'} alt="Preview" className="w-8 h-8 rounded object-contain bg-[#f8f9fa] p-1 flex-shrink-0" />
              <input
                type="text"
                value={editLogo}
                onChange={(e) => setEditLogo(e.target.value)}
                placeholder="/logo.svg"
                className="flex-1 border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white font-mono"
              />
              <label className="flex items-center gap-1.5 px-3 py-2 text-[13px] border border-[#e9ebec] text-[#495057] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded-[8px] transition-colors cursor-pointer flex-shrink-0">
                <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>upload</span>
                上传
                <input
                  type="file"
                  accept="image/png,image/jpeg,image/svg+xml,image/webp,image/x-icon"
                  className="hidden"
                  onChange={(e) => {
                    const file = e.target.files?.[0]
                    if (!file) return
                    if (file.size > 512 * 1024) {
                      alert('图片大小不能超过 512KB')
                      return
                    }
                    const reader = new FileReader()
                    reader.onload = () => {
                      if (typeof reader.result === 'string') {
                        setEditLogo(reader.result)
                      }
                    }
                    reader.readAsDataURL(file)
                    e.target.value = ''
                  }}
                />
              </label>
            </div>
          </div>
          {isAdmin && <div className="sm:col-span-2 flex items-center gap-3">
            <button
              onClick={handleSaveSettings}
              disabled={settingsSaving}
              className="flex items-center gap-1.5 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors disabled:opacity-50"
            >
              <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>save</span>
              {settingsSaving ? '保存中...' : '保存配置'}
            </button>
            {settingsSaved && (
              <span className="text-[12px] text-[#0ab39c] flex items-center gap-1">
                <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>check_circle</span>
                已保存
              </span>
            )}
          </div>}
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

      {/* ── NAS 设备 ── */}
      <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
        <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
            <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">hard_drive</span>
          </div>
          <h2 className="text-base font-semibold text-[#495057]">NAS 设备</h2>
          <span className="text-[11px] px-2 py-0.5 rounded-full bg-[#2ca07a]/10 text-[#2ca07a] font-semibold">
            {nasDevices.length}
          </span>
          <div className="ml-auto">
            <button
              onClick={openNasAdd}
              className="flex items-center gap-1.5 px-3 py-1.5 text-[12px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors"
            >
              <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>add</span>
              添加 NAS
            </button>
          </div>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-[#f8f9fa]">
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">状态</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">名称</th>
                <th className="hidden sm:table-cell text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">类型</th>
                <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">地址</th>
                <th className="hidden md:table-cell text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">SSH 用户</th>
                <th className="hidden md:table-cell text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">采集间隔</th>
                <th className="text-right text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">操作</th>
              </tr>
            </thead>
            <tbody>
              {nasDevices.map((d, idx) => (
                <tr
                  key={d.id}
                  className={`hover:bg-[#f8f9fa] transition-colors ${idx < nasDevices.length - 1 ? 'border-b border-[#f2f4f7]' : ''}`}
                >
                  <td className="px-5 py-3.5">
                    <span
                      className="inline-block w-2 h-2 rounded-full flex-shrink-0"
                      style={{ backgroundColor: NAS_STATUS_COLOR[d.status] ?? '#adb5bd' }}
                      title={d.status}
                    />
                  </td>
                  <td className="px-5 py-3.5">
                    <span className="text-[#495057] font-medium text-sm">{d.name}</span>
                  </td>
                  <td className="hidden sm:table-cell px-5 py-3.5">
                    <span className="text-[11px] px-2 py-0.5 rounded font-medium bg-[#2ca07a]/10 text-[#2ca07a]">
                      {NAS_TYPE_LABELS[d.nas_type] ?? d.nas_type}
                    </span>
                  </td>
                  <td className="px-5 py-3.5">
                    <span className="text-[12px] font-mono text-[#495057]">{d.host}:{d.port}</span>
                  </td>
                  <td className="hidden md:table-cell px-5 py-3.5">
                    <span className="text-[12px] text-[#878a99]">{d.ssh_user}</span>
                  </td>
                  <td className="hidden md:table-cell px-5 py-3.5">
                    <span className="text-[12px] text-[#878a99]">{d.collect_interval} 秒</span>
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <div className="flex items-center justify-end gap-2">
                      <button
                        onClick={() => openNasEdit(d)}
                        className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded transition-colors"
                      >
                        编辑
                      </button>
                      <button
                        onClick={() => setDeleteNasTarget(d)}
                        className="text-[11px] px-2.5 py-1 border border-[#ced4da] text-[#878a99] hover:border-[#f06548] hover:text-[#f06548] rounded transition-colors"
                      >
                        删除
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {nasDevices.length === 0 && (
            <div className="py-10 text-center">
              <span className="material-symbols-outlined text-3xl text-[#ced4da] mb-2 block">hard_drive</span>
              <p className="text-[#878a99] text-sm">暂无 NAS 设备</p>
            </div>
          )}
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
                            onClick={() => setRetryTarget(m)}
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

      {/* ── NAS 添加/编辑对话框 ── */}
      {showNasDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setShowNasDialog(false)}>
          <div className="bg-white rounded-xl shadow-xl w-[520px] max-w-[92vw] overflow-hidden" onClick={(e) => e.stopPropagation()}>
            <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
              <div className="w-8 h-8 rounded-full bg-[#2ca07a]/15 flex items-center justify-center">
                <span className="material-symbols-outlined text-[#2ca07a] text-[16px]">hard_drive</span>
              </div>
              <h3 className="text-sm font-semibold text-[#495057]">{editingNas ? '编辑 NAS 设备' : '添加 NAS 设备'}</h3>
            </div>
            <div className="px-5 py-4 grid grid-cols-2 gap-4">
              {/* 名称 */}
              <div className="col-span-2">
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">名称 <span className="text-[#f06548]">*</span></label>
                <input
                  type="text"
                  value={nasForm.name}
                  onChange={(e) => setNasForm((f) => ({ ...f, name: e.target.value }))}
                  placeholder="我的 NAS"
                  className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors"
                />
              </div>
              {/* NAS 类型 */}
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">NAS 类型</label>
                <select
                  value={nasForm.nas_type}
                  onChange={(e) => setNasForm((f) => ({ ...f, nas_type: e.target.value as 'synology' | 'fnos' }))}
                  className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
                >
                  <option value="synology">Synology</option>
                  <option value="fnos">fnOS</option>
                </select>
              </div>
              {/* 采集间隔 */}
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">采集间隔（秒，≥30）</label>
                <input
                  type="number"
                  min={30}
                  value={nasForm.collect_interval}
                  onChange={(e) => setNasForm((f) => ({ ...f, collect_interval: Math.max(30, parseInt(e.target.value) || 60) }))}
                  className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors"
                />
              </div>
              {/* 地址 */}
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">地址 <span className="text-[#f06548]">*</span></label>
                <input
                  type="text"
                  value={nasForm.host}
                  onChange={(e) => setNasForm((f) => ({ ...f, host: e.target.value }))}
                  placeholder="192.168.1.100"
                  className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors font-mono"
                />
              </div>
              {/* SSH 端口 */}
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">SSH 端口</label>
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={nasForm.port}
                  onChange={(e) => setNasForm((f) => ({ ...f, port: parseInt(e.target.value) || 22 }))}
                  className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors"
                />
              </div>
              {/* SSH 用户 */}
              <div>
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">SSH 用户</label>
                <input
                  type="text"
                  value={nasForm.ssh_user}
                  onChange={(e) => setNasForm((f) => ({ ...f, ssh_user: e.target.value }))}
                  placeholder="root"
                  className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors"
                />
              </div>
              {/* SSH 认证 */}
              <div className="col-span-2">
                <label className="block text-[12px] font-medium text-[#878a99] mb-1.5">SSH 认证 <span className="text-[#f06548]">*</span></label>
                <div className="flex gap-2 mb-2">
                  <button
                    type="button"
                    onClick={() => { setNasAuthMode('password'); setNasForm(f => ({ ...f, credential_id: 0 })) }}
                    className={`text-[12px] px-3 py-1.5 rounded-md transition-colors ${nasAuthMode === 'password' ? 'bg-[#2ca07a] text-white' : 'bg-[#f3f6f9] text-[#878a99] hover:text-[#495057]'}`}
                  >
                    输入密码
                  </button>
                  <button
                    type="button"
                    onClick={() => { setNasAuthMode('credential'); setNasPassword('') }}
                    className={`text-[12px] px-3 py-1.5 rounded-md transition-colors ${nasAuthMode === 'credential' ? 'bg-[#2ca07a] text-white' : 'bg-[#f3f6f9] text-[#878a99] hover:text-[#495057]'}`}
                  >
                    已有凭据
                  </button>
                </div>
                {nasAuthMode === 'password' ? (
                  <input
                    type="password"
                    value={nasPassword}
                    onChange={(e) => setNasPassword(e.target.value)}
                    placeholder="SSH 密码"
                    className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors"
                  />
                ) : (
                  <select
                    value={nasForm.credential_id}
                    onChange={(e) => setNasForm((f) => ({ ...f, credential_id: parseInt(e.target.value) }))}
                    className="w-full border border-[#e9ebec] rounded-[8px] px-3 py-2 text-sm text-[#495057] focus:outline-none focus:border-[#2ca07a] focus:ring-2 focus:ring-[#2ca07a]/15 transition-colors bg-white"
                  >
                    <option value={0} disabled>请选择凭据</option>
                    {sshCredentials.map((c) => (
                      <option key={c.id} value={c.id}>{c.name} ({c.type === 'ssh_key' ? '密钥' : '密码'})</option>
                    ))}
                  </select>
                )}
              </div>
              {/* 测试连接按钮 + 结果 */}
              <div className="col-span-2">
                <button
                  onClick={handleNasTest}
                  disabled={nasTestLoading || !nasForm.host || (nasAuthMode === 'password' ? !nasPassword : !nasForm.credential_id)}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-[12px] border border-[#ced4da] text-[#495057] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {nasTestLoading ? (
                    <span className="w-3 h-3 border border-[#2ca07a]/40 border-t-[#2ca07a] rounded-full animate-spin flex-shrink-0" />
                  ) : (
                    <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>network_check</span>
                  )}
                  {nasTestLoading ? '测试中...' : '测试连接'}
                </button>
                {nasTestResult && (
                  <div className={`mt-2 px-3 py-2 rounded-[6px] text-[12px] ${nasTestResult.ok ? 'bg-[#0ab39c]/08 text-[#0ab39c] border border-[#0ab39c]/20' : 'bg-[#f06548]/08 text-[#f06548] border border-[#f06548]/20'}`}>
                    {nasTestResult.ok ? (
                      <div className="flex flex-wrap gap-x-4 gap-y-1">
                        <span className="flex items-center gap-1">
                          <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>check_circle</span>
                          连接成功
                        </span>
                        {nasTestResult.detected_type && (
                          <span>检测类型：{nasTestResult.detected_type}</span>
                        )}
                        {nasTestResult.smart_available !== undefined && (
                          <span>smartctl：{nasTestResult.smart_available ? '可用' : '不可用'}</span>
                        )}
                      </div>
                    ) : (
                      <span className="flex items-center gap-1">
                        <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>error</span>
                        {nasTestResult.error ?? '连接失败'}
                      </span>
                    )}
                  </div>
                )}
              </div>
            </div>
            <div className="px-5 py-3 border-t border-[#e9ebec] flex justify-end gap-2">
              <button
                onClick={() => setShowNasDialog(false)}
                className="text-xs px-4 py-2 border border-[#ced4da] text-[#878a99] rounded-lg hover:bg-[#f8f9fa] transition-colors"
              >
                取消
              </button>
              <button
                onClick={handleNasSave}
                disabled={nasSaving || !nasFormValid}
                className="text-xs px-4 py-2 bg-[#2ca07a] text-white rounded-lg hover:bg-[#248a69] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {nasSaving ? '保存中...' : '保存'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── NAS 删除确认 ── */}
      {deleteNasTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setDeleteNasTarget(null)}>
          <div className="bg-white rounded-xl shadow-xl w-[400px] max-w-[90vw] overflow-hidden" onClick={(e) => e.stopPropagation()}>
            <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
              <div className="w-8 h-8 rounded-full bg-[#f06548]/15 flex items-center justify-center">
                <span className="material-symbols-outlined text-[#f06548] text-[16px]">delete</span>
              </div>
              <h3 className="text-sm font-semibold text-[#495057]">删除 NAS 设备</h3>
            </div>
            <div className="px-5 py-4">
              <p className="text-sm text-[#495057]">确定要删除 <span className="font-semibold">{deleteNasTarget.name}</span> 吗？此操作不可撤销。</p>
            </div>
            <div className="px-5 py-3 border-t border-[#e9ebec] flex justify-end gap-2">
              <button
                onClick={() => setDeleteNasTarget(null)}
                className="text-xs px-4 py-2 border border-[#ced4da] text-[#878a99] rounded-lg hover:bg-[#f8f9fa] transition-colors"
              >
                取消
              </button>
              <button
                onClick={() => handleNasDelete(deleteNasTarget.id)}
                className="text-xs px-4 py-2 bg-[#f06548] text-white rounded-lg hover:bg-[#d9533a] transition-colors"
              >
                确认删除
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── 重试确认对话框 ── */}
      {retryTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={() => setRetryTarget(null)}>
          <div className="bg-white rounded-xl shadow-xl w-[480px] max-w-[90vw] overflow-hidden" onClick={(e) => e.stopPropagation()}>
            <div className="px-5 py-4 border-b border-[#e9ebec] flex items-center gap-3">
              <div className="w-8 h-8 rounded-full bg-[#f06548]/15 flex items-center justify-center">
                <span className="material-symbols-outlined text-[#f06548] text-[16px]">error</span>
              </div>
              <h3 className="text-sm font-semibold text-[#495057]">部署失败 — {retryTarget.host}</h3>
            </div>
            <div className="px-5 py-4">
              <p className="text-xs text-[#878a99] mb-2">错误信息：</p>
              <div className="bg-[#f8f9fa] border border-[#e9ebec] rounded-lg px-3 py-2.5 text-[12px] text-[#495057] leading-relaxed whitespace-pre-wrap break-all max-h-[200px] overflow-y-auto font-mono">
                {retryTarget.install_error || '未知错误'}
              </div>
            </div>
            <div className="px-5 py-3 border-t border-[#e9ebec] flex justify-end gap-2">
              <button
                onClick={() => setRetryTarget(null)}
                className="text-xs px-4 py-2 border border-[#ced4da] text-[#878a99] rounded-lg hover:bg-[#f8f9fa] transition-colors"
              >
                取消
              </button>
              <button
                onClick={() => handleRetryDeploy(retryTarget.id)}
                className="text-xs px-4 py-2 bg-[#2ca07a] text-white rounded-lg hover:bg-[#248a69] transition-colors"
              >
                确认重试
              </button>
            </div>
          </div>
        </div>
      )}

    </div>
  )
}
