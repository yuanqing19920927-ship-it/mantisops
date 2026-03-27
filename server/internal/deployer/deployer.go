package deployer

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"opsboard/server/internal/store"
	"opsboard/server/internal/ws"
)

type InstallOptions struct {
	AgentID         string `json:"agent_id"`
	CollectInterval int    `json:"collect_interval"`
	Docker          bool   `json:"docker"`
	GPU             bool   `json:"gpu"`
}

type TestConnRequest struct {
	Host       string `json:"host"`
	SSHPort    int    `json:"ssh_port"`
	SSHUser    string `json:"ssh_user"`
	AuthType   string `json:"auth_type"` // "ssh_password" | "ssh_key"
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

type TestConnResult struct {
	Success   bool   `json:"success"`
	LatencyMs int    `json:"latency_ms"`
	HostKey   string `json:"host_key"`
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Error     string `json:"error,omitempty"`
}

type Deployer struct {
	managedStore *store.ManagedServerStore
	credStore    *store.CredentialStore
	serverStore  *store.ServerStore
	hub          *ws.Hub
	pskToken     string
	grpcAddr     string
	binaryDir    string
	timeout      time.Duration
	pendingCh    map[string]chan struct{}
	mu           sync.Mutex
}

func NewDeployer(
	ms *store.ManagedServerStore,
	cs *store.CredentialStore,
	ss *store.ServerStore,
	hub *ws.Hub,
	pskToken, grpcAddr, binaryDir string,
	timeoutSec int,
) *Deployer {
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	return &Deployer{
		managedStore: ms,
		credStore:    cs,
		serverStore:  ss,
		hub:          hub,
		pskToken:     pskToken,
		grpcAddr:     grpcAddr,
		binaryDir:    binaryDir,
		timeout:      time.Duration(timeoutSec) * time.Second,
		pendingCh:    make(map[string]chan struct{}),
	}
}

// TestConnection performs a dry-run SSH test without saving anything to DB
func (d *Deployer) TestConnection(req TestConnRequest) (*TestConnResult, error) {
	var client *SSHClient
	if req.AuthType == "ssh_key" {
		var err error
		client, err = NewSSHClientKey(req.Host, req.SSHPort, req.SSHUser, req.PrivateKey, req.Passphrase, "")
		if err != nil {
			return &TestConnResult{Success: false, Error: err.Error()}, nil
		}
	} else {
		client = NewSSHClientPassword(req.Host, req.SSHPort, req.SSHUser, req.Password, "")
	}

	latency, hostKey, arch, osName, err := client.TestConnection()
	if err != nil {
		return &TestConnResult{Success: false, Error: err.Error()}, nil
	}
	return &TestConnResult{
		Success:   true,
		LatencyMs: latency,
		HostKey:   hostKey,
		Arch:      arch,
		OS:        osName,
	}, nil
}

// Deploy starts async agent installation. Uses CAS to prevent concurrent deploys.
func (d *Deployer) Deploy(managedID int) error {
	ok, err := d.managedStore.CASUpdateState(managedID, []string{store.InstallStatePending, store.InstallStateFailed}, store.InstallStateTesting)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("deploy already in progress or server not in deployable state")
	}

	go d.runDeploy(managedID)
	return nil
}

// NotifyRegistered is called by gRPC handler when a new agent registers
func (d *Deployer) NotifyRegistered(hostID string) {
	d.mu.Lock()
	ch, ok := d.pendingCh[hostID]
	d.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (d *Deployer) registerWait(agentID string) chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	ch := make(chan struct{})
	d.pendingCh[agentID] = ch
	return ch
}

func (d *Deployer) unregisterWait(agentID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.pendingCh, agentID)
}

