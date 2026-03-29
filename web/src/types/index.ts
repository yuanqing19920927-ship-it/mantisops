export interface ServerGroup {
  id: number
  name: string
  sort_order: number
  server_count?: number
}

export interface Server {
  id: number
  host_id: string
  hostname: string
  ip_addresses: string
  os: string
  kernel: string
  arch: string
  agent_version: string
  cpu_cores: number
  cpu_model: string
  memory_total: number
  disk_total: number
  gpu_model: string
  gpu_memory: number
  boot_time: number
  last_seen: number
  status: 'online' | 'offline'
  display_name: string
  sort_order: number
  group_id?: number | null
  collect_docker?: boolean | null
  collect_gpu?: boolean | null
  probe_auto_scan?: boolean | null
}

export interface CpuMetrics {
  usage_percent: number
  load1: number
  load5: number
  load15: number
  cores: number
}

export interface MemoryMetrics {
  total: number
  used: number
  available: number
  usage_percent: number
  swap_total: number
  swap_used: number
}

export interface DiskMetrics {
  mount_point: string
  device: string
  fs_type: string
  total: number
  used: number
  usage_percent: number
}

export interface NetworkMetrics {
  interface: string
  rx_bytes_per_sec: number
  tx_bytes_per_sec: number
  rx_bytes_total: number
  tx_bytes_total: number
}

export interface DockerMetrics {
  container_id: string
  name: string
  image: string
  state: string
  status: string
  cpu_percent: number
  memory_usage: number
  memory_limit: number
  ports: string[]
}

export interface GpuMetrics {
  name: string
  usage_percent: number
  memory_used: number
  memory_total: number
  temperature: number
}

export interface MetricsPayload {
  host_id: string
  timestamp: number
  cpu?: CpuMetrics
  memory?: MemoryMetrics
  disks?: DiskMetrics[]
  networks?: NetworkMetrics[]
  containers?: DockerMetrics[]
  gpu?: GpuMetrics
}

export interface DashboardData {
  servers_online: number
  servers_total: number
  servers: Server[]
  metrics?: Record<string, MetricsPayload>
  groups?: ServerGroup[]
}

export interface ProbeResult {
  rule_id: number
  name: string
  host: string
  port: number
  status: 'up' | 'down'
  latency_ms: number
  checked_at: number
  error?: string
  http_status?: number
  ssl_expiry_days?: number
  ssl_issuer?: string
  ssl_expiry_date?: string
}

// Alert types
export interface AlertRule {
  id?: number
  name: string
  type: string
  target_id: string
  operator: string
  threshold: number
  unit: string
  duration: number
  level: string
  enabled: boolean
  created_at?: string
}

export interface AlertEvent {
  id: number
  rule_id: number
  rule_name: string
  target_id: string
  target_label: string
  level: string
  status: string
  silenced: boolean
  value: number
  message: string
  fired_at: string
  resolved_at?: string
  resolve_type?: string
  acked_at?: string
  acked_by?: string
}

export interface AlertStats {
  firing: number
  firing_unsilenced: number
  today_fired: number
  today_resolved: number
  today_silenced: number
}

export interface NotificationChannel {
  id?: number
  name: string
  type: string
  config: string
  enabled: boolean
  created_at?: string
}

export interface AlertNotificationDetail {
  channel_name: string
  channel_type: string
  notify_type: string
  status: string
  retry_count: number
  last_error: string
  sent_at?: string
}
