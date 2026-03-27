export interface ManagedServer {
  id: number
  host: string
  ssh_port: number
  ssh_user: string
  credential_id: number
  detected_arch: string
  ssh_host_key: string
  install_options: string
  install_state: string // pending | testing | connected | uploading | installing | waiting | online | failed
  install_error: string
  agent_host_id: string
  agent_version: string
  created_at: string
  updated_at: string
}

export interface CloudAccount {
  id: number
  name: string
  provider: string
  credential_id: number
  region_ids: string[]
  auto_discover: boolean
  sync_state: string // pending | syncing | synced | partial | failed
  sync_error: string
  last_synced_at: string | null
  created_at: string
  updated_at: string
}

export interface CloudInstance {
  id: number
  cloud_account_id: number
  instance_type: string // ecs | rds
  instance_id: string
  host_id: string
  instance_name: string
  region_id: string
  spec: string
  engine: string
  endpoint: string
  monitored: boolean
  extra: string
  created_at: string
  updated_at: string
}

export interface CredentialSummary {
  id: number
  name: string
  type: string
  created_at: string
  used_by: number
}

export interface SSHTestRequest {
  host: string
  ssh_port: number
  ssh_user: string
  auth_type: string
  password?: string
  private_key?: string
  passphrase?: string
}

export interface SSHTestResult {
  success: boolean
  latency_ms: number
  host_key: string
  arch: string
  os: string
  error?: string
}

export interface VerifyResult {
  valid: boolean
  account_uid: string
  account_name: string
  permissions: { action: string; allowed: boolean }[]
}

export interface DeleteImpact {
  servers: number
  assets: number
  probe_rules: number
  alert_rules: number
  alert_events: number
}

export interface DeployProgress {
  type: 'deploy_progress'
  managed_id: number
  state: string
  message: string
  timestamp: number
}

export interface CloudSyncProgress {
  type: 'cloud_sync_progress'
  account_id: number
  state: string
  message: string
  timestamp: number
}