func (d *Deployer) runDeploy(managedID int) {
	managed, err := d.managedStore.Get(managedID)
	if err != nil {
		log.Printf("[deployer] get managed %d: %v", managedID, err)
		return
	}

	cred, err := d.credStore.Get(managed.CredentialID)
	if err != nil {
		d.fail(managedID, "读取凭据失败: "+err.Error())
		return
	}

	var sshClient *SSHClient
	switch cred.Type {
	case "ssh_password":
		sshClient = NewSSHClientPassword(managed.Host, managed.SSHPort, managed.SSHUser, cred.Data["password"], managed.SSHHostKey)
	case "ssh_key":
		sshClient, err = NewSSHClientKey(managed.Host, managed.SSHPort, managed.SSHUser, cred.Data["private_key"], cred.Data["passphrase"], managed.SSHHostKey)
		if err != nil {
			d.fail(managedID, "SSH密钥解析失败: "+err.Error())
			return
		}
	default:
		d.fail(managedID, "不支持的凭据类型: "+cred.Type)
		return
	}

	d.broadcast(managedID, store.InstallStateTesting, "SSH 连通性测试中...")
	latency, _, _, _, err := sshClient.TestConnection()
	if err != nil {
		d.fail(managedID, "SSH连接失败: "+err.Error())
		return
	}
	d.broadcast(managedID, store.InstallStateTesting, fmt.Sprintf("SSH 连接成功 (延迟 %dms)", latency))

	d.managedStore.UpdateState(managedID, store.InstallStateConnected, "")
	d.broadcast(managedID, store.InstallStateConnected, "检查目标环境...")

	if err := sshClient.Connect(); err != nil {
		d.fail(managedID, "SSH连接失败: "+err.Error())
		return
	}
	defer sshClient.Close()

	arch, err := sshClient.DetectArch()
	if err != nil {
		d.fail(managedID, "检测架构失败: "+err.Error())
		return
	}
	d.managedStore.UpdateDetectedArch(managedID, arch)
	d.broadcast(managedID, store.InstallStateConnected, fmt.Sprintf("架构: %s", arch))

	stdout, _, _ := sshClient.Execute("pgrep -x opsboard-agent")
	if strings.TrimSpace(stdout) != "" {
		// Agent already running, stop it first
		sshClient.Execute("sudo systemctl stop opsboard-agent 2>/dev/null || sudo killall opsboard-agent 2>/dev/null")
		d.broadcast(managedID, store.InstallStateConnected, "已停止旧 Agent")
	}

	d.managedStore.UpdateState(managedID, store.InstallStateUploading, "")
	d.broadcast(managedID, store.InstallStateUploading, "上传 Agent 二进制...")

	binName := fmt.Sprintf("opsboard-agent-linux-%s", arch)
	binPath := filepath.Join(d.binaryDir, binName)
	if err := sshClient.Upload(binPath, "/tmp/opsboard-agent"); err != nil {
		d.fail(managedID, "上传失败: "+err.Error())
		return
	}
	d.broadcast(managedID, store.InstallStateUploading, "上传完成")

	d.managedStore.UpdateState(managedID, store.InstallStateInstalling, "")
	d.broadcast(managedID, store.InstallStateInstalling, "安装 Agent...")

	var opts InstallOptions
	if err := json.Unmarshal([]byte(managed.InstallOptions), &opts); err != nil {
		log.Printf("[deployer] invalid install_options for managed %d, using defaults: %v", managedID, err)
	}
	if opts.CollectInterval <= 0 {
		opts.CollectInterval = 5
	}

	agentID := opts.AgentID
	if agentID == "" {
		agentID = generateAgentID(managed.Host)
	}

	agentYAML := fmt.Sprintf(`server:
  address: "%s"
  token: "%s"
  tls:
    enabled: false

collect:
  interval: %d
  docker: %v
  gpu: %v

agent:
  id: "%s"
`, d.grpcAddr, d.pskToken, opts.CollectInterval, opts.Docker, opts.GPU, agentID)

	sshClient.Execute("sudo mkdir -p /etc/opsboard")
	if err := sshClient.WriteFile("/tmp/agent.yaml", []byte(agentYAML), 0644); err != nil {
		d.fail(managedID, "写入配置失败: "+err.Error())
		return
	}

	cmds := []string{
		"sudo mv /tmp/agent.yaml /etc/opsboard/agent.yaml",
		"sudo mv /tmp/opsboard-agent /usr/local/bin/opsboard-agent",
		"sudo chmod +x /usr/local/bin/opsboard-agent",
	}
	for _, cmd := range cmds {
		if _, stderr, err := sshClient.Execute(cmd); err != nil {
			d.fail(managedID, fmt.Sprintf("执行命令失败 [%s]: %s %v", cmd, stderr, err))
			return
		}
	}

	serviceUnit := `[Unit]
Description=OpsBoard Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/opsboard-agent -config /etc/opsboard/agent.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	if err := sshClient.WriteFile("/tmp/opsboard-agent.service", []byte(serviceUnit), 0644); err != nil {
		d.fail(managedID, "写入 systemd 服务文件失败: "+err.Error())
		return
	}

	startCmds := []string{
		"sudo mv /tmp/opsboard-agent.service /etc/systemd/system/opsboard-agent.service",
		"sudo systemctl daemon-reload",
		"sudo systemctl enable opsboard-agent",
		"sudo systemctl start opsboard-agent",
	}
	for _, cmd := range startCmds {
		if _, stderr, err := sshClient.Execute(cmd); err != nil {
			d.fail(managedID, fmt.Sprintf("启动服务失败 [%s]: %s %v", cmd, stderr, err))
			return
		}
	}
	d.broadcast(managedID, store.InstallStateInstalling, "Agent 已启动")

	d.managedStore.UpdateState(managedID, store.InstallStateWaiting, "")
	d.broadcast(managedID, store.InstallStateWaiting, "等待 Agent 注册...")

	waitCh := d.registerWait(agentID)
	defer d.unregisterWait(agentID)

	select {
	case <-waitCh:
		d.managedStore.UpdateState(managedID, store.InstallStateOnline, "")
		d.managedStore.UpdateAgentInfo(managedID, agentID, "")
		d.broadcast(managedID, store.InstallStateOnline, "Agent 已在线")
		log.Printf("[deployer] agent %s registered for managed server %d", agentID, managedID)
	case <-time.After(d.timeout):
		d.fail(managedID, "Agent注册超时，请检查网络和防火墙")
	}
}

func (d *Deployer) fail(managedID int, errMsg string) {
	d.managedStore.UpdateState(managedID, store.InstallStateFailed, errMsg)
	d.broadcast(managedID, store.InstallStateFailed, errMsg)
	log.Printf("[deployer] failed managed=%d: %s", managedID, errMsg)
}

func (d *Deployer) broadcast(managedID int, state, message string) {
	d.hub.BroadcastJSON(map[string]interface{}{
		"type":       "deploy_progress",
		"managed_id": managedID,
		"state":      state,
		"message":    message,
		"timestamp":  time.Now().Unix(),
	})
}

func generateAgentID(host string) string {
	// Extract last octet or hostname for a readable ID
	parts := strings.Split(host, ".")
	if len(parts) == 4 {
		return fmt.Sprintf("srv-%s", parts[3])
	}
	return fmt.Sprintf("srv-%s", host)
}
