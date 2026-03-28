package collector

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	openapiv1 "github.com/alibabacloud-go/darabonba-openapi/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v4/client"
	cms "github.com/alibabacloud-go/cms-20190101/v2/client"
	"github.com/alibabacloud-go/tea/tea"

	"mantisops/server/internal/config"
	"mantisops/server/internal/model"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
	pb "mantisops/server/proto/gen"
)

// ECSTarget 表示一个需要监控的 ECS 实例
type ECSTarget struct {
	InstanceID string
	HostID     string
	RegionID   string
}

// RDSTarget 表示一个需要监控的 RDS 实例
type RDSTarget struct {
	InstanceID string
	HostID     string
}

// AliyunCollector 通过阿里云 API 采集 ECS 监控数据
type AliyunCollector struct {
	cfg         config.AliyunConfig
	cloudStore  *store.CloudStore      // 从数据库读取实例列表
	credStore   *store.CredentialStore // 解密 AK
	ecsClients  map[string]*ecs.Client
	cmsClients  map[string]*cms.Client
	vm          *store.VictoriaStore
	serverStore *store.ServerStore
	hub         *ws.Hub
	stopCh      chan struct{}
	// 缓存实例信息：instanceID → memoryTotal (bytes)
	memoryCache map[string]int64
	// 缓存最新指标快照，页面加载时直接返回，无需等 WebSocket
	metricsMu     sync.RWMutex
	metricsCache  map[string]*pb.MetricsPayload
}

func NewAliyunCollector(cfg config.AliyunConfig, vm *store.VictoriaStore, ss *store.ServerStore, hub *ws.Hub, cloudStore *store.CloudStore, credStore *store.CredentialStore) (*AliyunCollector, error) {
	if cfg.Interval <= 0 {
		cfg.Interval = 60
	}

	// 凭证：环境变量 > 配置文件 > 数据库云账号凭据
	akID := os.Getenv("ALIYUN_ACCESS_KEY_ID")
	if akID == "" {
		akID = cfg.AccessKeyID
	}
	akSecret := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")
	if akSecret == "" {
		akSecret = cfg.AccessKeySecret
	}
	// Fallback: load from first cloud account credential in DB
	if (akID == "" || akSecret == "") && cloudStore != nil && credStore != nil {
		accounts, err := cloudStore.ListAccounts()
		if err == nil {
			for _, acc := range accounts {
				cred, err := credStore.Get(acc.CredentialID)
				if err == nil && cred.Data != nil {
					if v := cred.Data["access_key_id"]; v != "" {
						akID = v
					}
					if v := cred.Data["access_key_secret"]; v != "" {
						akSecret = v
					}
					if akID != "" && akSecret != "" {
						log.Printf("[aliyun] loaded AK from cloud account %q (id=%d)", acc.Name, acc.ID)
						break
					}
				}
			}
		}
	}
	if akID == "" || akSecret == "" {
		return nil, fmt.Errorf("aliyun access key not configured")
	}

	ac := &AliyunCollector{
		cfg:          cfg,
		cloudStore:   cloudStore,
		credStore:    credStore,
		ecsClients:   make(map[string]*ecs.Client),
		cmsClients:   make(map[string]*cms.Client),
		vm:           vm,
		serverStore:  ss,
		hub:          hub,
		stopCh:       make(chan struct{}),
		memoryCache:  make(map[string]int64),
		metricsCache: make(map[string]*pb.MetricsPayload),
	}

	// 按 region 创建客户端（ECS 需要 region，RDS 统一用默认 region）
	regions := make(map[string]bool)
	for _, inst := range cfg.Instances {
		if inst.InstanceID == "" {
			log.Printf("[aliyun] warning: instance_id is empty for host_id=%s, skipping", inst.HostID)
			continue
		}
		regions[inst.RegionID] = true
	}
	// RDS 使用第一个 region 的 CMS 客户端（CloudMonitor 不区分 region）
	if len(cfg.RDS) > 0 && len(regions) == 0 {
		regions["cn-hangzhou"] = true
	}
	// Also load regions from DB instances
	if cloudStore != nil {
		ecsInsts, rdsInsts, err := cloudStore.LoadMonitoredInstances()
		if err == nil {
			for _, e := range ecsInsts {
				if e.RegionID != "" {
					regions[e.RegionID] = true
				}
			}
			if len(rdsInsts) > 0 && len(regions) == 0 {
				regions["cn-hangzhou"] = true
			}
		}
	}
	// Ensure at least one region for DB-only setups
	if len(regions) == 0 {
		regions["cn-hangzhou"] = true
	}

	for region := range regions {
		openapiCfg := &openapi.Config{
			AccessKeyId:     tea.String(akID),
			AccessKeySecret: tea.String(akSecret),
			RegionId:        tea.String(region),
		}

		ecsEndpoint := fmt.Sprintf("ecs.%s.aliyuncs.com", region)
		openapiCfg.Endpoint = tea.String(ecsEndpoint)
		ecsClient, err := ecs.NewClient(openapiCfg)
		if err != nil {
			return nil, fmt.Errorf("create ecs client for %s: %w", region, err)
		}
		ac.ecsClients[region] = ecsClient

		cmsCfg := &openapiv1.Config{
			AccessKeyId:     tea.String(akID),
			AccessKeySecret: tea.String(akSecret),
			RegionId:        tea.String(region),
			Endpoint:        tea.String(fmt.Sprintf("metrics.%s.aliyuncs.com", region)),
		}
		cmsClient, err := cms.NewClient(cmsCfg)
		if err != nil {
			return nil, fmt.Errorf("create cms client for %s: %w", region, err)
		}
		ac.cmsClients[region] = cmsClient
	}

	// 启动验证：注册实例信息
	if err := ac.registerInstances(); err != nil {
		return nil, fmt.Errorf("register instances: %w", err)
	}

	return ac, nil
}

