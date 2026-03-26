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
}
