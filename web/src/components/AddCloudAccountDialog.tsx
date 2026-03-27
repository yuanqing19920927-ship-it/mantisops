// Usage:
// <AddCloudAccountDialog open={show} onClose={() => setShow(false)} onSuccess={() => refetch()} />

import { useState, useEffect } from 'react'
import { InstanceSelector } from './InstanceSelector'
import {
  verifyAK,
  addCloudAccount,
  syncCloudAccount,
  getCloudInstances,
  confirmCloudInstances,
} from '../api/onboarding'
import type { VerifyResult, CloudAccount, CloudInstance, CloudSyncProgress } from '../types/onboarding'

interface Props {
  open: boolean
  onClose: () => void
  onSuccess?: () => void
}

type Step = 'form' | 'verify' | 'saving' | 'instances'

const ALIYUN_REGIONS = [
  { id: 'cn-hangzhou', name: '华东1（杭州）' },
  { id: 'cn-shanghai', name: '华东2（上海）' },
  { id: 'cn-beijing', name: '华北2（北京）' },
  { id: 'cn-shenzhen', name: '华南1（深圳）' },
  { id: 'cn-guangzhou', name: '华南2（广州）' },
  { id: 'cn-chengdu', name: '西南1（成都）' },
  { id: 'cn-zhangjiakou', name: '华北3（张家口）' },
  { id: 'cn-huhehaote', name: '华北5（呼和浩特）' },
  { id: 'cn-wulanchabu', name: '华北6（乌兰察布）' },
  { id: 'cn-hongkong', name: '中国香港' },
  { id: 'ap-southeast-1', name: '亚太东南1（新加坡）' },
]