func (ac *AliyunCollector) Start() {
	go ac.loop()
}

func (ac *AliyunCollector) Stop() {
	close(ac.stopCh)
}

// GetCachedMetrics 返回最新的指标快照，供 API 直接返回给前端
func (ac *AliyunCollector) GetCachedMetrics() map[string]*pb.MetricsPayload {
	ac.metricsMu.RLock()
	defer ac.metricsMu.RUnlock()
	cp := make(map[string]*pb.MetricsPayload, len(ac.metricsCache))
	for k, v := range ac.metricsCache {
		cp[k] = v
	}
	return cp
}

// loadInstances 从数据库优先加载实例列表，失败则回退到配置文件
func (ac *AliyunCollector) loadInstances() ([]ECSTarget, []RDSTarget) {
	// 优先从数据库加载
	if ac.cloudStore != nil {
		ecsInsts, rdsInsts, err := ac.cloudStore.LoadMonitoredInstances()
		if err == nil && (len(ecsInsts) > 0 || len(rdsInsts) > 0) {
			var ecsList []ECSTarget
			var rdsList []RDSTarget
			for _, e := range ecsInsts {
				ecsList = append(ecsList, ECSTarget{
					InstanceID: e.InstanceID,
					HostID:     e.HostID,
					RegionID:   e.RegionID,
				})
			}
			for _, r := range rdsInsts {
				rdsList = append(rdsList, RDSTarget{
					InstanceID: r.InstanceID,
					HostID:     r.HostID,
				})
			}
			log.Printf("[aliyun] loaded from DB: %d ECS + %d RDS instances", len(ecsList), len(rdsList))
			return ecsList, rdsList
		}
	}

	// 回退到配置文件
	var ecsList []ECSTarget
	for _, inst := range ac.cfg.Instances {
		if inst.InstanceID != "" {
			ecsList = append(ecsList, ECSTarget{
				InstanceID: inst.InstanceID,
				HostID:     inst.HostID,
				RegionID:   inst.RegionID,
			})
		}
	}
	var rdsList []RDSTarget
	for _, rds := range ac.cfg.RDS {
		rdsList = append(rdsList, RDSTarget{
			InstanceID: rds.InstanceID,
			HostID:     rds.HostID,
		})
	}
	if len(ecsList) > 0 || len(rdsList) > 0 {
		log.Printf("[aliyun] loaded from config: %d ECS + %d RDS instances", len(ecsList), len(rdsList))
	}
	return ecsList, rdsList
}

