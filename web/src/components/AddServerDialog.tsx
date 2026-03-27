// Usage:
// <AddServerDialog open={showDialog} onClose={() => setShowDialog(false)} onSuccess={() => fetchServers()} />

import { useState, useEffect, useRef } from 'react'
import { DeployProgress } from './DeployProgress'
import { testSSHConnection, addManagedServer, deployAgent } from '../api/onboarding'
import type { SSHTestResult, DeployProgress as DeployProgressMsg } from '../types/onboarding'

interface Props {
  open: boolean
  onClose: () => void
  onSuccess?: () => void
}

interface FormState {
  host: string
  ssh_port: string
  ssh_user: string
  auth_type: 'password' | 'key'
  password: string
  private_key: string
  passphrase: string
  agent_id: string
  collect_interval: string
  enable_docker: boolean
  enable_gpu: boolean
}

const DEFAULT_FORM: FormState = {
  host: '',
  ssh_port: '22',
  ssh_user: 'root',
  auth_type: 'password',
  password: '',
  private_key: '',
  passphrase: '',
  agent_id: '',
  collect_interval: '15',
  enable_docker: false,
  enable_gpu: false,
}

type Phase = 'form' | 'deploying' | 'done'

export function AddServerDialog({ open, onClose, onSuccess }: Props) {
  const [form, setForm] = useState<FormState>(DEFAULT_FORM)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<SSHTestResult | null>(null)
  const [testError, setTestError] = useState<string | null>(null)
  const [installing, setInstalling] = useState(false)
  const [phase, setPhase] = useState<Phase>('form')
  const [deployState, setDeployState] = useState('')
  const [deployMessage, setDeployMessage] = useState('')
  const [managedId, setManagedId] = useState<number | null>(null)
  const deployStateRef = useRef(deployState)
  deployStateRef.current = deployState

  // Subscribe to deploy_progress events from WebSocket
  useEffect(() => {
    if (phase !== 'deploying' || managedId === null) return

    const handler = (e: Event) => {
      const msg = (e as CustomEvent<DeployProgressMsg>).detail
      if (msg.managed_id !== managedId) return
      setDeployState(msg.state)
      setDeployMessage(msg.message)
      if (msg.state === 'online') {
        setPhase('done')
        onSuccess?.()
      }
      if (msg.state === 'failed') {
        setPhase('deploying') // keep on deploying phase to show error
      }
    }

    window.addEventListener('deploy_progress', handler)
    return () => window.removeEventListener('deploy_progress', handler)
  }, [phase, managedId, onSuccess])

  if (!open) return null

  const setField = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }))
    // Reset test result when form changes
    setTestResult(null)
    setTestError(null)
  }

  const handleTest = async () => {
    setTesting(true)
    setTestResult(null)
    setTestError(null)
    try {
      const result = await testSSHConnection({
        host: form.host.trim(),
        ssh_port: Number(form.ssh_port) || 22,
        ssh_user: form.ssh_user.trim(),
        auth_type: form.auth_type,
        password: form.auth_type === 'password' ? form.password : undefined,
        private_key: form.auth_type === 'key' ? form.private_key : undefined,
        passphrase: form.auth_type === 'key' ? form.passphrase : undefined,
      })
      setTestResult(result)
      if (!result.success) setTestError(result.error || '连接失败')
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '请求失败，请检查网络'
      setTestError(msg)
    } finally {
      setTesting(false)
    }
  }

  const handleInstall = async () => {
    setInstalling(true)
    try {
      const managed = await addManagedServer({
        host: form.host.trim(),
        ssh_port: Number(form.ssh_port) || 22,
        ssh_user: form.ssh_user.trim(),
        auth_type: form.auth_type,
        password: form.auth_type === 'password' ? form.password : undefined,
        private_key: form.auth_type === 'key' ? form.private_key : undefined,
        passphrase: form.auth_type === 'key' ? form.passphrase : undefined,
        agent_id: form.agent_id.trim() || undefined,
        collect_interval: Number(form.collect_interval) || 15,
        enable_docker: form.enable_docker,
        enable_gpu: form.enable_gpu,
      })
      setManagedId(managed.id)
      setDeployState('testing')
      setDeployMessage('正在测试 SSH 连接...')
      setPhase('deploying')
      await deployAgent(managed.id)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '创建失败，请重试'
      setTestError(msg)
      setInstalling(false)
    }
  }

  const handleClose = () => {
    setForm(DEFAULT_FORM)
    setTestResult(null)
    setTestError(null)
    setPhase('form')
    setDeployState('')
    setDeployMessage('')
    setManagedId(null)
    setInstalling(false)
    onClose()
  }

  const canTest = form.host.trim() && form.ssh_user.trim() &&
    (form.auth_type === 'password' ? form.password : form.private_key)
  const canInstall = testResult?.success

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* Overlay */}
      <div
        className="absolute inset-0 bg-black/40 backdrop-blur-[2px]"
        onClick={phase === 'form' ? handleClose : undefined}
      />

      {/* Dialog */}
      <div className="relative bg-white rounded-[14px] shadow-2xl w-full max-w-lg max-h-[90vh] overflow-y-auto border border-[#e9ecef]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#e9ecef]">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-lg bg-[#2ca07a]/12 flex items-center justify-center"
              style={{ backgroundColor: 'rgba(44,160,122,0.1)' }}>
              <span className="material-symbols-outlined text-[#2ca07a] text-[18px]">dns</span>
            </div>
            <h2 className="text-[16px] font-semibold text-[#495057]">添加服务器</h2>
          </div>
          <button
            onClick={handleClose}
            className="w-7 h-7 flex items-center justify-center rounded hover:bg-[#f8f9fa] text-[#878a99] hover:text-[#495057] transition-colors"
          >
            <span className="material-symbols-outlined text-[18px]">close</span>
          </button>
        </div>

        {/* Body */}
        <div className="px-6 py-5">
          {phase === 'form' && (
            <div className="space-y-4">
              {/* Host & Port */}
              <div className="flex gap-3">
                <div className="flex-1">
                  <label className="block text-[12px] font-medium text-[#495057] mb-1.5">
                    主机地址 <span className="text-[#f06548]">*</span>
                  </label>
                  <input
                    type="text"
                    value={form.host}
                    onChange={(e) => setField('host', e.target.value)}
                    placeholder="192.168.1.100 或 example.com"
                    className="w-full text-[13px] px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                  />
                </div>
                <div className="w-24">
                  <label className="block text-[12px] font-medium text-[#495057] mb-1.5">SSH 端口</label>
                  <input
                    type="number"
                    value={form.ssh_port}
                    onChange={(e) => setField('ssh_port', e.target.value)}
                    className="w-full text-[13px] px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                  />
                </div>
              </div>

              {/* SSH User */}
              <div>
                <label className="block text-[12px] font-medium text-[#495057] mb-1.5">
                  SSH 用户名 <span className="text-[#f06548]">*</span>
                </label>
                <input
                  type="text"
                  value={form.ssh_user}
                  onChange={(e) => setField('ssh_user', e.target.value)}
                  placeholder="root"
                  className="w-full text-[13px] px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                />
              </div>

              {/* Auth Type */}
              <div>
                <label className="block text-[12px] font-medium text-[#495057] mb-1.5">认证方式</label>
                <div className="flex gap-2">
                  {(['password', 'key'] as const).map((t) => (
                    <button
                      key={t}
                      onClick={() => setField('auth_type', t)}
                      className={`flex-1 py-2 text-[12px] rounded-lg border font-medium transition-colors ${
                        form.auth_type === t
                          ? 'bg-[#2ca07a] border-[#2ca07a] text-white'
                          : 'bg-[#f8f9fa] border-[#e9ecef] text-[#878a99] hover:border-[#2ca07a] hover:text-[#2ca07a]'
                      }`}
                    >
                      {t === 'password' ? '密码' : 'SSH 密钥'}
                    </button>
                  ))}
                </div>
              </div>

              {/* Password */}
              {form.auth_type === 'password' && (
                <div>
                  <label className="block text-[12px] font-medium text-[#495057] mb-1.5">
                    密码 <span className="text-[#f06548]">*</span>
                  </label>
                  <input
                    type="password"
                    value={form.password}
                    onChange={(e) => setField('password', e.target.value)}
                    placeholder="SSH 登录密码"
                    className="w-full text-[13px] px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                  />
                </div>
              )}

              {/* Private Key */}
              {form.auth_type === 'key' && (
                <>
                  <div>
                    <label className="block text-[12px] font-medium text-[#495057] mb-1.5">
                      私钥内容 <span className="text-[#f06548]">*</span>
                    </label>
                    <textarea
                      value={form.private_key}
                      onChange={(e) => setField('private_key', e.target.value)}
                      placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                      rows={5}
                      className="w-full text-[12px] font-mono px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors resize-none"
                    />
                  </div>
                  <div>
                    <label className="block text-[12px] font-medium text-[#495057] mb-1.5">私钥密码（可选）</label>
                    <input
                      type="password"
                      value={form.passphrase}
                      onChange={(e) => setField('passphrase', e.target.value)}
                      placeholder="如私钥有密码保护则填写"
                      className="w-full text-[13px] px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                    />
                  </div>
                </>
              )}

              {/* Test Result */}
              {testResult?.success && (
                <div className="flex items-start gap-2 px-3 py-2.5 bg-[#0ab39c]/8 border border-[#0ab39c]/25 rounded-lg"
                  style={{ backgroundColor: 'rgba(10,179,156,0.06)' }}>
                  <span className="material-symbols-outlined text-[#0ab39c] text-[16px] mt-0.5 flex-shrink-0">check_circle</span>
                  <div className="text-[12px] text-[#0ab39c]">
                    连接成功 · 延迟 {testResult.latency_ms}ms · {testResult.arch} · {testResult.os}
                  </div>
                </div>
              )}

              {testError && (
                <div className="flex items-start gap-2 px-3 py-2.5 bg-[#f06548]/8 border border-[#f06548]/25 rounded-lg"
                  style={{ backgroundColor: 'rgba(240,101,72,0.06)' }}>
                  <span className="material-symbols-outlined text-[#f06548] text-[16px] mt-0.5 flex-shrink-0">error</span>
                  <div className="text-[12px] text-[#f06548]">{testError}</div>
                </div>
              )}

              {/* Advanced Options */}
              <div>
                <button
                  onClick={() => setShowAdvanced(!showAdvanced)}
                  className="flex items-center gap-1 text-[12px] text-[#878a99] hover:text-[#495057] transition-colors"
                >
                  <span
                    className={`material-symbols-outlined text-[14px] transition-transform ${showAdvanced ? 'rotate-90' : ''}`}
                  >
                    chevron_right
                  </span>
                  高级选项
                </button>

                {showAdvanced && (
                  <div className="mt-3 space-y-3 pl-4 border-l-2 border-[#e9ecef]">
                    <div>
                      <label className="block text-[12px] font-medium text-[#495057] mb-1.5">
                        Agent ID（自定义，留空则自动生成）
                      </label>
                      <input
                        type="text"
                        value={form.agent_id}
                        onChange={(e) => setField('agent_id', e.target.value)}
                        placeholder="自定义 Agent 标识符"
                        className="w-full text-[13px] px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] placeholder:text-[#ced4da] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                      />
                    </div>
                    <div>
                      <label className="block text-[12px] font-medium text-[#495057] mb-1.5">
                        采集间隔（秒）
                      </label>
                      <input
                        type="number"
                        value={form.collect_interval}
                        onChange={(e) => setField('collect_interval', e.target.value)}
                        min={5}
                        max={300}
                        className="w-32 text-[13px] px-3 py-2 bg-[#f8f9fa] border border-[#e9ecef] rounded-lg text-[#495057] focus:outline-none focus:border-[#2ca07a] focus:bg-white transition-colors"
                      />
                    </div>
                    <div className="flex items-center gap-4">
                      <label className="flex items-center gap-2 cursor-pointer">
                        <div
                          onClick={() => setField('enable_docker', !form.enable_docker)}
                          className={`w-9 h-5 rounded-full transition-colors relative cursor-pointer ${
                            form.enable_docker ? 'bg-[#2ca07a]' : 'bg-[#ced4da]'
                          }`}
                        >
                          <span
                            className={`absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform ${
                              form.enable_docker ? 'translate-x-4' : ''
                            }`}
                          />
                        </div>
                        <span className="text-[12px] text-[#495057]">启用 Docker 监控</span>
                      </label>
                      <label className="flex items-center gap-2 cursor-pointer">
                        <div
                          onClick={() => setField('enable_gpu', !form.enable_gpu)}
                          className={`w-9 h-5 rounded-full transition-colors relative cursor-pointer ${
                            form.enable_gpu ? 'bg-[#2ca07a]' : 'bg-[#ced4da]'
                          }`}
                        >
                          <span
                            className={`absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform ${
                              form.enable_gpu ? 'translate-x-4' : ''
                            }`}
                          />
                        </div>
                        <span className="text-[12px] text-[#495057]">启用 GPU 监控</span>
                      </label>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Deploying Phase */}
          {(phase === 'deploying' || phase === 'done') && (
            <div>
              <p className="text-[13px] text-[#495057] mb-1">
                正在部署 Agent 到 <span className="font-mono font-semibold">{form.host}</span>
              </p>
              <p className="text-[12px] text-[#878a99] mb-4">请勿关闭此窗口，部署完成后 Agent 将自动开始采集数据</p>
              <DeployProgress state={deployState} message={deployMessage} />
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-6 py-4 border-t border-[#e9ecef] flex items-center justify-between gap-3">
          {phase === 'form' && (
            <>
              <button
                onClick={handleClose}
                className="px-4 py-2 text-[13px] text-[#878a99] hover:text-[#495057] transition-colors"
              >
                取消
              </button>
              <div className="flex items-center gap-2">
                <button
                  onClick={handleTest}
                  disabled={!canTest || testing}
                  className="flex items-center gap-1.5 px-4 py-2 text-[13px] border border-[#2ca07a] text-[#2ca07a] rounded-lg hover:bg-[#2ca07a]/5 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  {testing ? (
                    <span className="w-3.5 h-3.5 border border-[#2ca07a]/30 border-t-[#2ca07a] rounded-full animate-spin" />
                  ) : (
                    <span className="material-symbols-outlined text-[15px]">network_check</span>
                  )}
                  {testing ? '测试中...' : '测试连接'}
                </button>
                <button
                  onClick={handleInstall}
                  disabled={!canInstall || installing}
                  className="flex items-center gap-1.5 px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded-lg disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  {installing ? (
                    <span className="w-3.5 h-3.5 border border-white/30 border-t-white rounded-full animate-spin" />
                  ) : (
                    <span className="material-symbols-outlined text-[15px]">download</span>
                  )}
                  安装
                </button>
              </div>
            </>
          )}

          {phase === 'deploying' && (
            <>
              <span className="text-[12px] text-[#878a99]">部署进行中，请稍候...</span>
              {deployState === 'failed' && (
                <button
                  onClick={handleClose}
                  className="px-4 py-2 text-[13px] text-[#878a99] hover:text-[#495057] transition-colors"
                >
                  关闭
                </button>
              )}
            </>
          )}

          {phase === 'done' && (
            <>
              <span className="text-[12px] text-[#0ab39c] flex items-center gap-1">
                <span className="material-symbols-outlined text-[14px]">check_circle</span>
                Agent 部署成功
              </span>
              <button
                onClick={handleClose}
                className="px-4 py-2 text-[13px] bg-[#2ca07a] hover:bg-[#1f7d5e] text-white rounded-lg transition-colors"
              >
                完成
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
