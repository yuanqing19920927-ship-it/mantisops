package deployer

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

type InstallOptions struct {
	AgentID         string `json:"agent_id"`
	CollectInterval int    `json:"collect_interval"`
	Docker          *bool  `json:"docker"`
	EnableDocker    *bool  `json:"enable_docker"` // alias from frontend API
	GPU             *bool  `json:"gpu"`
	EnableGPU       *bool  `json:"enable_gpu"` // alias from frontend API
}

// dockerFlag returns the YAML fragment for docker config.
// nil means omit (let agent auto-detect).
func (o *InstallOptions) dockerFlag() *bool {
	if o.EnableDocker != nil {
		return o.EnableDocker
	}
	return o.Docker
}

// gpuFlag returns the YAML fragment for gpu config.
func (o *InstallOptions) gpuFlag() *bool {
	if o.EnableGPU != nil {
		return o.EnableGPU
	}
	return o.GPU
}

// buildAgentYAML generates agent.yaml content.
// docker/gpu nil = omit (let agent auto-detect), non-nil = write explicit value.
func buildAgentYAML(grpcAddr, pskToken string, interval int, docker, gpu *bool, agentID string) string {
	var collectExtra string
	if docker != nil {
		collectExtra += fmt.Sprintf("\n  docker: %v", *docker)
	}
	if gpu != nil {
		collectExtra += fmt.Sprintf("\n  gpu: %v", *gpu)
	}
	return fmt.Sprintf(`server:
  address: "%s"
  token: "%s"
  tls:
    enabled: false

collect:
  interval: %d%s

agent:
  id: "%s"
`, grpcAddr, pskToken, interval, collectExtra, agentID)
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
	ok, err := d.managedStore.CASUpdateState(managedID,
		[]string{store.InstallStatePending, store.InstallStateFailed, store.InstallStateOnline},
		store.InstallStateTesting)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("deploy already in progress or server not in deployable state")
	}

	go d.runDeploy(managedID)
	return nil
}

// NotifyRegistered is called by gRPC handler when a new agent registers.
// It notifies any active deploy goroutine AND resolves stale "waiting" records.
func (d *Deployer) NotifyRegistered(hostID string) {
	// Notify active deploy goroutine
	d.mu.Lock()
	ch, ok := d.pendingCh[hostID]
	d.mu.Unlock()
	if ok {
		close(ch)
		return
	}

	// No active deploy — check DB for stale "waiting" record with this agent_host_id
	d.managedStore.ResolveWaiting(hostID)
}