// MigrateFromConfig 将配置文件中的实例信息一次性导入数据库
// MigrateFromConfig imports existing config into DB. Returns the new account ID (0 if skipped).
func (ac *AliyunCollector) MigrateFromConfig() int {
	if ac.cloudStore == nil || ac.credStore == nil {
		return 0
	}
	// 如果数据库已有云账号数据，跳过迁移
	accounts, _ := ac.cloudStore.ListAccounts()
	if len(accounts) > 0 {
		return 0
	}
	// 配置文件中没有数据则无需迁移
	if !ac.cfg.Enabled || (len(ac.cfg.Instances) == 0 && len(ac.cfg.RDS) == 0) {
		return 0
	}

	log.Println("[aliyun] migrating config to database...")

	// 获取凭证（环境变量优先）
	akID := ac.cfg.AccessKeyID
	akSecret := ac.cfg.AccessKeySecret
	if envAK := os.Getenv("ALIYUN_ACCESS_KEY_ID"); envAK != "" {
		akID = envAK
	}
	if envSK := os.Getenv("ALIYUN_ACCESS_KEY_SECRET"); envSK != "" {
		akSecret = envSK
	}

	credID, err := ac.credStore.Create("从配置文件导入", "aliyun_ak", map[string]string{
		"access_key_id":     akID,
		"access_key_secret": akSecret,
	})
	if err != nil {
		log.Printf("[aliyun] migration: create credential failed: %v", err)
		return 0
	}

	accountID, err := ac.cloudStore.CreateAccount("从配置文件导入", "aliyun", credID, nil, false)
	if err != nil {
		log.Printf("[aliyun] migration: create account failed: %v", err)
		return 0
	}

	// 导入 ECS 实例
	for _, inst := range ac.cfg.Instances {
		if inst.InstanceID == "" {
			continue
		}
		ci := &store.CloudInstance{
			InstanceType: "ecs",
			InstanceID:   inst.InstanceID,
			HostID:       inst.HostID,
			RegionID:     inst.RegionID,
			Monitored:    true,
		}
		ac.cloudStore.UpsertInstance(accountID, ci)
	}
	// 确认 ECS 实例（设置 monitored=1 并注册到 servers 表）
	instances, _ := ac.cloudStore.ListInstances(accountID)
	var ids []int
	for _, inst := range instances {
		ids = append(ids, inst.ID)
	}
	if len(ids) > 0 {
		ac.cloudStore.ConfirmInstances(ids)
	}

	// 导入 RDS 实例
	for _, rdsInst := range ac.cfg.RDS {
		ci := &store.CloudInstance{
			InstanceType: "rds",
			InstanceID:   rdsInst.InstanceID,
			HostID:       rdsInst.HostID,
			Monitored:    true,
		}
		ac.cloudStore.UpsertInstance(accountID, ci)
	}
	// RDS 实例不需要注册到 servers 表，直接更新 monitored 状态
	instances, _ = ac.cloudStore.ListInstances(accountID)
	for _, inst := range instances {
		if inst.InstanceType == "rds" {
			ac.cloudStore.UpdateInstanceMonitored(inst.ID, true)
		}
	}

	ac.cloudStore.UpdateAccountSyncState(accountID, store.SyncStateSynced, "")

	log.Printf("[aliyun] migrated: %d ECS + %d RDS instances from config, triggering sync for metadata...", len(ac.cfg.Instances), len(ac.cfg.RDS))
	return accountID
}

func (ac *AliyunCollector) loop() {
	// 首次立即采集
	ac.collectAll()

	ticker := time.NewTicker(time.Duration(ac.cfg.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ac.collectAll()
		case <-ac.stopCh:
			return
		}
	}
}

