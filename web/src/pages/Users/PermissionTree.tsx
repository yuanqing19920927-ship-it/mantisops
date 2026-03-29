import { useEffect, useState, useCallback } from 'react'
import { useParams, useNavigate, Navigate } from 'react-router-dom'
import { useAuthStore } from '../../stores/authStore'
import { getUser, getUserPermissions, setUserPermissions, type PermissionItem } from '../../api/users'
import { getGroups, getServers, getDatabases, getProbes, type ProbeRule, type RDSInfo } from '../../api/client'
import type { Server, ServerGroup } from '../../types'

export default function PermissionTree() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const role = useAuthStore((s) => s.role)
  const userId = Number(id)

  if (role !== 'admin') return <Navigate to="/" replace />
  if (!id || isNaN(userId)) return <Navigate to="/users" replace />

  const [username, setUsername] = useState('')
  const [groups, setGroups] = useState<ServerGroup[]>([])
  const [servers, setServers] = useState<Server[]>([])
  const [databases, setDatabases] = useState<RDSInfo[]>([])
  const [probes, setProbes] = useState<ProbeRule[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set()) // "type:id" format
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [loadError, setLoadError] = useState('')

  const load = useCallback(async () => {
    try {
      setLoadError('')
      const [user, perms, grps, srvs, dbs, prbs] = await Promise.all([
        getUser(userId),
        getUserPermissions(userId),
        getGroups(),
        getServers(),
        getDatabases(),
        getProbes(),
      ])
      setUsername(user.username)
      setGroups(grps)
      setServers(srvs)
      setDatabases(dbs)
      setProbes(prbs)
      const sel = new Set<string>()
      for (const p of perms as PermissionItem[]) {
        sel.add(`${p.res_type}:${p.res_id}`)
      }
      setSelected(sel)
    } catch {
      setLoadError('加载权限数据失败，请重试')
    }
  }, [userId])

  useEffect(() => { load() }, [load])

  const toggle = (key: string) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const isGroupChecked = (gid: number) => selected.has(`group:${gid}`)

  const isServerVisible = (srv: Server) => {
    if (srv.group_id && isGroupChecked(srv.group_id)) return true
    return selected.has(`server:${srv.host_id}`)
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const perms: PermissionItem[] = []
      selected.forEach(key => {
        const [type, ...rest] = key.split(':')
        perms.push({ res_type: type, res_id: rest.join(':') })
      })
      await setUserPermissions(userId, perms)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (err) {
      alert((err as Error).message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  // Group servers by group_id
  const groupedServers: Record<number, Server[]> = {}
  const ungrouped: Server[] = []
  for (const s of servers) {
    if (s.group_id) {
      if (!groupedServers[s.group_id]) groupedServers[s.group_id] = []
      groupedServers[s.group_id].push(s)
    } else {
      ungrouped.push(s)
    }
  }

  if (loadError) return (
    <div className="flex flex-col items-center justify-center py-20">
      <p className="text-red-400 text-sm mb-4">{loadError}</p>
      <button onClick={load} className="px-4 py-2 text-sm bg-primary/20 text-primary rounded hover:bg-primary/30 transition-colors">重试</button>
    </div>
  )

  return (
    <div className="flex flex-col gap-5">
      {/* Header */}
      <div className="flex items-center gap-3">
        <button onClick={() => navigate('/users')} className="text-[#878a99] hover:text-[#495057] transition-colors">
          <span className="material-symbols-outlined text-[20px]">arrow_back</span>
        </button>
        <div>
          <h1 className="text-[22px] font-semibold text-[#495057]">权限配置</h1>
          <p className="text-sm text-[#878a99] mt-0.5">用户：{username}</p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
        {/* Left: Tree */}
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
          <div className="px-5 py-4 border-b border-[#e9ebec]">
            <h2 className="text-base font-semibold text-[#495057]">资源列表</h2>
            <p className="text-[11px] text-[#878a99] mt-0.5">勾选分组将包含组内所有服务器</p>
          </div>
          <div className="p-5 space-y-4 max-h-[600px] overflow-y-auto">
            {/* Server groups */}
            <Section title="服务器分组" icon="dns">
              {groups.map(g => (
                <div key={g.id}>
                  <CheckItem
                    label={`[组] ${g.name}`}
                    checked={isGroupChecked(g.id)}
                    onChange={() => toggle(`group:${g.id}`)}
                    bold
                  />
                  {groupedServers[g.id]?.map(s => (
                    <CheckItem
                      key={s.host_id}
                      label={`${s.display_name || s.hostname} (${s.ip_addresses})`}
                      checked={isServerVisible(s)}
                      disabled={isGroupChecked(s.group_id!)}
                      onChange={() => toggle(`server:${s.host_id}`)}
                      indent
                    />
                  ))}
                </div>
              ))}
              {ungrouped.length > 0 && (
                <div>
                  <div className="text-[11px] text-[#878a99] font-medium mt-2 mb-1 pl-1">未分组</div>
                  {ungrouped.map(s => (
                    <CheckItem
                      key={s.host_id}
                      label={`${s.display_name || s.hostname} (${s.ip_addresses})`}
                      checked={selected.has(`server:${s.host_id}`)}
                      onChange={() => toggle(`server:${s.host_id}`)}
                      indent
                    />
                  ))}
                </div>
              )}
            </Section>

            {/* Databases */}
            {databases.length > 0 && (
              <Section title="数据库" icon="database">
                {databases.map(d => (
                  <CheckItem
                    key={d.host_id}
                    label={`${d.name} (${d.engine})`}
                    checked={selected.has(`database:${d.host_id}`)}
                    onChange={() => toggle(`database:${d.host_id}`)}
                  />
                ))}
              </Section>
            )}

            {/* Probes */}
            {probes.length > 0 && (
              <Section title="探测规则" icon="sensors">
                {probes.map(p => (
                  <CheckItem
                    key={p.id}
                    label={`${p.name} (${p.protocol === 'tcp' ? `${p.host}:${p.port}` : p.url})`}
                    checked={selected.has(`probe:${p.id}`)}
                    onChange={() => toggle(`probe:${p.id}`)}
                  />
                ))}
              </Section>
            )}
          </div>
        </div>

        {/* Right: Selected summary */}
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] overflow-hidden">
          <div className="px-5 py-4 border-b border-[#e9ebec]">
            <h2 className="text-base font-semibold text-[#495057]">已选资源</h2>
            <p className="text-[11px] text-[#878a99] mt-0.5">共 {selected.size} 项</p>
          </div>
          <div className="p-5 max-h-[500px] overflow-y-auto">
            {selected.size === 0 ? (
              <p className="text-sm text-[#adb5bd] text-center py-8">未选择任何资源</p>
            ) : (
              <div className="space-y-1">
                {Array.from(selected).sort().map(key => {
                  const [type, ...rest] = key.split(':')
                  const resId = rest.join(':')
                  const label = resolveLabel(type, resId, groups, servers, databases, probes)
                  const typeLabel = { group: '分组', server: '服务器', database: '数据库', probe: '探测' }[type] || type
                  return (
                    <div key={key} className="flex items-center justify-between py-1.5 px-2 rounded hover:bg-[#f8f9fa] group">
                      <div className="flex items-center gap-2">
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-[#2ca07a]/10 text-[#2ca07a] font-medium">{typeLabel}</span>
                        <span className="text-sm text-[#495057]">{label}</span>
                      </div>
                      <button onClick={() => toggle(key)} className="opacity-0 group-hover:opacity-100 text-[#878a99] hover:text-[#f06548] transition-all">
                        <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>close</span>
                      </button>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
          <div className="px-5 py-4 border-t border-[#e9ebec] flex items-center gap-3">
            <button
              onClick={handleSave}
              disabled={saving}
              className="flex items-center gap-1.5 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded transition-colors disabled:opacity-50"
            >
              <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>save</span>
              {saving ? '保存中...' : '保存权限'}
            </button>
            {saved && (
              <span className="text-[12px] text-[#0ab39c] flex items-center gap-1">
                <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>check_circle</span>
                已保存
              </span>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// --- Helper components ---

function Section({ title, icon, children }: { title: string; icon: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="flex items-center gap-2 mb-2">
        <span className="material-symbols-outlined text-[#2ca07a] text-[18px]">{icon}</span>
        <span className="text-[13px] font-semibold text-[#495057]">{title}</span>
      </div>
      <div className="pl-2">{children}</div>
    </div>
  )
}

function CheckItem({ label, checked, onChange, disabled, indent, bold }: {
  label: string; checked: boolean; onChange: () => void; disabled?: boolean; indent?: boolean; bold?: boolean
}) {
  return (
    <label className={`flex items-center gap-2 py-1 cursor-pointer hover:bg-[#f8f9fa] rounded px-1 ${indent ? 'pl-6' : ''} ${disabled ? 'opacity-60 cursor-not-allowed' : ''}`}>
      <input
        type="checkbox"
        checked={checked}
        onChange={disabled ? undefined : onChange}
        disabled={disabled}
        className="accent-[#2ca07a] w-3.5 h-3.5"
      />
      <span className={`text-[13px] ${bold ? 'font-medium text-[#495057]' : 'text-[#878a99]'}`}>{label}</span>
    </label>
  )
}

function resolveLabel(type: string, resId: string, groups: ServerGroup[], servers: Server[], databases: RDSInfo[], probes: ProbeRule[]): string {
  if (type === 'group') {
    const g = groups.find(g => String(g.id) === resId)
    return g ? g.name : resId
  }
  if (type === 'server') {
    const s = servers.find(s => s.host_id === resId)
    return s ? (s.display_name || s.hostname) : resId
  }
  if (type === 'database') {
    const d = databases.find(d => d.host_id === resId)
    return d ? d.name : resId
  }
  if (type === 'probe') {
    const p = probes.find(p => String(p.id) === resId)
    return p ? p.name : resId
  }
  return resId
}
