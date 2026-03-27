package api

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	cas "github.com/alibabacloud-go/cas-20200407/v3/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v4/client"
	rds "github.com/alibabacloud-go/rds-20140815/v3/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/gin-gonic/gin"
	"mantisops/server/internal/config"
	"mantisops/server/internal/store"
)

type BillingHandler struct {
	cloudStore *store.CloudStore
	credStore  *store.CredentialStore
	fallbackCfg config.AliyunConfig // 向后兼容：当数据库无账号时使用

	mu    sync.RWMutex
	cache []BillingItem
}

type BillingItem struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Engine      string `json:"engine"`
	Spec        string `json:"spec"`
	ChargeType  string `json:"charge_type"`
	ExpireDate  string `json:"expire_date"`
	DaysLeft    int    `json:"days_left"`
	Status      string `json:"status"`
	AccountID   int    `json:"account_id"`
	AccountName string `json:"account_name"`
}

// billingAccount 封装一个可用于查询的云账号信息
type billingAccount struct {
	id       int
	name     string
	akID     string
	akSecret string
	regions  []string
}

func NewBillingHandler(cloudStore *store.CloudStore, credStore *store.CredentialStore, fallbackCfg config.AliyunConfig) *BillingHandler {
	h := &BillingHandler{
		cloudStore:  cloudStore,
		credStore:   credStore,
		fallbackCfg: fallbackCfg,
	}

	// 启动时后台预加载，不阻塞主线程
	go h.refreshLoop()

	return h
}

// refreshLoop 后台定时刷新，每小时更新一次
func (h *BillingHandler) refreshLoop() {
	// 首次立即加载
	h.refresh()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		h.refresh()
	}
}

func (h *BillingHandler) refresh() {
	items := h.fetchAll()
	h.mu.Lock()
	h.cache = items
	h.mu.Unlock()
	log.Printf("[billing] refreshed: %d items", len(items))
}

// List API 直接返回缓存，无阻塞
func (h *BillingHandler) List(c *gin.Context) {
	h.mu.RLock()
	data := h.cache
	h.mu.RUnlock()

	if data == nil {
		data = []BillingItem{}
	}
	c.JSON(http.StatusOK, data)
}

// loadAccounts 从数据库加载云账号，若无则回退到配置文件
func (h *BillingHandler) loadAccounts() []billingAccount {
	var accounts []billingAccount

	// 尝试从数据库加载
	if h.cloudStore != nil && h.credStore != nil {
		dbAccounts, err := h.cloudStore.ListAccounts()
		if err != nil {
			log.Printf("[billing] list cloud accounts error: %v", err)
		} else {
			for _, acct := range dbAccounts {
				cred, err := h.credStore.Get(acct.CredentialID)
				if err != nil {
					log.Printf("[billing] get credential %d for account %s error: %v", acct.CredentialID, acct.Name, err)
					continue
				}
				akID := cred.Data["access_key_id"]
				akSecret := cred.Data["access_key_secret"]
				if akID == "" || akSecret == "" {
					log.Printf("[billing] account %s: missing AK/SK in credential %d", acct.Name, acct.CredentialID)
					continue
				}
				regions := acct.RegionIDs
				if len(regions) == 0 {
					regions = []string{"cn-hangzhou"}
				}
				accounts = append(accounts, billingAccount{
					id:       acct.ID,
					name:     acct.Name,
					akID:     akID,
					akSecret: akSecret,
					regions:  regions,
				})
			}
		}
	}

	// 回退到配置文件
	if len(accounts) == 0 {
		akID := os.Getenv("ALIYUN_ACCESS_KEY_ID")
		if akID == "" {
			akID = h.fallbackCfg.AccessKeyID
		}
		akSecret := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")
		if akSecret == "" {
			akSecret = h.fallbackCfg.AccessKeySecret
		}
		if akID == "" || akSecret == "" {
			return nil
		}

		regions := make(map[string]bool)
		for _, inst := range h.fallbackCfg.Instances {
			regions[inst.RegionID] = true
		}
		if len(regions) == 0 {
			regions["cn-hangzhou"] = true
		}
		var regionList []string
		for r := range regions {
			regionList = append(regionList, r)
		}
		accounts = append(accounts, billingAccount{
			id:       0,
			name:     "",
			akID:     akID,
			akSecret: akSecret,
			regions:  regionList,
		})
	}

	return accounts
}