// registerInstances 调用 ECS API 获取实例信息并注册到 ServerStore
func (ac *AliyunCollector) registerInstances() error {
	// Collect all ECS targets: config file + database
	type target struct {
		InstanceID string
		HostID     string
		RegionID   string
	}
	var targets []target

	for _, inst := range ac.cfg.Instances {
		if inst.InstanceID != "" {
			targets = append(targets, target{inst.InstanceID, inst.HostID, inst.RegionID})
		}
	}
	if ac.cloudStore != nil {
		ecsInsts, _, err := ac.cloudStore.LoadMonitoredInstances()
		if err == nil {
			for _, e := range ecsInsts {
				targets = append(targets, target{e.InstanceID, e.HostID, e.RegionID})
			}
		}
	}

	// Deduplicate by instanceID
	seen := make(map[string]bool)
	for _, t := range targets {
		if seen[t.InstanceID] {
			continue
		}
		seen[t.InstanceID] = true

		if err := ac.registerOneInstance(t.InstanceID, t.HostID, t.RegionID); err != nil {
			log.Printf("[aliyun] register %s failed: %v", t.InstanceID, err)
			continue
		}
	}
	return nil
}

// registerOneInstance registers a single ECS instance by calling DescribeInstances API.
func (ac *AliyunCollector) registerOneInstance(instanceID, hostID, regionID string) error {
	ecsClient, ok := ac.ecsClients[regionID]
	if !ok {
		return fmt.Errorf("no ecs client for region %s", regionID)
	}

	resp, err := ecsClient.DescribeInstances(&ecs.DescribeInstancesRequest{
		RegionId:    tea.String(regionID),
		InstanceIds: tea.String(fmt.Sprintf(`["%s"]`, instanceID)),
	})
	if err != nil {
		return fmt.Errorf("describe instance %s: %w", instanceID, err)
	}

	instances := resp.Body.Instances.Instance
	if len(instances) == 0 {
		return fmt.Errorf("instance %s not found in region %s", instanceID, regionID)
	}

	ecsInst := instances[0]

	// 收集 IP 地址
	var ips []string
	if ecsInst.PublicIpAddress != nil && ecsInst.PublicIpAddress.IpAddress != nil {
		for _, ip := range ecsInst.PublicIpAddress.IpAddress {
			ips = append(ips, tea.StringValue(ip))
		}
	}
	if ecsInst.EipAddress != nil && ecsInst.EipAddress.IpAddress != nil {
		ips = append(ips, tea.StringValue(ecsInst.EipAddress.IpAddress))
	}
	if ecsInst.VpcAttributes != nil && ecsInst.VpcAttributes.PrivateIpAddress != nil {
		for _, ip := range ecsInst.VpcAttributes.PrivateIpAddress.IpAddress {
			ips = append(ips, tea.StringValue(ip))
		}
	}

	// 内存：ECS 返回 MB，转 bytes
	memoryMB := int64(tea.Int32Value(ecsInst.Memory))
	memoryBytes := memoryMB * 1024 * 1024
	ac.memoryCache[instanceID] = memoryBytes

	// 解析启动时间
	var bootTime int64
	if ecsInst.StartTime != nil {
		if t, err := time.Parse("2006-01-02T15:04Z", tea.StringValue(ecsInst.StartTime)); err == nil {
			bootTime = t.Unix()
		}
	}

	hostname := tea.StringValue(ecsInst.InstanceName)
	if hostname == "" {
		hostname = tea.StringValue(ecsInst.HostName)
	}

	srv := &model.Server{
		HostID:       hostID,
		Hostname:     hostname,
		IPAddresses:  store.IPListToJSON(ips),
		OS:           tea.StringValue(ecsInst.OSName),
		Kernel:       "",
		Arch:         "x86_64",
		AgentVersion: "cloud-api/1.0",
		CPUCores:     int(tea.Int32Value(ecsInst.Cpu)),
		CPUModel:     tea.StringValue(ecsInst.InstanceType),
		MemoryTotal:  memoryBytes,
		BootTime:     bootTime,
	}

	if err := ac.serverStore.Upsert(srv); err != nil {
		return fmt.Errorf("upsert server %s: %w", hostID, err)
	}

	log.Printf("[aliyun] registered: %s (%s) cpu=%d mem=%dMB",
		hostname, instanceID, srv.CPUCores, memoryMB)
	return nil
}

