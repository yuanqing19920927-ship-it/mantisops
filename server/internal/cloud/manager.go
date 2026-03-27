package cloud

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v4/client"
	rds "github.com/alibabacloud-go/rds-20140815/v6/client"
	sts "github.com/alibabacloud-go/sts-20150401/v2/client"
	"github.com/alibabacloud-go/tea/tea"

	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

type Manager struct {
	db        *sql.DB
	cloud     *store.CloudStore
	credStore *store.CredentialStore
	hub       *ws.Hub
}

type VerifyResult struct {
	Valid       bool              `json:"valid"`
	AccountUID  string            `json:"account_uid"`
	AccountName string            `json:"account_name"`
	Permissions []PermissionCheck `json:"permissions"`
}

type PermissionCheck struct {
	Action  string `json:"action"`
	Allowed bool   `json:"allowed"`
}

func NewManager(db *sql.DB, cloud *store.CloudStore, cred *store.CredentialStore, hub *ws.Hub) *Manager {
	return &Manager{db: db, cloud: cloud, credStore: cred, hub: hub}
}

// Verify validates an AK/SK pair via STS GetCallerIdentity and tests ECS/RDS permissions.
func (m *Manager) Verify(ak, sk string) (*VerifyResult, error) {
	result := &VerifyResult{Permissions: []PermissionCheck{}}

	// 1. STS GetCallerIdentity
	stsCfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String("sts.aliyuncs.com"),
	}
	stsClient, err := sts.NewClient(stsCfg)
	if err != nil {
		return nil, fmt.Errorf("create STS client: %w", err)
	}
	identity, err := stsClient.GetCallerIdentity()
	if err != nil {
		result.Valid = false
		return result, nil
	}
	result.Valid = true
	result.AccountUID = tea.StringValue(identity.Body.AccountId)
	result.AccountName = tea.StringValue(identity.Body.Arn)

	// 2. Test ECS permission
	ecsPerm := PermissionCheck{Action: "ECS:DescribeInstances"}
	ecsCfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String("ecs.cn-hangzhou.aliyuncs.com"),
	}
	ecsClient, _ := ecs.NewClient(ecsCfg)
	if ecsClient != nil {
		_, err := ecsClient.DescribeInstances(&ecs.DescribeInstancesRequest{
			RegionId: tea.String("cn-hangzhou"),
			PageSize: tea.Int32(1),
		})
		ecsPerm.Allowed = err == nil
	}
	result.Permissions = append(result.Permissions, ecsPerm)

	// 3. Test RDS permission
	rdsPerm := PermissionCheck{Action: "RDS:DescribeDBInstances"}
	rdsCfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String("rds.aliyuncs.com"),
	}
	rdsClient, _ := rds.NewClient(rdsCfg)
	if rdsClient != nil {
		_, err := rdsClient.DescribeDBInstances(&rds.DescribeDBInstancesRequest{
			RegionId: tea.String("cn-hangzhou"),
			PageSize: tea.Int32(1),
		})
		rdsPerm.Allowed = err == nil
	}
	result.Permissions = append(result.Permissions, rdsPerm)

	return result, nil
}