func (h *BillingHandler) fetchAll() []BillingItem {
	accounts := h.loadAccounts()
	var items []BillingItem
	for _, acct := range accounts {
		for _, region := range acct.regions {
			items = append(items, h.fetchECS(acct, region)...)
			items = append(items, h.fetchRDS(acct, region)...)
		}
		items = append(items, h.fetchSSL(acct)...)
	}
	return items
}

func (h *BillingHandler) fetchECS(acct billingAccount, region string) []BillingItem {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(acct.akID),
		AccessKeySecret: tea.String(acct.akSecret),
		RegionId:        tea.String(region),
		Endpoint:        tea.String(fmt.Sprintf("ecs.%s.aliyuncs.com", region)),
	}
	client, err := ecs.NewClient(cfg)
	if err != nil {
		log.Printf("[billing] ecs client error: %v", err)
		return nil
	}

	var items []BillingItem
	page := int32(1)
	for {
		req := &ecs.DescribeInstancesRequest{
			RegionId:   tea.String(region),
			PageSize:   tea.Int32(50),
			PageNumber: tea.Int32(page),
		}
		resp, err := client.DescribeInstances(req)
		if err != nil {
			log.Printf("[billing] ecs query error: %v", err)
			break
		}
		for _, inst := range resp.Body.Instances.Instance {
			chargeType := tea.StringValue(inst.InstanceChargeType)
			expireStr := tea.StringValue(inst.ExpiredTime)
			expireDate, daysLeft := parseExpire(expireStr)

			chargeCN := "按量付费"
			if chargeType == "PrePaid" {
				chargeCN = "包年包月"
			}

			items = append(items, BillingItem{
				Type:        "ecs",
				ID:          tea.StringValue(inst.InstanceId),
				Name:        tea.StringValue(inst.InstanceName),
				Spec:        fmt.Sprintf("%dC/%dMB", tea.Int32Value(inst.Cpu), tea.Int32Value(inst.Memory)),
				ChargeType:  chargeCN,
				ExpireDate:  expireDate,
				DaysLeft:    daysLeft,
				Status:      tea.StringValue(inst.Status),
				AccountID:   acct.id,
				AccountName: acct.name,
			})
		}
		total := tea.Int32Value(resp.Body.TotalCount)
		if int32(len(items)) >= total {
			break
		}
		page++
	}
	return items
}

func (h *BillingHandler) fetchRDS(acct billingAccount, region string) []BillingItem {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(acct.akID),
		AccessKeySecret: tea.String(acct.akSecret),
		RegionId:        tea.String(region),
		Endpoint:        tea.String(fmt.Sprintf("rds.%s.aliyuncs.com", region)),
	}
	client, err := rds.NewClient(cfg)
	if err != nil {
		log.Printf("[billing] rds client error: %v", err)
		return nil
	}

	req := &rds.DescribeDBInstancesRequest{
		RegionId: tea.String(region),
		PageSize: tea.Int32(50),
	}
	resp, err := client.DescribeDBInstances(req)
	if err != nil {
		log.Printf("[billing] rds query error: %v", err)
		return nil
	}

	var items []BillingItem
	for _, inst := range resp.Body.Items.DBInstance {
		payType := tea.StringValue(inst.PayType)
		expireStr := tea.StringValue(inst.ExpireTime)
		expireDate, daysLeft := parseExpire(expireStr)

		chargeCN := "按量付费"
		if payType == "Prepaid" {
			chargeCN = "包年包月"
		}

		engine := fmt.Sprintf("%s %s", tea.StringValue(inst.Engine), tea.StringValue(inst.EngineVersion))

		items = append(items, BillingItem{
			Type:        "rds",
			ID:          tea.StringValue(inst.DBInstanceId),
			Name:        tea.StringValue(inst.DBInstanceDescription),
			Engine:      engine,
			Spec:        tea.StringValue(inst.DBInstanceClass),
			ChargeType:  chargeCN,
			ExpireDate:  expireDate,
			DaysLeft:    daysLeft,
			Status:      tea.StringValue(inst.DBInstanceStatus),
			AccountID:   acct.id,
			AccountName: acct.name,
		})
	}
	return items
}

