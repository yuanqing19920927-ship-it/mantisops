// Usage:
// <InstanceSelector instances={instances} onConfirm={(ids) => handleConfirm(ids)} loading={false} />

import { useState, useMemo } from 'react'
import type { CloudInstance } from '../types/onboarding'

interface Props {
  instances: CloudInstance[]
  onConfirm: (ids: number[]) => void
  loading?: boolean
}

const TYPE_LABEL: Record<string, string> = {
  ecs: 'ECS 云服务器',
  rds: 'RDS 数据库',
}

export function InstanceSelector({ instances, onConfirm, loading }: Props) {
  const [selected, setSelected] = useState<Set<number>>(new Set())

  const grouped = useMemo(() => {
    const map = new Map<string, CloudInstance[]>()
    for (const inst of instances) {
      const type = inst.instance_type || 'other'
      if (!map.has(type)) map.set(type, [])
      map.get(type)!.push(inst)
    }
    return map
  }, [instances])

  const allIds = instances.map((i) => i.id)
  const isAllSelected = allIds.length > 0 && allIds.every((id) => selected.has(id))
  const isNoneSelected = selected.size === 0

  const toggleAll = () => {
    if (isAllSelected) {
      setSelected(new Set())
    } else {
      setSelected(new Set(allIds))
    }
  }

  const toggleInstance = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  const toggleGroup = (type: string) => {
    const groupIds = (grouped.get(type) || []).map((i) => i.id)
    const allGroupSelected = groupIds.every((id) => selected.has(id))
    setSelected((prev) => {
      const next = new Set(prev)
      if (allGroupSelected) {
        groupIds.forEach((id) => next.delete(id))
      } else {
        groupIds.forEach((id) => next.add(id))
      }
      return next
    })
  }

  if (instances.length === 0) {
    return (
      <div className="py-10 text-center">
        <span className="material-symbols-outlined text-3xl text-[#ced4da] block mb-2">cloud_off</span>
        <p className="text-[#878a99] text-[13px]">未发现可接入的实例</p>
      </div>
    )
  }

  return (
    <div>
      {/* Toolbar */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <button
            onClick={toggleAll}
            className="text-[12px] px-3 py-1.5 border border-[#ced4da] text-[#878a99] hover:border-[#2ca07a] hover:text-[#2ca07a] rounded transition-colors"
          >
            {isAllSelected ? '全不选' : '全选'}
          </button>
          <span className="text-[12px] text-[#878a99]">
            已选 <span className="font-semibold text-[#495057]">{selected.size}</span> / {instances.length} 个
          </span>
        </div>
        <button
          onClick={() => onConfirm(Array.from(selected))}
          disabled={isNoneSelected || loading}
          className="flex items-center gap-1.5 px-4 py-1.5 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded-lg disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          {loading ? (
            <span className="w-3.5 h-3.5 border border-white/30 border-t-white rounded-full animate-spin" />
          ) : (
            <span className="material-symbols-outlined text-[15px]">add_circle</span>
          )}
          确认接入
        </button>
      </div>

      {/* Groups */}
      <div className="space-y-4">
        {Array.from(grouped.entries()).map(([type, group]) => {
          const groupIds = group.map((i) => i.id)
          const allGroupSelected = groupIds.every((id) => selected.has(id))
          const someGroupSelected = groupIds.some((id) => selected.has(id))

          return (
            <div key={type}>
              {/* Group header */}
              <div className="flex items-center gap-2 mb-2">
                <button
                  onClick={() => toggleGroup(type)}
                  className="w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 transition-colors"
                  style={{
                    backgroundColor: allGroupSelected ? '#2ca07a' : someGroupSelected ? '#2ca07a' : 'white',
                    borderColor: allGroupSelected || someGroupSelected ? '#2ca07a' : '#ced4da',
                  }}
                >
                  {allGroupSelected ? (
                    <span className="material-symbols-outlined text-white" style={{ fontSize: '11px' }}>check</span>
                  ) : someGroupSelected ? (
                    <span className="w-2 h-0.5 bg-white" />
                  ) : null}
                </button>
                <span className="text-[12px] font-semibold text-[#495057]">{TYPE_LABEL[type] || type.toUpperCase()}</span>
                <span className="text-[11px] text-[#878a99]">({group.length} 个)</span>
              </div>

              {/* Instance rows */}
              <div className="rounded-lg border border-[#e9ecef] overflow-hidden">
                {group.map((inst, idx) => {
                  const isSelected = selected.has(inst.id)
                  return (
                    <div
                      key={inst.id}
                      onClick={() => toggleInstance(inst.id)}
                      className={`flex items-center gap-3 px-4 py-3 cursor-pointer transition-colors ${
                        idx < group.length - 1 ? 'border-b border-[#e9ecef]' : ''
                      } ${isSelected ? 'bg-[#2ca07a]/4' : 'hover:bg-[#f8f9fa]'}`}
                      style={isSelected ? { backgroundColor: 'rgba(44,160,122,0.04)' } : {}}
                    >
                      {/* Checkbox */}
                      <div
                        className="w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 transition-colors"
                        style={{
                          backgroundColor: isSelected ? '#2ca07a' : 'white',
                          borderColor: isSelected ? '#2ca07a' : '#ced4da',
                        }}
                      >
                        {isSelected && (
                          <span className="material-symbols-outlined text-white" style={{ fontSize: '11px' }}>check</span>
                        )}
                      </div>

                      {/* Icon */}
                      <div className="w-7 h-7 rounded bg-[#f8f9fa] border border-[#e9ecef] flex items-center justify-center flex-shrink-0">
                        <span className="material-symbols-outlined text-[#878a99]" style={{ fontSize: '14px' }}>
                          {type === 'rds' ? 'database' : 'dns'}
                        </span>
                      </div>

                      {/* Info */}
                      <div className="flex-1 min-w-0">
                        <div className="text-[13px] font-medium text-[#495057] truncate">{inst.instance_name || inst.instance_id}</div>
                        <div className="text-[11px] text-[#878a99] font-mono truncate">{inst.instance_id}</div>
                      </div>

                      {/* Meta */}
                      <div className="text-right flex-shrink-0 hidden sm:block">
                        {inst.engine && (
                          <div className="text-[11px] text-[#2ca07a] font-medium">{inst.engine}</div>
                        )}
                        {inst.spec && (
                          <div className="text-[11px] text-[#878a99]">{inst.spec}</div>
                        )}
                      </div>

                      {/* Region */}
                      <div className="flex-shrink-0 hidden md:block">
                        <span className="text-[11px] py-0.5 px-2 bg-[#f8f9fa] border border-[#e9ecef] rounded text-[#878a99]">
                          {inst.region_id}
                        </span>
                      </div>

                      {/* Monitored badge */}
                      {inst.monitored && (
                        <span className="text-[10px] py-0.5 px-2 bg-[#0ab39c]/15 text-[#0ab39c] rounded font-medium flex-shrink-0">
                          已接入
                        </span>
                      )}
                    </div>
                  )
                })}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
