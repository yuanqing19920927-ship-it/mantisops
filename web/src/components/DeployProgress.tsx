// Usage:
// <DeployProgress state="installing" message="正在安装 Agent..." />

const STEPS = [
  { key: 'testing', label: '测试连接' },
  { key: 'connected', label: '连接成功' },
  { key: 'uploading', label: '上传文件' },
  { key: 'installing', label: '安装中' },
  { key: 'waiting', label: '等待上线' },
  { key: 'online', label: '已上线' },
]

const STATE_ORDER: Record<string, number> = {
  pending: -1,
  testing: 0,
  connected: 1,
  uploading: 2,
  installing: 3,
  waiting: 4,
  online: 5,
  failed: 99,
}

interface Props {
  state: string
  message?: string
}

export function DeployProgress({ state, message }: Props) {
  const currentOrder = STATE_ORDER[state] ?? -1
  const isFailed = state === 'failed'

  return (
    <div className="mt-4">
      {/* Step indicators */}
      <div className="flex items-center gap-0">
        {STEPS.map((step, idx) => {
          const stepOrder = STATE_ORDER[step.key] ?? idx
          const isDone = !isFailed && currentOrder > stepOrder
          const isCurrent = !isFailed && currentOrder === stepOrder

          return (
            <div key={step.key} className="flex items-center flex-1 min-w-0">
              {/* Step node */}
              <div className="flex flex-col items-center flex-shrink-0">
                <div
                  className={`w-7 h-7 rounded-full flex items-center justify-center text-[12px] font-bold transition-all ${
                    isDone
                      ? 'bg-[#0ab39c] text-white'
                      : isCurrent
                      ? 'bg-[#2ca07a] text-white ring-2 ring-[#2ca07a]/30'
                      : isFailed && idx === STEPS.findIndex((s) => STATE_ORDER[s.key] === currentOrder)
                      ? 'bg-[#f06548] text-white'
                      : 'bg-[#eff2f7] text-[#ced4da]'
                  }`}
                >
                  {isDone ? (
                    <span className="material-symbols-outlined" style={{ fontSize: '14px' }}>check</span>
                  ) : isCurrent ? (
                    <span className="w-2.5 h-2.5 rounded-full bg-white animate-pulse" />
                  ) : (
                    <span className="w-2 h-2 rounded-full bg-[#ced4da]" />
                  )}
                </div>
                <span
                  className={`text-[10px] mt-1 whitespace-nowrap font-medium ${
                    isDone
                      ? 'text-[#0ab39c]'
                      : isCurrent
                      ? 'text-[#2ca07a]'
                      : 'text-[#ced4da]'
                  }`}
                >
                  {step.label}
                </span>
              </div>

              {/* Connector line (not after last step) */}
              {idx < STEPS.length - 1 && (
                <div
                  className={`flex-1 h-[2px] mx-1 rounded transition-colors ${
                    !isFailed && currentOrder > stepOrder ? 'bg-[#0ab39c]' : 'bg-[#eff2f7]'
                  }`}
                />
              )}
            </div>
          )
        })}
      </div>

      {/* Failed state */}
      {isFailed && (
        <div className="mt-3 flex items-center gap-2 px-3 py-2 bg-[#f06548]/8 border border-[#f06548]/25 rounded-lg"
          style={{ backgroundColor: 'rgba(240,101,72,0.06)' }}>
          <span className="material-symbols-outlined text-[#f06548] text-[16px] flex-shrink-0">error</span>
          <span className="text-[12px] text-[#f06548]">{message || '安装失败'}</span>
        </div>
      )}

      {/* Current message */}
      {!isFailed && message && state !== 'online' && (
        <div className="mt-3 flex items-center gap-2 px-3 py-2 bg-[#2ca07a]/6 border border-[#2ca07a]/20 rounded-lg"
          style={{ backgroundColor: 'rgba(44,160,122,0.05)' }}>
          <span className="w-3 h-3 rounded-full bg-[#2ca07a] animate-pulse flex-shrink-0" />
          <span className="text-[12px] text-[#2ca07a]">{message}</span>
        </div>
      )}

      {/* Online success */}
      {state === 'online' && (
        <div className="mt-3 flex items-center gap-2 px-3 py-2 bg-[#0ab39c]/8 border border-[#0ab39c]/25 rounded-lg"
          style={{ backgroundColor: 'rgba(10,179,156,0.06)' }}>
          <span className="material-symbols-outlined text-[#0ab39c] text-[16px] flex-shrink-0">check_circle</span>
          <span className="text-[12px] text-[#0ab39c]">{message || 'Agent 已成功上线，开始采集数据'}</span>
        </div>
      )}
    </div>
  )
}