// collectAll 采集所有实例的监控指标
func (ac *AliyunCollector) collectAll() {
	ecsTargets, rdsTargets := ac.loadInstances()

	// 按 region 分组实例
	regionInstances := make(map[string][]config.AliyunInstance)
	for _, t := range ecsTargets {
		regionInstances[t.RegionID] = append(regionInstances[t.RegionID], config.AliyunInstance{
			RegionID:   t.RegionID,
			InstanceID: t.InstanceID,
			HostID:     t.HostID,
		})
	}

	for region, instances := range regionInstances {
		cmsClient, ok := ac.cmsClients[region]
		if !ok {
			continue
		}
		ac.collectRegion(cmsClient, instances)
	}

	// RDS 采集：用 loadInstances 的结果刷新 ac.cfg.RDS
	if len(rdsTargets) > 0 {
		ac.cfg.RDS = nil
		for _, t := range rdsTargets {
			ac.cfg.RDS = append(ac.cfg.RDS, config.AliyunRDS{
				InstanceID: t.InstanceID,
				HostID:     t.HostID,
			})
		}
		// 使用任意一个 region 的 CMS 客户端
		for _, cmsClient := range ac.cmsClients {
			ac.collectRDS(cmsClient)
			break
		}
	}
}

// 需要采集的指标
var aliyunMetrics = []struct {
	MetricName string
	Namespace  string
}{
	{"CPUUtilization", "acs_ecs_dashboard"},
	{"load_1m", "acs_ecs_dashboard"},
	{"load_5m", "acs_ecs_dashboard"},
	{"memory_usedutilization", "acs_ecs_dashboard"},
	{"diskusage_utilization", "acs_ecs_dashboard"},
	{"InternetInRate", "acs_ecs_dashboard"},
	{"InternetOutRate", "acs_ecs_dashboard"},
	{"IntranetInRate", "acs_ecs_dashboard"},
	{"IntranetOutRate", "acs_ecs_dashboard"},
	{"DiskReadBPS", "acs_ecs_dashboard"},
	{"DiskWriteBPS", "acs_ecs_dashboard"},
}

type metricDatapoint struct {
	InstanceID string  `json:"instanceId"`
	Average    float64 `json:"Average"`
	Maximum    float64 `json:"Maximum"`
	Timestamp  int64   `json:"timestamp"`
}

