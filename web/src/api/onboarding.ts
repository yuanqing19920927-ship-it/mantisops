import api from './client'
import type {
  ManagedServer,
  CloudAccount,
  CloudInstance,
  CredentialSummary,
  SSHTestRequest,
  SSHTestResult,
  VerifyResult,
  DeleteImpact,
} from '../types/onboarding'

// Managed servers
export async function getManagedServers(): Promise<ManagedServer[]> {
  const { data } = await api.get('/managed-servers')
  return data || []
}

export async function addManagedServer(req: {
  host: string
  ssh_port: number
  ssh_user: string
  auth_type: string
  password?: string
  private_key?: string
  passphrase?: string
  agent_id?: string
  collect_interval?: number
  enable_docker?: boolean
  enable_gpu?: boolean
}): Promise<ManagedServer> {
  const { data } = await api.post('/managed-servers', req)
  return data
}

export async function testSSHConnection(req: SSHTestRequest): Promise<SSHTestResult> {
  const { data } = await api.post('/managed-servers/test-connection', req)
  return data
}

export async function deployAgent(id: number): Promise<void> {
  await api.post(`/managed-servers/${id}/deploy`)
}

export async function retryDeploy(id: number): Promise<void> {
  await api.post(`/managed-servers/${id}/retry`)
}

export async function deleteManagedServer(id: number): Promise<void> {
  await api.delete(`/managed-servers/${id}`)
}

// Cloud accounts
export async function getCloudAccounts(): Promise<CloudAccount[]> {
  const { data } = await api.get('/cloud-accounts')
  return data || []
}

export async function addCloudAccount(req: {
  name: string
  provider: string
  access_key_id: string
  access_key_secret: string
  region_ids?: string[]
  auto_discover?: boolean
}): Promise<CloudAccount> {
  const { data } = await api.post('/cloud-accounts', req)
  return data
}

export async function updateCloudAccount(
  id: number,
  req: Partial<{
    name: string
    region_ids: string[]
    auto_discover: boolean
  }>
): Promise<void> {
  await api.put(`/cloud-accounts/${id}`, req)
}

export async function deleteCloudAccount(id: number, force = false): Promise<DeleteImpact | null> {
  const { data } = await api.delete(`/cloud-accounts/${id}`, { params: { force } })
  return data
}

export async function verifyAK(req: {
  access_key_id: string
  access_key_secret: string
}): Promise<VerifyResult> {
  const { data } = await api.post('/cloud-accounts/verify', req)
  return data
}

export async function syncCloudAccount(id: number): Promise<void> {
  await api.post(`/cloud-accounts/${id}/sync`)
}

export async function getCloudInstances(accountId: number): Promise<CloudInstance[]> {
  const { data } = await api.get(`/cloud-accounts/${accountId}/instances`)
  return data || []
}

export async function confirmCloudInstances(ids: number[]): Promise<void> {
  await api.post('/cloud-instances/confirm', { instance_ids: ids })
}

export async function deleteCloudInstance(id: number, force = false): Promise<DeleteImpact | null> {
  const { data } = await api.delete(`/cloud-instances/${id}`, { params: { force } })
  return data
}

// Credentials
export async function getCredentials(): Promise<CredentialSummary[]> {
  const { data } = await api.get('/credentials')
  return data || []
}
