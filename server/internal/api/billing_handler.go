package api

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v4/client"
	rds "github.com/alibabacloud-go/rds-20140815/v3/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/gin-gonic/gin"
	"opsboard/server/internal/config"
	"opsboard/server/internal/model"
)

// ProbeResultProvider avoids direct dependency on probe package
type ProbeResultProvider interface {
	GetAllResults() []*model.ProbeResult
}

type BillingHandler struct {
	akID     string
	akSecret string
	regions  []string
	mu       sync.RWMutex
	cache    []BillingItem
	prober   ProbeResultProvider
}

type BillingItem struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Engine     string `json:"engine"`
	Spec       string `json:"spec"`
	ChargeType string `json:"charge_type"`
	ExpireDate string `json:"expire_date"`
	DaysLeft   int    `json:"days_left"`
	Status     string `json:"status"`
}

func NewBillingHandler(cfg config.AliyunConfig, prober ProbeResultProvider) *BillingHandler {
	akID := os.Getenv("ALIYUN_ACCESS_KEY_ID")
	if akID == "" {
		akID = cfg.AccessKeyID
	}
	akSecret := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")
	if akSecret == "" {
		akSecret = cfg.AccessKeySecret
	}

	regions := make(map[string]bool)
	for _, inst := range cfg.Instances {
		regions[inst.RegionID] = true
	}
	if len(regions) == 0 {
		regions["cn-hangzhou"] = true
	}
	var regionList []string
	for r := range regions {
		regionList = append(regionList, r)
	}

	h := &BillingHandler{
		akID:     akID,
		akSecret: akSecret,
		regions:  regionList,
		prober:   prober,
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
	ecsRds := h.cache
	h.mu.RUnlock()

	data := make([]BillingItem, len(ecsRds))
	copy(data, ecsRds)

	// Merge SSL cert data from Prober (real-time, not cached)
	if h.prober != nil {
		for _, r := range h.prober.GetAllResults() {
			if r.SSLExpiryDays != nil {
				status := "active"
				if *r.SSLExpiryDays <= 0 {
					status = "expired"
				}
				data = append(data, BillingItem{
					Type:       "ssl",
					ID:         fmt.Sprintf("probe-%d", r.RuleID),
					Name:       r.Name,
					Engine:     r.SSLIssuer,
					Spec:       r.Host,
					ChargeType: "—",
					ExpireDate: r.SSLExpiryDate,
					DaysLeft:   *r.SSLExpiryDays,
					Status:     status,
				})
			}
		}
	}

	if data == nil {
		data = []BillingItem{}
	}
	c.JSON(http.StatusOK, data)
}

func (h *BillingHandler) fetchAll() []BillingItem {
	var items []BillingItem
	for _, region := range h.regions {
		items = append(items, h.fetchECS(region)...)
		items = append(items, h.fetchRDS(region)...)
	}
	return items
}

func (h *BillingHandler) fetchECS(region string) []BillingItem {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(h.akID),
		AccessKeySecret: tea.String(h.akSecret),
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
				Type:       "ecs",
				ID:         tea.StringValue(inst.InstanceId),
				Name:       tea.StringValue(inst.InstanceName),
				Spec:       fmt.Sprintf("%dC/%dMB", tea.Int32Value(inst.Cpu), tea.Int32Value(inst.Memory)),
				ChargeType: chargeCN,
				ExpireDate: expireDate,
				DaysLeft:   daysLeft,
				Status:     tea.StringValue(inst.Status),
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

func (h *BillingHandler) fetchRDS(region string) []BillingItem {
	cfg := &openapi.Config{
		AccessKeyId:     tea.String(h.akID),
		AccessKeySecret: tea.String(h.akSecret),
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
			Type:       "rds",
			ID:         tea.StringValue(inst.DBInstanceId),
			Name:       tea.StringValue(inst.DBInstanceDescription),
			Engine:     engine,
			Spec:       tea.StringValue(inst.DBInstanceClass),
			ChargeType: chargeCN,
			ExpireDate: expireDate,
			DaysLeft:   daysLeft,
			Status:     tea.StringValue(inst.DBInstanceStatus),
		})
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