func (ac *AliyunCollector) collectRegion(cmsClient *cms.Client, instances []config.AliyunInstance) {
	// 构建 instanceID → config 映射
	instMap := make(map[string]config.AliyunInstance)
	for _, inst := range instances {
		instMap[inst.InstanceID] = inst
	}

	// 收集所有指标数据: hostID → metricName → value
	metricsData := make(map[string]map[string]float64)
	for _, inst := range instances {
		metricsData[inst.HostID] = make(map[string]float64)
	}

	// 按指标名聚合查询
	for _, metric := range aliyunMetrics {
		datapoints := ac.queryMetric(cmsClient, metric.MetricName, metric.Namespace, instances)
		for _, dp := range datapoints {
			inst, ok := instMap[dp.InstanceID]
			if !ok {
				continue
			}
			metricsData[inst.HostID][metric.MetricName] = dp.Average
		}
	}

	// 写入 VictoriaMetrics + 广播 WebSocket
	for _, inst := range instances {
		data := metricsData[inst.HostID]
		if len(data) == 0 {
			continue
		}

		host := inst.HostID
		if srv, err := ac.serverStore.GetByHostID(inst.HostID); err == nil {
			host = srv.Hostname
		}
		hostID := inst.HostID

		var lines []string

		// CPU
		if cpuPct, ok := data["CPUUtilization"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_cpu_usage_percent{host_id="%s",host="%s"} %f`, hostID, host, cpuPct),
			)
		}

		// Load
		if v, ok := data["load_1m"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_cpu_load1{host_id="%s",host="%s"} %f`, hostID, host, v),
			)
		}
		if v, ok := data["load_5m"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_cpu_load5{host_id="%s",host="%s"} %f`, hostID, host, v),
			)
		}

		// Memory
		memoryTotal := ac.memoryCache[inst.InstanceID]
		if memPct, ok := data["memory_usedutilization"]; ok {
			memUsed := int64(memPct * float64(memoryTotal) / 100)
			lines = append(lines,
				fmt.Sprintf(`mantisops_memory_usage_percent{host_id="%s",host="%s"} %f`, hostID, host, memPct),
				fmt.Sprintf(`mantisops_memory_used_bytes{host_id="%s",host="%s"} %d`, hostID, host, memUsed),
				fmt.Sprintf(`mantisops_memory_total_bytes{host_id="%s",host="%s"} %d`, hostID, host, memoryTotal),
			)
		}

		// Disk
		if diskPct, ok := data["diskusage_utilization"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_disk_usage_percent{host_id="%s",host="%s",mount="/"} %f`, hostID, host, diskPct),
			)
		}

		// Network: bits/s → bytes/s
		// 公网
		if rxBits, ok := data["InternetInRate"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_network_rx_bytes_per_sec{host_id="%s",host="%s",iface="internet"} %f`, hostID, host, rxBits/8),
			)
		}
		if txBits, ok := data["InternetOutRate"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_network_tx_bytes_per_sec{host_id="%s",host="%s",iface="internet"} %f`, hostID, host, txBits/8),
			)
		}
		// 内网
		if rxBits, ok := data["IntranetInRate"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_network_rx_bytes_per_sec{host_id="%s",host="%s",iface="intranet"} %f`, hostID, host, rxBits/8),
			)
		}
		if txBits, ok := data["IntranetOutRate"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_network_tx_bytes_per_sec{host_id="%s",host="%s",iface="intranet"} %f`, hostID, host, txBits/8),
			)
		}

		// Disk IO
		if readBPS, ok := data["DiskReadBPS"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_disk_read_bytes_per_sec{host_id="%s",host="%s",mount="/"} %f`, hostID, host, readBPS),
			)
		}
		if writeBPS, ok := data["DiskWriteBPS"]; ok {
			lines = append(lines,
				fmt.Sprintf(`mantisops_disk_write_bytes_per_sec{host_id="%s",host="%s",mount="/"} %f`, hostID, host, writeBPS),
			)
		}

		// 写入 VictoriaMetrics
		if len(lines) > 0 {
			if err := ac.vm.WriteMetrics(lines); err != nil {
				log.Printf("[aliyun] vm write error for %s: %v", hostID, err)
			} else {
				log.Printf("[aliyun] collected %s: %d metrics", hostID, len(lines))
			}
		} else {
			log.Printf("[aliyun] no metrics for %s", hostID)
		}

		// 更新心跳
		ac.serverStore.UpdateLastSeen(hostID)

		// WebSocket 广播
		payload := &pb.MetricsPayload{
			HostId:    hostID,
			Timestamp: time.Now().Unix(),
		}
		if cpuPct, ok := data["CPUUtilization"]; ok {
			payload.Cpu = &pb.CpuMetrics{UsagePercent: cpuPct}
			if load1, ok2 := data["load_1m"]; ok2 {
				payload.Cpu.Load1 = load1
			}
			if load5, ok2 := data["load_5m"]; ok2 {
				payload.Cpu.Load5 = load5
			}
		}
		if memPct, ok := data["memory_usedutilization"]; ok {
			memUsed := uint64(memPct * float64(memoryTotal) / 100)
			payload.Memory = &pb.MemoryMetrics{
				Total:        uint64(memoryTotal),
				Used:         memUsed,
				UsagePercent: memPct,
			}
		}
		if diskPct, ok := data["diskusage_utilization"]; ok {
			payload.Disks = []*pb.DiskMetrics{
				{MountPoint: "/", UsagePercent: diskPct},
			}
		}
		// 网络：优先内网，补充公网
		var nets []*pb.NetworkMetrics
		if rxBits, ok := data["IntranetInRate"]; ok {
			net := &pb.NetworkMetrics{
				Interface:     "intranet",
				RxBytesPerSec: rxBits / 8,
			}
			if txBits, ok2 := data["IntranetOutRate"]; ok2 {
				net.TxBytesPerSec = txBits / 8
			}
			nets = append(nets, net)
		}
		if rxBits, ok := data["InternetInRate"]; ok {
			net := &pb.NetworkMetrics{
				Interface:     "internet",
				RxBytesPerSec: rxBits / 8,
			}
			if txBits, ok2 := data["InternetOutRate"]; ok2 {
				net.TxBytesPerSec = txBits / 8
			}
			nets = append(nets, net)
		}
		if len(nets) > 0 {
			payload.Networks = nets
		}

		// 缓存最新指标快照
		ac.metricsMu.Lock()
		ac.metricsCache[hostID] = payload
		ac.metricsMu.Unlock()

		ac.hub.BroadcastMetrics(hostID, map[string]interface{}{
			"type":    "metrics",
			"host_id": hostID,
			"data":    payload,
		})
	}
}

// queryMetric 按指标名批量查询所有实例的最新数据
func (ac *AliyunCollector) queryMetric(cmsClient *cms.Client, metricName, namespace string, instances []config.AliyunInstance) []metricDatapoint {
	// 构建 Dimensions: [{"instanceId":"i-xxx"},{"instanceId":"i-yyy"}]
	var dims []map[string]string
	for _, inst := range instances {
		dims = append(dims, map[string]string{"instanceId": inst.InstanceID})
	}
	dimsJSON, _ := json.Marshal(dims)

	req := &cms.DescribeMetricLastRequest{
		Namespace:  tea.String(namespace),
		MetricName: tea.String(metricName),
		Dimensions: tea.String(string(dimsJSON)),
		Period:     tea.String("60"),
	}

	var resp *cms.DescribeMetricLastResponse
	var err error

	// 重试机制（限流退避）
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = cmsClient.DescribeMetricLast(req)
		if err == nil {
			break
		}
		errStr := err.Error()
		if strings.Contains(errStr, "Throttling") {
			wait := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("[aliyun] throttled on %s, retry in %v", metricName, wait)
			time.Sleep(wait)
			continue
		}
		log.Printf("[aliyun] query %s error: %v", metricName, err)
		return nil
	}

	if err != nil {
		log.Printf("[aliyun] query %s failed after retries: %v", metricName, err)
		return nil
	}

	if resp.Body == nil || resp.Body.Datapoints == nil || tea.StringValue(resp.Body.Datapoints) == "" {
		return nil
	}

	var datapoints []metricDatapoint
	if err := json.Unmarshal([]byte(tea.StringValue(resp.Body.Datapoints)), &datapoints); err != nil {
		log.Printf("[aliyun] parse %s datapoints error: %v", metricName, err)
		return nil
	}

	return datapoints
}

// ===== RDS 采集 =====

var rdsMetrics = []string{
	"CpuUsage",
	"MemoryUsage",
	"DiskUsage",
	"IOPSUsage",
	"ConnectionUsage",
	"MySQL_QPS",
	"MySQL_TPS",
	"MySQL_NetworkInNew",
	"MySQL_NetworkOutNew",
	"MySQL_ActiveSessions",
}

func (ac *AliyunCollector) collectRDS(cmsClient *cms.Client) {
	// 构建 instanceID → hostID 映射
	instMap := make(map[string]config.AliyunRDS)
	for _, rds := range ac.cfg.RDS {
		instMap[rds.InstanceID] = rds
	}

	// 收集指标: hostID → metricName → value
	metricsData := make(map[string]map[string]float64)
	for _, rds := range ac.cfg.RDS {
		metricsData[rds.HostID] = make(map[string]float64)
	}

	// 按指标名聚合查询
	for _, metricName := range rdsMetrics {
		datapoints := ac.queryRDSMetric(cmsClient, metricName)
		for _, dp := range datapoints {
			rds, ok := instMap[dp.InstanceID]
			if !ok {
				continue
			}
			metricsData[rds.HostID][metricName] = dp.Average
		}
	}

	// 写入 VictoriaMetrics
	for _, rds := range ac.cfg.RDS {
		data := metricsData[rds.HostID]
		if len(data) == 0 {
			continue
		}

		hostID := rds.HostID
		host := rds.HostID

		var lines []string

		if v, ok := data["CpuUsage"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_cpu_usage{host_id="%s",host="%s"} %f`, hostID, host, v))
		}
		if v, ok := data["MemoryUsage"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_memory_usage{host_id="%s",host="%s"} %f`, hostID, host, v))
		}
		if v, ok := data["DiskUsage"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_disk_usage{host_id="%s",host="%s"} %f`, hostID, host, v))
		}
		if v, ok := data["IOPSUsage"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_iops_usage{host_id="%s",host="%s"} %f`, hostID, host, v))
		}
		if v, ok := data["ConnectionUsage"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_connection_usage{host_id="%s",host="%s"} %f`, hostID, host, v))
		}
		if v, ok := data["MySQL_QPS"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_qps{host_id="%s",host="%s"} %f`, hostID, host, v))
		}
		if v, ok := data["MySQL_TPS"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_tps{host_id="%s",host="%s"} %f`, hostID, host, v))
		}
		if v, ok := data["MySQL_NetworkInNew"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_network_in_bytes{host_id="%s",host="%s"} %f`, hostID, host, v))
		}
		if v, ok := data["MySQL_NetworkOutNew"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_network_out_bytes{host_id="%s",host="%s"} %f`, hostID, host, v))
		}
		if v, ok := data["MySQL_ActiveSessions"]; ok {
			lines = append(lines, fmt.Sprintf(`mantisops_rds_active_sessions{host_id="%s",host="%s"} %f`, hostID, host, v))
		}

		if len(lines) > 0 {
			if err := ac.vm.WriteMetrics(lines); err != nil {
				log.Printf("[aliyun-rds] vm write error for %s: %v", hostID, err)
			} else {
				log.Printf("[aliyun-rds] collected %s: %d metrics", hostID, len(lines))
			}
		}

		// WebSocket 广播 RDS 数据
		ac.hub.BroadcastMetrics(hostID, map[string]interface{}{
			"type":    "rds_metrics",
			"host_id": hostID,
			"data":    data,
		})
	}
}