export function AddCloudAccountDialog({ open, onClose, onSuccess }: Props) {
  const [step, setStep] = useState<Step>('form')

  // Form fields
  const [name, setName] = useState('')
  const [akId, setAkId] = useState('')
  const [akSecret, setAkSecret] = useState('')
  const [autoDiscover, setAutoDiscover] = useState(true)
  const [selectedRegions, setSelectedRegions] = useState<string[]>([])
  const [showRegionPicker, setShowRegionPicker] = useState(false)

  // Verify
  const [verifying, setVerifying] = useState(false)
  const [verifyResult, setVerifyResult] = useState<VerifyResult | null>(null)
  const [verifyError, setVerifyError] = useState<string | null>(null)

  // Saving
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [savedAccount, setSavedAccount] = useState<CloudAccount | null>(null)

  // Sync progress
  const [syncMessage, setSyncMessage] = useState('')

  // Instances
  const [instances, setInstances] = useState<CloudInstance[]>([])
  const [loadingInstances, setLoadingInstances] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [confirmDone, setConfirmDone] = useState(false)

  // Listen for cloud_sync_progress events
  useEffect(() => {
    if (!savedAccount) return

    const handler = (e: Event) => {
      const msg = (e as CustomEvent<CloudSyncProgress>).detail
      if (msg.account_id !== savedAccount.id) return
      setSyncMessage(msg.message)
      if (msg.state === 'synced' || msg.state === 'partial') {
        // Fetch discovered instances
        setLoadingInstances(true)
        getCloudInstances(savedAccount.id)
          .then((data) => {
            setInstances(data)
            setStep('instances')
          })
          .finally(() => setLoadingInstances(false))
      }
    }

    window.addEventListener('cloud_sync_progress', handler)
    return () => window.removeEventListener('cloud_sync_progress', handler)
  }, [savedAccount])

  if (!open) return null

  const handleClose = () => {
    setStep('form')
    setName('')
    setAkId('')
    setAkSecret('')
    setAutoDiscover(true)
    setSelectedRegions([])
    setVerifyResult(null)
    setVerifyError(null)
    setSaveError(null)
    setSavedAccount(null)
    setSyncMessage('')
    setInstances([])
    setConfirmDone(false)
    onClose()
  }

  const handleVerify = async () => {
    setVerifying(true)
    setVerifyResult(null)
    setVerifyError(null)
    try {
      const result = await verifyAK({ access_key_id: akId.trim(), access_key_secret: akSecret.trim() })
      setVerifyResult(result)
      setStep('verify')
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '验证失败，请检查 AK 信息'
      setVerifyError(msg)
      setStep('verify')
    } finally {
      setVerifying(false)
    }
  }

  const handleSave = async () => {
    setSaving(true)
    setSaveError(null)
    try {
      const account = await addCloudAccount({
        name: name.trim(),
        provider: 'aliyun',
        access_key_id: akId.trim(),
        access_key_secret: akSecret.trim(),
        region_ids: selectedRegions,
        auto_discover: autoDiscover,
      })
      setSavedAccount(account)
      setStep('saving')
      setSyncMessage('正在同步云资源...')
      await syncCloudAccount(account.id)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '保存失败，请重试'
      setSaveError(msg)
    } finally {
      setSaving(false)
    }
  }

  const handleConfirmInstances = async (ids: number[]) => {
    setConfirming(true)
    try {
      await confirmCloudInstances(ids)
      setConfirmDone(true)
      onSuccess?.()
    } catch (err) {
      console.error('[cloud] confirm instances:', err)
    } finally {
      setConfirming(false)
    }
  }

  const toggleRegion = (id: string) => {
    setSelectedRegions((prev) =>
      prev.includes(id) ? prev.filter((r) => r !== id) : [...prev, id]
    )
  }

  const canVerify = name.trim() && akId.trim() && akSecret.trim()
  const canSave = verifyResult?.valid

  const stepLabels: { key: Step; label: string }[] = [
    { key: 'form', label: '填写信息' },
    { key: 'verify', label: '权限验证' },
    { key: 'saving', label: '保存同步' },
    { key: 'instances', label: '选择实例' },
  ]
  const stepOrder: Record<Step, number> = { form: 0, verify: 1, saving: 2, instances: 3 }
  const currentStepOrder = stepOrder[step]

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/40 backdrop-blur-[2px]"
        onClick={step === 'form' ? handleClose : undefined}
      />

      <div className="relative bg-white rounded-[14px] shadow-2xl w-full max-w-2xl max-h-[90vh] flex flex-col border border-[#e9ecef]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#e9ecef] flex-shrink-0">
          <div className="flex items-center gap-3">
            <div
              className="w-9 h-9 rounded-lg flex items-center justify-center"
              style={{ backgroundColor: 'rgba(44,160,122,0.1)' }}
            >
              <span className="material-symbols-outlined text-[#2ca07a] text-[18px]">cloud</span>
            </div>
            <h2 className="text-[16px] font-semibold text-[#495057]">添加云账号</h2>
          </div>
          <button
            onClick={handleClose}
            className="w-7 h-7 flex items-center justify-center rounded hover:bg-[#f8f9fa] text-[#878a99] hover:text-[#495057] transition-colors"
          >
            <span className="material-symbols-outlined text-[18px]">close</span>
          </button>
        </div>

        {/* Step indicator */}
        <div className="px-6 py-3 border-b border-[#e9ecef] flex items-center gap-0 flex-shrink-0">
          {stepLabels.map((s, idx) => {
            const order = stepOrder[s.key]
            const isDone = currentStepOrder > order
            const isCurrent = currentStepOrder === order
            return (
              <div key={s.key} className="flex items-center flex-1 min-w-0">
                <div className="flex flex-col items-center">
                  <div
                    className={`w-6 h-6 rounded-full flex items-center justify-center text-[11px] font-bold transition-all ${
                      isDone
                        ? 'bg-[#0ab39c] text-white'
                        : isCurrent
                        ? 'bg-[#2ca07a] text-white'
                        : 'bg-[#eff2f7] text-[#ced4da]'
                    }`}
                  >
                    {isDone ? (
                      <span className="material-symbols-outlined" style={{ fontSize: '12px' }}>check</span>
                    ) : (
                      idx + 1
                    )}
                  </div>
                  <span
                    className={`text-[10px] mt-1 whitespace-nowrap ${
                      isDone ? 'text-[#0ab39c]' : isCurrent ? 'text-[#2ca07a] font-semibold' : 'text-[#ced4da]'
                    }`}
                  >
                    {s.label}
                  </span>
                </div>
                {idx < stepLabels.length - 1 && (
                  <div
                    className={`flex-1 h-[2px] mx-2 mt-[-10px] rounded transition-colors ${
                      currentStepOrder > order ? 'bg-[#0ab39c]' : 'bg-[#eff2f7]'
                    }`}
                  />
                )}
              </div>
            )
          })}
        </div>

        {/* Body - scrollable */}
        <div className="flex-1 overflow-y-auto px-6 py-5">
          {/* Step 1: Form */}
          {step === 'form' && (
            <div className="space-y-4">
              <div>
                <label className="block text-[12px] font-medium text-[#495057] mb-1.5">
                  账号名称 <span className="text-[#f06548]">*</span>
                </label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="如：生产环境-阿里云"
                  className="w-full text-[13px] px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                />
              </div>

              <div>
                <label className="block text-[12px] font-medium text-[#495057] mb-1.5">
                  Access Key ID <span className="text-[#f06548]">*</span>
                </label>
                <input
                  type="text"
                  value={akId}
                  onChange={(e) => setAkId(e.target.value)}
                  placeholder="LTAI5t..."
                  className="w-full text-[13px] font-mono px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                />
              </div>

              <div>
                <label className="block text-[12px] font-medium text-[#495057] mb-1.5">
                  Access Key Secret <span className="text-[#f06548]">*</span>
                </label>
                <input
                  type="password"
                  value={akSecret}
                  onChange={(e) => setAkSecret(e.target.value)}
                  placeholder="AK Secret"
                  className="w-full text-[13px] font-mono px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                />
              </div>

              {/* Auto discover toggle */}
              <div className="flex items-center justify-between py-2 px-3 bg-[#f8f9fa] rounded-lg border border-[#e9ecef]">
                <div>
                  <div className="text-[13px] font-medium text-[#495057]">自动发现实例</div>
                  <div className="text-[11px] text-[#878a99] mt-0.5">定期同步账号下 ECS 和 RDS 实例列表</div>
                </div>
                <div
                  onClick={() => setAutoDiscover(!autoDiscover)}
                  className={`w-10 h-6 rounded-full transition-colors relative cursor-pointer flex-shrink-0 ${
                    autoDiscover ? 'bg-[#2ca07a]' : 'bg-[#ced4da]'
                  }`}
                >
                  <span
                    className={`absolute top-1 left-1 w-4 h-4 rounded-full bg-white shadow transition-transform ${
                      autoDiscover ? 'translate-x-4' : ''
                    }`}
                  />
                </div>
              </div>

              {/* Region filter */}
              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <label className="text-[12px] font-medium text-[#495057]">地域筛选（留空则同步所有地域）</label>
                  <button
                    onClick={() => setShowRegionPicker(!showRegionPicker)}
                    className="text-[11px] text-[#2ca07a] hover:underline"
                  >
                    {showRegionPicker ? '收起' : '选择地域'}
                  </button>
                </div>

                {selectedRegions.length > 0 && (
                  <div className="flex flex-wrap gap-1.5 mb-2">
                    {selectedRegions.map((r) => {
                      const region = ALIYUN_REGIONS.find((rg) => rg.id === r)
                      return (
                        <span
                          key={r}
                          className="flex items-center gap-1 text-[11px] py-0.5 px-2 bg-[#2ca07a]/10 text-[#2ca07a] rounded"
                        >
                          {region?.name || r}
                          <button onClick={() => toggleRegion(r)} className="hover:text-[#f06548]">
                            <span className="material-symbols-outlined" style={{ fontSize: '12px' }}>close</span>
                          </button>
                        </span>
                      )
                    })}
                  </div>
                )}

                {showRegionPicker && (
                  <div className="grid grid-cols-2 gap-1.5 p-3 bg-[#f8f9fa] rounded-lg border border-[#e9ecef]">
                    {ALIYUN_REGIONS.map((r) => {
                      const isSelected = selectedRegions.includes(r.id)
                      return (
                        <button
                          key={r.id}
                          onClick={() => toggleRegion(r.id)}
                          className={`text-left text-[11px] px-2.5 py-1.5 rounded border transition-colors ${
                            isSelected
                              ? 'bg-[#2ca07a] border-[#2ca07a] text-white'
                              : 'bg-white border-[#e9ecef] text-[#495057] hover:border-[#2ca07a] hover:text-[#2ca07a]'
                          }`}
                        >
                          {r.name}
                        </button>
                      )
                    })}
                  </div>
                )}
              </div>

              {/* CloudMonitor notice */}
              <div className="flex items-start gap-2 px-3 py-2.5 bg-[#f7b84b]/8 border border-[#f7b84b]/30 rounded-lg"
                style={{ backgroundColor: 'rgba(247,184,75,0.06)' }}>
                <span className="material-symbols-outlined text-[#f7b84b] text-[15px] mt-0.5 flex-shrink-0">info</span>
                <p className="text-[11px] text-[#878a99] leading-relaxed">
                  如需获取 ECS 完整操作系统级指标，请前往
                  <a
                    href="https://cloudmonitor.console.aliyun.com/"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-[#2ca07a] hover:underline mx-0.5"
                  >
                    阿里云云监控控制台
                  </a>
                  安装监控插件
                </p>
              </div>

              {verifyError && (
                <div className="flex items-start gap-2 px-3 py-2.5 bg-[#f06548]/8 border border-[#f06548]/25 rounded-lg"
                  style={{ backgroundColor: 'rgba(240,101,72,0.06)' }}>
                  <span className="material-symbols-outlined text-[#f06548] text-[16px] mt-0.5 flex-shrink-0">error</span>
                  <div className="text-[12px] text-[#f06548]">{verifyError}</div>
                </div>
              )}
            </div>
          )}

          {/* Step 2: Verify */}
          {step === 'verify' && verifyResult && (
            <div className="space-y-4">
              <div
                className={`flex items-center gap-3 px-4 py-3 rounded-lg border ${
                  verifyResult.valid
                    ? 'bg-[#0ab39c]/6 border-[#0ab39c]/25'
                    : 'bg-[#f06548]/6 border-[#f06548]/25'
                }`}
                style={{
                  backgroundColor: verifyResult.valid
                    ? 'rgba(10,179,156,0.05)'
                    : 'rgba(240,101,72,0.05)',
                }}
              >
                <span
                  className={`material-symbols-outlined text-[22px] flex-shrink-0 ${
                    verifyResult.valid ? 'text-[#0ab39c]' : 'text-[#f06548]'
                  }`}
                >
                  {verifyResult.valid ? 'verified_user' : 'gpp_bad'}
                </span>
                <div>
                  <div className={`text-[13px] font-semibold ${verifyResult.valid ? 'text-[#0ab39c]' : 'text-[#f06548]'}`}>
                    {verifyResult.valid ? 'AK 验证通过' : 'AK 验证失败'}
                  </div>
                  {verifyResult.account_name && (
                    <div className="text-[12px] text-[#878a99] mt-0.5">
                      账号：{verifyResult.account_name} ({verifyResult.account_uid})
                    </div>
                  )}
                </div>
              </div>

              {verifyResult.permissions && verifyResult.permissions.length > 0 && (
                <div>
                  <div className="text-[12px] font-semibold text-[#495057] mb-2">权限检查</div>
                  <div className="space-y-1.5">
                    {verifyResult.permissions.map((perm) => (
                      <div key={perm.action} className="flex items-center gap-2">
                        <span
                          className={`material-symbols-outlined text-[14px] ${
                            perm.allowed ? 'text-[#0ab39c]' : 'text-[#f06548]'
                          }`}
                        >
                          {perm.allowed ? 'check_circle' : 'cancel'}
                        </span>
                        <span className="text-[12px] font-mono text-[#495057]">{perm.action}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {saveError && (
                <div className="flex items-start gap-2 px-3 py-2.5 bg-[#f06548]/8 border border-[#f06548]/25 rounded-lg"
                  style={{ backgroundColor: 'rgba(240,101,72,0.06)' }}>
                  <span className="material-symbols-outlined text-[#f06548] text-[16px] mt-0.5 flex-shrink-0">error</span>
                  <div className="text-[12px] text-[#f06548]">{saveError}</div>
                </div>
              )}
            </div>
          )}

          {/* Step 3: Saving & syncing */}
          {step === 'saving' && (
            <div className="py-4 space-y-4">
              <div className="flex items-center gap-3 px-4 py-3 bg-[#2ca07a]/6 border border-[#2ca07a]/20 rounded-lg"
                style={{ backgroundColor: 'rgba(44,160,122,0.05)' }}>
                <span className="w-4 h-4 border-2 border-[#2ca07a]/30 border-t-[#2ca07a] rounded-full animate-spin flex-shrink-0" />
                <span className="text-[13px] text-[#2ca07a]">{syncMessage || '正在同步云资源，请稍候...'}</span>
              </div>

              {loadingInstances && (
                <div className="flex items-center gap-2 text-[12px] text-[#878a99]">
                  <span className="w-3.5 h-3.5 border border-[#878a99]/30 border-t-[#878a99] rounded-full animate-spin" />
                  加载实例列表...
                </div>
              )}
            </div>
          )}

          {/* Step 4: Instance selection */}
          {step === 'instances' && (
            <div>
              {confirmDone ? (
                <div className="py-6 text-center">
                  <div className="w-14 h-14 rounded-full bg-[#0ab39c]/12 flex items-center justify-center mx-auto mb-4"
                    style={{ backgroundColor: 'rgba(10,179,156,0.1)' }}>
                    <span className="material-symbols-outlined text-[#0ab39c] text-3xl">check_circle</span>
                  </div>
                  <p className="text-[15px] font-semibold text-[#495057] mb-1">接入成功</p>
                  <p className="text-[13px] text-[#878a99]">实例已添加到监控，数据将开始采集</p>
                </div>
              ) : (
                <>
                  <p className="text-[13px] text-[#495057] mb-4">
                    发现 <span className="font-semibold text-[#2ca07a]">{instances.length}</span> 个实例，请选择需要接入监控的实例
                  </p>
                  <InstanceSelector
                    instances={instances}
                    onConfirm={handleConfirmInstances}
                    loading={confirming}
                  />
                </>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-6 py-4 border-t border-[#e9ecef] flex items-center justify-between gap-3 flex-shrink-0">
          {step === 'form' && (
            <>
              <button
                onClick={handleClose}
                className="px-4 py-2 text-[13px] text-[#878a99] hover:text-[#495057] transition-colors"
              >
                取消
              </button>
              <button
                onClick={handleVerify}
                disabled={!canVerify || verifying}
                className="flex items-center gap-1.5 px-5 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded-lg disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                {verifying ? (
                  <span className="w-3.5 h-3.5 border border-white/30 border-t-white rounded-full animate-spin" />
                ) : (
                  <span className="material-symbols-outlined text-[15px]">verified</span>
                )}
                {verifying ? '验证中...' : '验证 AK'}
              </button>
            </>
          )}

          {step === 'verify' && (
            <>
              <button
                onClick={() => { setStep('form'); setVerifyResult(null); setVerifyError(null) }}
                className="flex items-center gap-1 px-4 py-2 text-[13px] text-[#878a99] hover:text-[#495057] transition-colors"
              >
                <span className="material-symbols-outlined text-[15px]">arrow_back</span>
                返回
              </button>
              <button
                onClick={handleSave}
                disabled={!canSave || saving}
                className="flex items-center gap-1.5 px-5 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded-lg disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                {saving ? (
                  <span className="w-3.5 h-3.5 border border-white/30 border-t-white rounded-full animate-spin" />
                ) : (
                  <span className="material-symbols-outlined text-[15px]">save</span>
                )}
                {saving ? '保存中...' : '保存并开始同步'}
              </button>
            </>
          )}

          {step === 'saving' && (
            <span className="text-[12px] text-[#878a99]">同步进行中，请稍候...</span>
          )}

          {step === 'instances' && (
            <>
              {confirmDone ? (
                <button
                  onClick={handleClose}
                  className="ml-auto px-5 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded-lg transition-colors"
                >
                  完成
                </button>
              ) : (
                <button
                  onClick={handleClose}
                  className="px-4 py-2 text-[13px] text-[#878a99] hover:text-[#495057] transition-colors"
                >
                  跳过，稍后添加
                </button>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  )
}