// Sync triggers an async discovery of ECS and RDS instances for a cloud account.
// Progress is broadcast via WebSocket.
func (m *Manager) Sync(accountID int) error {
	account, err := m.cloud.GetAccount(accountID)
	if err != nil {
		return err
	}

	m.cloud.UpdateAccountSyncState(accountID, store.SyncStateSyncing, "")
	m.broadcast("cloud_sync_progress", accountID, store.SyncStateSyncing, "开始同步...")

	go func() {
		syncErrors := map[string]string{}

		cred, err := m.credStore.Get(account.CredentialID)
		if err != nil {
			m.cloud.UpdateAccountSyncState(accountID, store.SyncStateFailed, err.Error())
			m.broadcast("cloud_sync_progress", accountID, store.SyncStateFailed, "凭据读取失败")
			return
		}
		ak := cred.Data["access_key_id"]
		sk := cred.Data["access_key_secret"]

		m.broadcast("cloud_sync_progress", accountID, store.SyncStateSyncing, "正在发现 ECS 实例...")
		ecsInstances, err := m.DiscoverECS(ak, sk, account.RegionIDs)
		if err != nil {
			syncErrors["ecs"] = err.Error()
			log.Printf("[cloud] ECS discovery failed for account %d: %v", accountID, err)
		} else {
			syncErrors["ecs"] = "ok"
			for _, inst := range ecsInstances {
				inst.CloudAccountID = accountID
				m.cloud.UpsertInstance(accountID, &inst)
			}
			m.broadcast("cloud_sync_progress", accountID, store.SyncStateSyncing,
				fmt.Sprintf("发现 %d 台 ECS 实例", len(ecsInstances)))
		}

		m.broadcast("cloud_sync_progress", accountID, store.SyncStateSyncing, "正在发现 RDS 实例...")
		rdsInstances, err := m.DiscoverRDS(ak, sk, account.RegionIDs)
		if err != nil {
			syncErrors["rds"] = err.Error()
			log.Printf("[cloud] RDS discovery failed for account %d: %v", accountID, err)
		} else {
			syncErrors["rds"] = "ok"
			for _, inst := range rdsInstances {
				inst.CloudAccountID = accountID
				m.cloud.UpsertInstance(accountID, &inst)
			}
			m.broadcast("cloud_sync_progress", accountID, store.SyncStateSyncing,
				fmt.Sprintf("发现 %d 个 RDS 实例", len(rdsInstances)))
		}

		errJSON, _ := json.Marshal(syncErrors)
		allOK := syncErrors["ecs"] == "ok" && syncErrors["rds"] == "ok"
		allFail := syncErrors["ecs"] != "ok" && syncErrors["rds"] != "ok"

		var state string
		switch {
		case allOK:
			state = store.SyncStateSynced
		case allFail:
			state = store.SyncStateFailed
		default:
			state = store.SyncStatePartial
		}

		m.cloud.UpdateAccountSyncState(accountID, state, string(errJSON))

		// Update servers table with cloud instance info for monitored ECS instances
		if err := m.cloud.SyncServersFromCloud(accountID); err != nil {
			log.Printf("[cloud] sync servers info for account %d: %v", accountID, err)
		}

		m.broadcast("cloud_sync_progress", accountID, state, "同步完成")
	}()

	return nil
}

func (m *Manager) broadcast(msgType string, accountID int, state, message string) {
	m.hub.BroadcastJSON(map[string]interface{}{
		"type":       msgType,
		"account_id": accountID,
		"state":      state,
		"message":    message,
		"timestamp":  time.Now().Unix(),
	})
}

// DiscoverECS discovers all ECS instances across the given regions.
func (m *Manager) DiscoverECS(ak, sk string, regionIDs []string) ([]store.CloudInstance, error) {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
	}

	// If no regions specified, use common China regions
	if len(regionIDs) == 0 {
		regionIDs = []string{"cn-hangzhou", "cn-shanghai", "cn-beijing", "cn-shenzhen", "cn-zhangjiakou", "cn-huhehaote", "cn-chengdu"}
	}

	var instances []store.CloudInstance
	for _, region := range regionIDs {
		cfg.Endpoint = tea.String(fmt.Sprintf("ecs.%s.aliyuncs.com", region))
		client, err := ecs.NewClient(cfg)
		if err != nil {
			continue
		}

		pageNum := int32(1)
		for {
			resp, err := client.DescribeInstances(&ecs.DescribeInstancesRequest{
				RegionId:   tea.String(region),
				PageSize:   tea.Int32(100),
				PageNumber: tea.Int32(pageNum),
			})
			if err != nil {
				log.Printf("[cloud] ECS list error region=%s: %v", region, err)
				break
			}
			if resp.Body == nil || resp.Body.Instances == nil {
				break
			}

			for _, inst := range resp.Body.Instances.Instance {
				instanceID := tea.StringValue(inst.InstanceId)
				name := tea.StringValue(inst.InstanceName)
				spec := tea.StringValue(inst.InstanceType)

				// Collect IPs
				var ips []string
				if inst.PublicIpAddress != nil {
					ips = append(ips, tea.StringSliceValue(inst.PublicIpAddress.IpAddress)...)
				}
				if inst.VpcAttributes != nil && inst.VpcAttributes.PrivateIpAddress != nil {
					ips = append(ips, tea.StringSliceValue(inst.VpcAttributes.PrivateIpAddress.IpAddress)...)
				}

				// Store extra system info for populating servers table
				extra := map[string]interface{}{
					"os_name": tea.StringValue(inst.OSName),
					"cpu":     tea.Int32Value(inst.Cpu),
					"memory":  tea.Int32Value(inst.Memory),
					"ips":     ips,
				}
				extraJSON, _ := json.Marshal(extra)

				instances = append(instances, store.CloudInstance{
					InstanceType: "ecs",
					InstanceID:   instanceID,
					HostID:       "ecs-" + instanceID,
					InstanceName: name,
					RegionID:     region,
					Spec:         spec,
					Extra:        string(extraJSON),
				})
			}

			total := tea.Int32Value(resp.Body.TotalCount)
			if pageNum*100 >= total {
				break
			}
			pageNum++
		}
	}
	return instances, nil
}

