import { useEffect, useState } from 'react'
import { getAssets, createAsset, deleteAsset, type AssetInfo } from '../../api/client'
import { useServerStore } from '../../stores/serverStore'
import { useAuthStore } from '../../stores/authStore'

export default function Assets() {
  const canEdit = useAuthStore((s) => s.role === 'admin' || s.role === 'operator')
  const [assets, setAssets] = useState<AssetInfo[]>([])
  const servers = useServerStore((s) => s.servers)
  const fetchDashboard = useServerStore((s) => s.fetchDashboard)
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ server_id: 0, name: '', category: '项目', tech_stack: '', path: '', port: '', description: '' })

  const load = async () => {
    const data = await getAssets()
    setAssets(data)
  }

  useEffect(() => { load(); fetchDashboard() }, [fetchDashboard])

  const getServer = (sid: number) => servers.find((sv) => sv.id === sid)

  const getServerName = (sid: number) => {
    const s = getServer(sid)
    return s?.display_name || s?.hostname || `Server #${sid}`
  }

  const getServerIp = (sid: number) => {
    const s = getServer(sid)
    if (!s?.ip_addresses) return '-'
    try { return JSON.parse(s.ip_addresses)[0] || '-' } catch { return '-' }
  }

  const getServerInfo = (sid: number) => {
    const s = getServer(sid)
    if (!s) return ''
    const parts: string[] = []
    if (s.cpu_model) parts.push(s.cpu_model)
    if (s.cpu_cores) parts.push(`${s.cpu_cores} 核`)
    if (s.memory_total) parts.push(`${(s.memory_total / (1024 * 1024 * 1024)).toFixed(0)} GB 内存`)
    if (s.os) parts.push(s.os)
    return parts.join(' / ')
  }

  const getAssetCount = (sid: number) => assets.filter((a) => a.server_id === sid).length

  const grouped = assets.reduce((acc, a) => {
    if (!acc[a.server_id]) acc[a.server_id] = []
    acc[a.server_id].push(a)
    return acc
  }, {} as Record<number, AssetInfo[]>)

  const handleAdd = async () => {
    if (!form.name || !form.server_id) return
    await createAsset({ ...form, status: 'active', extra_info: '' } as AssetInfo)
    setShowAdd(false)
    setForm({ server_id: 0, name: '', category: '项目', tech_stack: '', path: '', port: '', description: '' })
    load()
  }

  const handleDelete = async (id: number) => {
    if (!window.confirm('确定要删除此资产记录吗？此操作不可撤销。')) return
    try {
      await deleteAsset(id)
      load()
    } catch {
      alert('删除失败，请重试')
    }
  }

  // Tech stack badge colors cycling
  const stackColors = [
    'bg-[#2ca07a]/10 text-[#2ca07a]',
    'bg-[#3577f1]/10 text-[#3577f1]',
    'bg-[#f7b84b]/10 text-[#c98a1a]',
    'bg-[#f06548]/10 text-[#f06548]',
    'bg-[#6f42c1]/10 text-[#6f42c1]',
    'bg-[#20a8d8]/10 text-[#20a8d8]',
  ]

  const renderTechStack = (stack: string) => {
    if (!stack) return <span className="text-[#adb5bd] text-xs">-</span>
    return (
      <div className="flex flex-wrap gap-1">
        {stack.split(/[,，、/]/).map((t, i) => (
          <span
            key={i}
            className={`text-[11px] px-2 py-0.5 rounded font-medium ${stackColors[i % stackColors.length]}`}
          >
            {t.trim()}
          </span>
        ))}
      </div>
    )
  }

  const activeAssets = assets.filter((a) => a.status === 'active').length

  return (
    <div>
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 className="text-[22px] font-semibold text-[#495057]">资产台账</h1>
          <p className="text-sm text-[#878a99] mt-1">实时监控全链路应用资产，自动识别技术栈与端口占用</p>
        </div>
        {canEdit && (
          <button
            onClick={() => setShowAdd(!showAdd)}
            className="inline-flex items-center gap-2 bg-[#2ca07a] hover:bg-[#259b73] text-white px-4 py-2 rounded-[6px] text-sm font-medium transition-colors shadow-sm"
          >
            <span className="material-symbols-outlined text-[16px]">add</span>
            添加业务
          </button>
        )}
      </div>

      {/* Add form */}
      {showAdd && (
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-6 mb-6">
          <h3 className="text-base font-semibold text-[#495057] mb-4">新增资产</h3>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-3">
            <select
              value={form.server_id}
              onChange={(e) => setForm({ ...form, server_id: parseInt(e.target.value) })}
              className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] bg-white focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
            >
              <option value={0}>选择服务器</option>
              {servers.map((s) => (
                <option key={s.id} value={s.id}>{s.display_name || s.hostname}</option>
              ))}
            </select>
            <input
              placeholder="项目名称"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
            />
            <input
              placeholder="技术栈 (逗号分隔)"
              value={form.tech_stack}
              onChange={(e) => setForm({ ...form, tech_stack: e.target.value })}
              className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
            />
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <input
              placeholder="部署路径"
              value={form.path}
              onChange={(e) => setForm({ ...form, path: e.target.value })}
              className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
            />
            <input
              placeholder="端口"
              value={form.port}
              onChange={(e) => setForm({ ...form, port: e.target.value })}
              className="border border-[#e9ebec] rounded-[6px] px-3 py-2 text-sm text-[#495057] placeholder:text-[#adb5bd] focus:outline-none focus:border-[#2ca07a] focus:ring-1 focus:ring-[#2ca07a]/20 transition-colors"
            />
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

      {/* Server groups */}
      {Object.entries(grouped).map(([sid, items]) => (
        <div key={sid} className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] mb-5 overflow-hidden">
          {/* Group header */}
          <div className="flex items-center gap-4 px-5 py-4 bg-[#f8f9fa] border-b border-[#e9ebec]">
            <div className="w-12 h-12 rounded-full bg-[#2ca07a]/15 flex items-center justify-center flex-shrink-0">
              <span className="material-symbols-outlined text-[#2ca07a] text-[20px]">dns</span>
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <h2 className="text-base font-semibold text-[#495057]">
                  {getServerName(parseInt(sid))}
                </h2>
                <span className="text-[11px] px-2 py-0.5 rounded font-mono bg-[#0ab39c]/10 text-[#0ab39c]">
                  {getServerIp(parseInt(sid))}
                </span>
              </div>
              <p className="text-[12px] text-[#878a99] mt-0.5 truncate">{getServerInfo(parseInt(sid))}</p>
            </div>
            <div className="flex-shrink-0 text-right">
              <div className="text-lg font-bold text-[#495057]">{getAssetCount(parseInt(sid))}</div>
              <div className="text-[11px] text-[#878a99]">个资产</div>
            </div>
          </div>

          {/* Asset table - desktop */}
          <div className="hidden md:block overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-[#f8f9fa]">
                  <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">名称</th>
                  <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">技术栈</th>
                  <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">路径</th>
                  <th className="text-left text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">端口</th>
                  <th className="text-right text-[11px] text-[#878a99] uppercase tracking-wider px-5 py-3 font-medium border-b border-[#e9ebec]">操作</th>
                </tr>
              </thead>
              <tbody>
                {items.map((a, idx) => (
                  <tr
                    key={a.id}
                    className={`group hover:bg-[#f8f9fa] transition-colors ${idx < items.length - 1 ? 'border-b border-[#f2f4f7]' : ''}`}
                  >
                    <td className="px-5 py-3.5 text-[#495057] font-medium text-sm">{a.name}</td>
                    <td className="px-5 py-3.5">{renderTechStack(a.tech_stack)}</td>
                    <td className="px-5 py-3.5">
                      {a.path ? (
                        <span className="text-xs font-mono bg-[#f3f6f9] text-[#495057] px-2 py-0.5 rounded">
                          {a.path}
                        </span>
                      ) : (
                        <span className="text-[#adb5bd] text-xs">-</span>
                      )}
                    </td>
                    <td className="px-5 py-3.5">
                      {a.port ? (
                        <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-[#495057] text-white">
                          {a.port}
                        </span>
                      ) : (
                        <span className="text-[#adb5bd] text-xs">-</span>
                      )}
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      <button
                        onClick={() => handleDelete(a.id!)}
                        className="text-xs text-[#878a99] opacity-0 group-hover:opacity-100 transition-opacity hover:text-[#f06548] px-2 py-1 rounded hover:bg-[#f06548]/10"
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Asset cards - mobile */}
          <div className="md:hidden divide-y divide-[#f2f4f7]">
            {items.map((a) => (
              <div key={a.id} className="p-4">
                <div className="flex items-start justify-between gap-2 mb-2">
                  <span className="text-sm font-medium text-[#495057]">{a.name}</span>
                  <button
                    onClick={() => handleDelete(a.id!)}
                    className="text-xs text-[#f06548] px-2 py-1 hover:bg-[#f06548]/10 rounded transition-colors"
                  >
                    删除
                  </button>
                </div>
                {a.tech_stack && <div className="mb-2">{renderTechStack(a.tech_stack)}</div>}
                <div className="flex flex-wrap gap-2 text-xs text-[#878a99] font-mono">
                  {a.path && <span className="bg-[#f3f6f9] px-2 py-0.5 rounded">{a.path}</span>}
                  {a.port && <span className="bg-[#495057] text-white px-2 py-0.5 rounded">:{a.port}</span>}
                </div>
              </div>
            ))}
          </div>
        </div>
      ))}

      {/* Empty state */}
      {assets.length === 0 && (
        <div className="bg-white rounded-[10px] shadow-[0_1px_2px_rgba(56,65,74,0.15)] p-16 text-center">
          <span className="material-symbols-outlined text-4xl text-[#ced4da] mb-4 block">inventory_2</span>
          <p className="text-[#495057] font-medium mb-1">暂无资产信息</p>
          <p className="text-[#878a99] text-sm">点击「添加业务」录入第一条记录</p>
        </div>
      )}

      {/* Bottom stats */}
      {assets.length > 0 && (
        <div className="mt-6 flex items-center gap-6 text-xs text-[#878a99]">
          <div className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-full bg-[#0ab39c]" />
            <span>活跃资产 <span className="font-semibold text-[#495057]">{activeAssets}</span></span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-full bg-[#2ca07a]" />
            <span>总计 <span className="font-semibold text-[#495057]">{assets.length}</span></span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-full bg-[#f7b84b]" />
            <span>服务器 <span className="font-semibold text-[#495057]">{Object.keys(grouped).length}</span></span>
          </div>
        </div>
      )}
    </div>
  )
}