func (h *BillingHandler) fetchSSL(acct billingAccount) []BillingItem {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(acct.akID),
		AccessKeySecret: tea.String(acct.akSecret),
		Endpoint:        tea.String("cas.aliyuncs.com"),
	}
	client, err := cas.NewClient(cfg)
	if err != nil {
		log.Printf("[billing] cas client error: %v", err)
		return nil
	}

	var items []BillingItem
	seen := make(map[string]bool) // 按域名去重（CPACK 和 CERT 可能有重复）

	// 查询 CPACK（购买的证书包）和 CERT（托管/个人证书）两种类型
	for _, orderType := range []string{"CPACK", "CERT"} {
		page := int64(1)
		for {
			req := &cas.ListUserCertificateOrderRequest{
				ShowSize:    tea.Int64(50),
				CurrentPage: tea.Int64(page),
				OrderType:   tea.String(orderType),
			}
			resp, err := client.ListUserCertificateOrder(req)
			if err != nil {
				log.Printf("[billing] ssl query (%s) error: %v", orderType, err)
				break
			}

			for _, cert := range resp.Body.CertificateOrderList {
				domain := tea.StringValue(cert.Domain)
				if domain == "" {
					domain = tea.StringValue(cert.CommonName)
				}
				status := tea.StringValue(cert.Status)
				instanceID := tea.StringValue(cert.InstanceId)
				brand := tea.StringValue(cert.RootBrand)

				// 只展示 ISSUED（有效）和 EXPIRED（已过期）的证书
				if status != "ISSUED" && status != "EXPIRED" {
					continue
				}

				endTime := tea.Int64Value(cert.CertEndTime)
				if endTime == 0 {
					continue
				}
				expireAt := time.Unix(endTime/1000, 0)
				daysLeft := int(math.Ceil(time.Until(expireAt).Hours() / 24))

				// 过期超过 90 天的不展示
				if daysLeft < -90 {
					continue
				}

				// 按域名+到期日去重（同一域名可能在 CPACK 和 CERT 中都有）
				dedup := domain + expireAt.Format("2006-01-02")
				if seen[dedup] {
					continue
				}
				seen[dedup] = true

				expireDate := expireAt.Format("2006-01-02")
				statusCN := "active"
				if status == "EXPIRED" || daysLeft <= 0 {
					statusCN = "expired"
				}

				items = append(items, BillingItem{
					Type:        "ssl",
					ID:          instanceID,
					Name:        domain,
					Engine:      brand,
					Spec:        domain,
					ChargeType:  "—",
					ExpireDate:  expireDate,
					DaysLeft:    daysLeft,
					Status:      statusCN,
					AccountID:   acct.id,
					AccountName: acct.name,
				})
			}

			total := tea.Int64Value(resp.Body.TotalCount)
			fetched := page * 50
			if fetched >= total || len(resp.Body.CertificateOrderList) == 0 {
				break
			}
			page++
		}
	}
	return items
}

func parseExpire(s string) (string, int) {
	if s == "" {
		return "", -1
	}
	layouts := []string{
		"2006-01-02T15:04Z",
		"2006-01-02T15:04:05Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			days := int(math.Ceil(time.Until(t).Hours() / 24))
			return t.Format("2006-01-02"), days
		}
	}
	return s, -1
}