// DiscoverRDS discovers all RDS instances across the given regions.
func (m *Manager) DiscoverRDS(ak, sk string, regionIDs []string) ([]store.CloudInstance, error) {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(ak),
		AccessKeySecret: tea.String(sk),
		Endpoint:        tea.String("rds.aliyuncs.com"),
	}
	client, err := rds.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create RDS client: %w", err)
	}

	if len(regionIDs) == 0 {
		regionIDs = []string{"cn-hangzhou", "cn-shanghai", "cn-beijing", "cn-shenzhen"}
	}

	var instances []store.CloudInstance
	for _, region := range regionIDs {
		pageNum := int32(1)
		for {
			resp, err := client.DescribeDBInstances(&rds.DescribeDBInstancesRequest{
				RegionId:   tea.String(region),
				PageSize:   tea.Int32(100),
				PageNumber: tea.Int32(pageNum),
			})
			if err != nil {
				log.Printf("[cloud] RDS list error region=%s: %v", region, err)
				break
			}
			if resp.Body == nil || resp.Body.Items == nil {
				break
			}

			for _, inst := range resp.Body.Items.DBInstance {
				instanceID := tea.StringValue(inst.DBInstanceId)
				name := tea.StringValue(inst.DBInstanceDescription)
				engine := tea.StringValue(inst.Engine)
				engineVer := tea.StringValue(inst.EngineVersion)
				spec := tea.StringValue(inst.DBInstanceClass)
				endpoint := tea.StringValue(inst.ConnectionString)

				instances = append(instances, store.CloudInstance{
					InstanceType: "rds",
					InstanceID:   instanceID,
					HostID:       "rds-" + instanceID,
					InstanceName: name,
					RegionID:     region,
					Spec:         spec,
					Engine:       fmt.Sprintf("%s %s", engine, engineVer),
					Endpoint:     endpoint,
				})
			}

			total := tea.Int32Value(resp.Body.TotalRecordCount)
			if pageNum*100 >= total {
				break
			}
			pageNum++
		}
	}
	return instances, nil
}

// ConfirmInstances marks the given instances as monitored and registers ECS instances as servers.
func (m *Manager) ConfirmInstances(instanceIDs []int) error {
	return m.cloud.ConfirmInstances(instanceIDs)
}

// DeleteAccount performs a two-phase delete: if force is false, returns impact summary;
// if force is true, executes cascade delete in a single transaction.
func (m *Manager) DeleteAccount(accountID int, force bool) (*store.DeleteImpact, error) {
	// Get all ECS host_ids for this account
	instances, err := m.cloud.ListInstances(accountID)
	if err != nil {
		return nil, err
	}
	var hostIDs []string
	for _, inst := range instances {
		if inst.InstanceType == "ecs" && inst.Monitored {
			hostIDs = append(hostIDs, inst.HostID)
		}
	}

	if !force {
		// Return impact summary only
		impact, err := m.cloud.GetDeleteImpact(hostIDs)
		if err != nil {
			return nil, err
		}
		return impact, nil
	}

	// Force delete: single transaction
	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if len(hostIDs) > 0 {
		if err := m.cloud.CascadeDeleteServers(tx, hostIDs); err != nil {
			return nil, fmt.Errorf("cascade delete: %w", err)
		}
	}
	if err := m.cloud.DeleteAccountRow(tx, accountID); err != nil {
		return nil, fmt.Errorf("delete account: %w", err)
	}

	return nil, tx.Commit()
}

// DeleteInstance performs a two-phase delete for a single cloud instance.
func (m *Manager) DeleteInstance(instanceID int, force bool) (*store.DeleteImpact, error) {
	inst, err := m.cloud.GetInstance(instanceID)
	if err != nil {
		return nil, err
	}

	var hostIDs []string
	if inst.InstanceType == "ecs" && inst.Monitored {
		hostIDs = append(hostIDs, inst.HostID)
	}

	if !force {
		impact, err := m.cloud.GetDeleteImpact(hostIDs)
		if err != nil {
			return nil, err
		}
		return impact, nil
	}

	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if len(hostIDs) > 0 {
		if err := m.cloud.CascadeDeleteServers(tx, hostIDs); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec("DELETE FROM cloud_instances WHERE id = ?", instanceID); err != nil {
		return nil, err
	}

	return nil, tx.Commit()
}
