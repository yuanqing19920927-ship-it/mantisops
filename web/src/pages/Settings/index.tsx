import { useServerStore } from '../../stores/serverStore'
import { useSettingsStore } from '../../stores/settingsStore'
import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { StatusBadge } from '../../components/StatusBadge'
import { timeSince } from '../../utils/format'

export default function Settings() {
  const { servers, metrics, fetchDashboard } = useServerStore()
  const { platformName } = useSettingsStore()

  useEffect(() => { fetchDashboard() }, [fetchDashboard])

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

    </div>
  )
}