// RecoverStaleWaiting scans for managed_servers stuck in "waiting" state
// whose agents have already registered. Call once at startup.
func (d *Deployer) RecoverStaleWaiting() {
	d.managedStore.ResolveAllWaiting(d.serverStore)
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

	// Detect sudo availability (try passwordless first, then with password via -S)
	hasSudo := false
	sudoPrefix := "sudo"
	if _, _, err := sshClient.Execute("sudo -n true 2>/dev/null"); err == nil {
		hasSudo = true
		sudoPrefix = "sudo"
	} else if sshClient.password != "" {
		// Try sudo with password piped via stdin
		if _, _, err := sshClient.Execute(fmt.Sprintf("echo '%s' | sudo -S true 2>/dev/null", sshClient.password)); err == nil {
			hasSudo = true
			sudoPrefix = fmt.Sprintf("echo '%s' | sudo -S", sshClient.password)
		}
	}

	// sudo helper: wraps command with the appropriate sudo invocation
	sudo := func(cmd string) string {
		return sudoPrefix + " " + cmd
	}

	if hasSudo {
		d.broadcast(managedID, store.InstallStateConnected, "检测到 sudo，使用系统级安装")
	} else {
		d.broadcast(managedID, store.InstallStateConnected, "无 sudo 权限，使用用户级安装")
	}

	// Stop old agent if running
	stdout, _, _ := sshClient.Execute("pgrep -x mantisops-agent")
	if strings.TrimSpace(stdout) != "" {
		if hasSudo {
			sshClient.Execute(sudo("systemctl stop mantisops-agent 2>/dev/null") + " || " + sudo("killall mantisops-agent 2>/dev/null"))
		} else {
			sshClient.Execute("pkill -x mantisops-agent 2>/dev/null")
		}
		d.broadcast(managedID, store.InstallStateConnected, "已停止旧 Agent")
	}

	d.managedStore.UpdateState(managedID, store.InstallStateUploading, "")
	d.broadcast(managedID, store.InstallStateUploading, "上传 Agent 二进制...")

	binName := fmt.Sprintf("mantisops-agent-linux-%s", arch)
	binPath := filepath.Join(d.binaryDir, binName)
	if err := sshClient.Upload(binPath, "/tmp/mantisops-agent"); err != nil {
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

	// Override with latest config from servers table (set via detail page config dialog)
	if managed.AgentHostID != "" {
		if srv, err := d.serverStore.GetByHostID(managed.AgentHostID); err == nil {
			if srv.CollectDocker != nil {
				opts.EnableDocker = srv.CollectDocker
			}
			if srv.CollectGPU != nil {
				opts.EnableGPU = srv.CollectGPU
			}
		}
	}

	if hasSudo {
		// ── System-level install (sudo) ──
		agentYAML := buildAgentYAML(d.grpcAddr, d.pskToken, opts.CollectInterval, opts.dockerFlag(), opts.gpuFlag(), agentID)

		sshClient.Execute(sudo("mkdir -p /etc/mantisops"))
		if err := sshClient.WriteFile("/tmp/agent.yaml", []byte(agentYAML), 0644); err != nil {
			d.fail(managedID, "写入配置失败: "+err.Error())
			return
		}

		cmds := []string{
			sudo("mv /tmp/agent.yaml /etc/mantisops/agent.yaml"),
			sudo("mv /tmp/mantisops-agent /usr/local/bin/mantisops-agent"),
			sudo("chmod +x /usr/local/bin/mantisops-agent"),
		}
		for _, cmd := range cmds {
			if _, stderr, err := sshClient.Execute(cmd); err != nil {
				d.fail(managedID, fmt.Sprintf("执行命令失败 [%s]: %s %v", cmd, stderr, err))
				return
			}
		}

		serviceUnit := `[Unit]
Description=MantisOps Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mantisops-agent -config /etc/mantisops/agent.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
		if err := sshClient.WriteFile("/tmp/mantisops-agent.service", []byte(serviceUnit), 0644); err != nil {
			d.fail(managedID, "写入 systemd 服务文件失败: "+err.Error())
			return
		}

		startCmds := []string{
			sudo("mv /tmp/mantisops-agent.service /etc/systemd/system/mantisops-agent.service"),
			sudo("systemctl daemon-reload"),
			sudo("systemctl enable mantisops-agent"),
			sudo("systemctl start mantisops-agent"),
		}
		for _, cmd := range startCmds {
			if _, stderr, err := sshClient.Execute(cmd); err != nil {
				d.fail(managedID, fmt.Sprintf("启动服务失败 [%s]: %s %v", cmd, stderr, err))
				return
			}
		}
	} else {
		// ── User-level install (no sudo) ──
		agentYAML := buildAgentYAML(d.grpcAddr, d.pskToken, opts.CollectInterval, opts.dockerFlag(), opts.gpuFlag(), agentID)

		setupCmds := []string{
			"mkdir -p ~/.local/bin ~/.config/mantisops",
			"mv /tmp/mantisops-agent ~/.local/bin/mantisops-agent",
			"chmod +x ~/.local/bin/mantisops-agent",
		}
		for _, cmd := range setupCmds {
			if _, stderr, err := sshClient.Execute(cmd); err != nil {
				d.fail(managedID, fmt.Sprintf("执行命令失败 [%s]: %s %v", cmd, stderr, err))
				return
			}
		}

		if err := sshClient.WriteFile("/tmp/agent.yaml", []byte(agentYAML), 0644); err != nil {
			d.fail(managedID, "写入配置失败: "+err.Error())
			return
		}
		if _, stderr, err := sshClient.Execute("mv /tmp/agent.yaml ~/.config/mantisops/agent.yaml"); err != nil {
			d.fail(managedID, fmt.Sprintf("移动配置失败: %s %v", stderr, err))
			return
		}

		// Use systemd user service if available, otherwise crontab + nohup
		if _, _, err := sshClient.Execute("systemctl --user status 2>/dev/null"); err == nil {
			// systemd user mode available
			sshClient.Execute("mkdir -p ~/.config/systemd/user")
			userService := `[Unit]
Description=MantisOps Agent

[Service]
Type=simple
ExecStart=%h/.local/bin/mantisops-agent -config %h/.config/mantisops/agent.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`
			if err := sshClient.WriteFile("/tmp/mantisops-agent.service", []byte(userService), 0644); err != nil {
				d.fail(managedID, "写入用户服务文件失败: "+err.Error())
				return
			}
			userCmds := []string{
				"mv /tmp/mantisops-agent.service ~/.config/systemd/user/mantisops-agent.service",
				"systemctl --user daemon-reload",
				"systemctl --user enable mantisops-agent",
				"systemctl --user start mantisops-agent",
			}
			for _, cmd := range userCmds {
				if _, stderr, err := sshClient.Execute(cmd); err != nil {
					d.fail(managedID, fmt.Sprintf("启动用户服务失败 [%s]: %s %v", cmd, stderr, err))
					return
				}
			}
		} else {
			// Fallback: nohup + crontab @reboot
			startScript := `#!/bin/bash
pkill -x mantisops-agent 2>/dev/null
sleep 1
nohup ~/.local/bin/mantisops-agent -config ~/.config/mantisops/agent.yaml >> ~/.config/mantisops/agent.log 2>&1 &
`
			if err := sshClient.WriteFile("/tmp/mantisops-start.sh", []byte(startScript), 0755); err != nil {
				d.fail(managedID, "写入启动脚本失败: "+err.Error())
				return
			}
			fallbackCmds := []string{
				"mv /tmp/mantisops-start.sh ~/.local/bin/mantisops-start.sh",
				"chmod +x ~/.local/bin/mantisops-start.sh",
				"~/.local/bin/mantisops-start.sh",
				`(crontab -l 2>/dev/null | grep -v mantisops-start; echo "@reboot ~/.local/bin/mantisops-start.sh") | crontab -`,
			}
			for _, cmd := range fallbackCmds {
				if _, stderr, err := sshClient.Execute(cmd); err != nil {
					d.fail(managedID, fmt.Sprintf("启动失败 [%s]: %s %v", cmd, stderr, err))
					return
				}
			}
		}
	}
	d.broadcast(managedID, store.InstallStateInstalling, "Agent 已启动")

	d.managedStore.UpdateState(managedID, store.InstallStateWaiting, "")
	d.broadcast(managedID, store.InstallStateWaiting, "等待 Agent 注册...")

	waitCh := d.registerWait(agentID)
	defer d.unregisterWait(agentID)

	// Periodically check agent health while waiting for registration
	healthTicker := time.NewTicker(5 * time.Second)
	defer healthTicker.Stop()

	for {
		select {
		case <-waitCh:
			d.managedStore.UpdateState(managedID, store.InstallStateOnline, "")
			d.managedStore.UpdateAgentInfo(managedID, agentID, "")
			d.broadcast(managedID, store.InstallStateOnline, "Agent 已在线")
			log.Printf("[deployer] agent %s registered for managed server %d", agentID, managedID)
			return
		case <-time.After(d.timeout):
			d.fail(managedID, "Agent注册超时，请检查网络和防火墙")
			return
		case <-healthTicker.C:
			// Check if agent process is still running
			stdout, _, _ := sshClient.Execute("pgrep -x mantisops-agent")
			if strings.TrimSpace(stdout) == "" {
				// Agent process died — check logs for error
				var errMsg string
				if hasSudo {
					logOut, _, _ := sshClient.Execute(sudo("journalctl -u mantisops-agent --no-pager -n 5 2>/dev/null"))
					errMsg = strings.TrimSpace(logOut)
				} else {
					logOut, _, _ := sshClient.Execute("tail -5 ~/.config/mantisops/agent.log 2>/dev/null")
					errMsg = strings.TrimSpace(logOut)
				}
				if errMsg == "" {
					errMsg = "Agent 进程已退出，未获取到日志"
				}
				d.fail(managedID, "Agent 启动后异常退出: "+errMsg)
				return
			}
			// Check for gRPC errors in agent log (incompatible proto, auth failure, etc.)
			var logOut string
			if hasSudo {
				logOut, _, _ = sshClient.Execute(sudo("journalctl -u mantisops-agent --no-pager -n 10 2>/dev/null"))
			} else {
				logOut, _, _ = sshClient.Execute("tail -10 ~/.config/mantisops/agent.log 2>/dev/null")
			}
			if strings.Contains(logOut, "unknown service") || strings.Contains(logOut, "Unimplemented") {
				d.fail(managedID, "Agent 版本不兼容（proto 服务名不匹配），请更新 build 目录中的 Agent 二进制后重试")
				// Stop the broken agent
				if hasSudo {
					sshClient.Execute(sudo("systemctl stop mantisops-agent 2>/dev/null"))
				} else {
					sshClient.Execute("pkill -x mantisops-agent 2>/dev/null")
				}
				return
			}
			if strings.Contains(logOut, "authentication") || strings.Contains(logOut, "PermissionDenied") {
				d.fail(managedID, "Agent 认证失败，请检查 PSK Token 配置")
				return
			}
		}
	}
}

func (d *Deployer) fail(managedID int, errMsg string) {
	d.managedStore.UpdateState(managedID, store.InstallStateFailed, errMsg)
	d.broadcast(managedID, store.InstallStateFailed, errMsg)
	log.Printf("[deployer] failed managed=%d: %s", managedID, errMsg)
}

func (d *Deployer) broadcast(managedID int, state, message string) {
	d.hub.BroadcastAdmin(map[string]interface{}{
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
