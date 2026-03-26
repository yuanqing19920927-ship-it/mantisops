import { useEffect, useState } from 'react'
import { getAssets, createAsset, deleteAsset, type AssetInfo } from '../../api/client'
import { useServerStore } from '../../stores/serverStore'

export default function Assets() {
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

  const getServerName = (sid: number) => {
    const s = servers.find((sv) => sv.id === sid)
    return s?.display_name || s?.hostname || `Server #${sid}`
  }

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
    await deleteAsset(id)
    load()
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold" style={{ color: 'var(--text-primary)' }}>资产信息</h1>
        <button onClick={() => setShowAdd(!showAdd)}
          className="px-4 py-2 rounded-lg text-sm text-white" style={{ backgroundColor: 'var(--accent)' }}>
          + 添加项目
        </button>
      </div>

      {showAdd && (
        <div className="rounded-xl p-4 mb-6" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-3">
            <select value={form.server_id} onChange={(e) => setForm({ ...form, server_id: parseInt(e.target.value) })}
              className="px-3 py-2 rounded-lg text-sm" style={{ backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border)' }}>
              <option value={0}>选择服务器</option>
              {servers.map((s) => <option key={s.id} value={s.id}>{s.display_name || s.hostname}</option>)}
            </select>
            <input placeholder="项目名称" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="px-3 py-2 rounded-lg text-sm" style={{ backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border)' }} />
            <input placeholder="技术栈" value={form.tech_stack} onChange={(e) => setForm({ ...form, tech_stack: e.target.value })}
              className="px-3 py-2 rounded-lg text-sm" style={{ backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border)' }} />
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-3">
            <input placeholder="路径" value={form.path} onChange={(e) => setForm({ ...form, path: e.target.value })}
              className="px-3 py-2 rounded-lg text-sm" style={{ backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border)' }} />
            <input placeholder="端口" value={form.port} onChange={(e) => setForm({ ...form, port: e.target.value })}
              className="px-3 py-2 rounded-lg text-sm" style={{ backgroundColor: 'var(--bg-secondary)', color: 'var(--text-primary)', border: '1px solid var(--border)' }} />
            <button onClick={handleAdd} className="px-4 py-2 rounded-lg text-sm text-white" style={{ backgroundColor: 'var(--success)' }}>保存</button>
          </div>
        </div>
      )}

      {Object.entries(grouped).map(([sid, items]) => (
        <div key={sid} className="mb-6">
          <h2 className="text-lg font-semibold mb-3" style={{ color: 'var(--text-primary)' }}>
            {getServerName(parseInt(sid))}
          </h2>
          <div className="rounded-xl overflow-hidden" style={{ backgroundColor: 'var(--bg-card)', border: '1px solid var(--border)' }}>
            <table className="w-full text-sm">
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)', color: 'var(--text-secondary)' }}>
                  <th className="text-left p-3">名称</th>
                  <th className="text-left p-3">技术栈</th>
                  <th className="text-left p-3">路径</th>
                  <th className="text-left p-3">端口</th>
                  <th className="text-center p-3">操作</th>
                </tr>
              </thead>
              <tbody>
                {items.map((a) => (
                  <tr key={a.id} style={{ borderBottom: '1px solid var(--border)' }}>
                    <td className="p-3" style={{ color: 'var(--text-primary)' }}>{a.name}</td>
                    <td className="p-3" style={{ color: 'var(--text-secondary)' }}>{a.tech_stack}</td>
                    <td className="p-3 text-xs font-mono" style={{ color: 'var(--text-secondary)' }}>{a.path}</td>
                    <td className="p-3" style={{ color: 'var(--text-secondary)' }}>{a.port}</td>
                    <td className="p-3 text-center">
                      <button onClick={() => handleDelete(a.id!)} className="text-xs" style={{ color: 'var(--danger)' }}>删除</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ))}

      {assets.length === 0 && (
        <div className="text-center py-12" style={{ color: 'var(--text-secondary)' }}>
          暂无资产信息，点击"添加项目"录入
        </div>
      )}
    </div>
  )
}