func (ac *AliyunCollector) queryRDSMetric(cmsClient *cms.Client, metricName string) []metricDatapoint {
	var dims []map[string]string
	for _, rds := range ac.cfg.RDS {
		dims = append(dims, map[string]string{"instanceId": rds.InstanceID})
	}
	dimsJSON, _ := json.Marshal(dims)

	req := &cms.DescribeMetricLastRequest{
		Namespace:  tea.String("acs_rds_dashboard"),
		MetricName: tea.String(metricName),
		Dimensions: tea.String(string(dimsJSON)),
		Period:     tea.String("60"),
	}

	var resp *cms.DescribeMetricLastResponse
	var err error

	for attempt := 0; attempt < 3; attempt++ {
		resp, err = cmsClient.DescribeMetricLast(req)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "Throttling") {
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
			continue
		}
		log.Printf("[aliyun-rds] query %s error: %v", metricName, err)
		return nil
	}
	if err != nil {
		return nil
	}
	if resp.Body == nil || resp.Body.Datapoints == nil || tea.StringValue(resp.Body.Datapoints) == "" {
		return nil
	}

	var datapoints []metricDatapoint
	if err := json.Unmarshal([]byte(tea.StringValue(resp.Body.Datapoints)), &datapoints); err != nil {
		log.Printf("[aliyun-rds] parse %s error: %v", metricName, err)
		return nil
	}
	return datapoints
}
