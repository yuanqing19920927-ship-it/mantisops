package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/network"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

// NetworkHandler handles HTTP requests for the network topology feature.
type NetworkHandler struct {
	networkStore *store.NetworkStore
	scanner      *network.Scanner
	snmpProber   *network.SNMPProber
	hub          *ws.Hub
	credStore    *store.CredentialStore
	serverStore  *store.ServerStore
}

// NewNetworkHandler constructs a NetworkHandler with all required dependencies.
func NewNetworkHandler(
	networkStore *store.NetworkStore,
	scanner *network.Scanner,
	snmpProber *network.SNMPProber,
	hub *ws.Hub,
	credStore *store.CredentialStore,
	serverStore *store.ServerStore,
) *NetworkHandler {
	return &NetworkHandler{
		networkStore: networkStore,
		scanner:      scanner,
		snmpProber:   snmpProber,
		hub:          hub,
		credStore:    credStore,
		serverStore:  serverStore,
	}
}

// ---- Scan -------------------------------------------------------------------

// StartScan godoc
// POST /api/v1/network/scan
// Body: { "subnets": ["192.168.10.0/24"] }
func (h *NetworkHandler) StartScan(c *gin.Context) {
	var req struct {
		Subnets []string `json:"subnets" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate every CIDR before starting anything.
	for _, cidr := range req.Subnets {
		if err := network.ValidateCIDR(cidr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subnet " + cidr + ": " + err.Error()})
			return
		}
	}

	// Transition scanner to "scanning" state; returns 409 if already running.
	scanCtx, _, err := h.scanner.BeginScan(context.Background())
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	// Build a snapshot of server IPs for matching discovered devices.
	serverIPMap := h.buildServerIPMap()

	// Run the full scan pipeline in a background goroutine.
	go h.runScanPipeline(scanCtx, req.Subnets, serverIPMap)

	c.JSON(http.StatusAccepted, gin.H{"ok": true, "message": "scan started"})
}

// scanResult is an internal record of a successfully stored device during a scan.
type scanResult struct {
	IP       string
	DeviceID int
}

// runScanPipeline orchestrates the full scan: ICMP sweep → OUI + SNMP → store.
func (h *NetworkHandler) runScanPipeline(ctx context.Context, subnets []string, serverIPMap map[string]int) {
	var allResults []scanResult

	for _, cidr := range subnets {
		// Check for cancellation between subnets.
		select {
		case <-ctx.Done():
			h.scanner.FinishScan("cancelled", "", nil)
			return
		default:
		}

		h.scanner.SetCurrentSubnet(cidr)
		log.Printf("[network/handler] scanning subnet %s", cidr)

		// Step 1: ICMP sweep to find alive hosts.
		pingResults := h.scanner.ScanSubnet(ctx, cidr)

		// Count alive hosts.
		aliveCount := 0
		for _, r := range pingResults {
			if r.Alive {
				aliveCount++
			}
		}

		// Upsert the subnet record so we have a subnetID.
		subnetID, err := h.networkStore.UpsertSubnet(cidr, cidr, "", len(pingResults), aliveCount)
		if err != nil {
			log.Printf("[network/handler] UpsertSubnet %s error: %v", cidr, err)
		}

		// Step 2: Enrich each alive host.
		for _, r := range pingResults {
			if !r.Alive {
				continue
			}

			select {
			case <-ctx.Done():
				h.scanner.FinishScan("cancelled", "", nil)
				return
			default:
			}

			vendor := network.LookupVendor(r.MAC)
			deviceType := "unknown"
			hostname := ""
			sysDescr, sysName, sysObjID, model := "", "", "", ""
			snmpSupported := false

			// SNMP probe.
			snmpResult := h.snmpProber.Probe(ctx, r.IP)
			if snmpResult != nil && snmpResult.Supported {
				snmpSupported = true
				deviceType = snmpResult.DeviceType
				sysDescr = snmpResult.SysDescr
				sysName = snmpResult.SysName
				sysObjID = snmpResult.SysObjectID
				model = snmpResult.Model
				if sysName != "" {
					hostname = sysName
				}
			}

			// Match to a known server.
			serverID := serverIPMap[r.IP]

			deviceID, err := h.networkStore.UpsertDevice(
				r.IP, r.MAC, vendor, deviceType, hostname,
				snmpSupported, 0,
				sysDescr, sysName, sysObjID, model,
				subnetID, serverID,
			)
			if err != nil {
				log.Printf("[network/handler] UpsertDevice %s error: %v", r.IP, err)
				continue
			}

			// Step 3: LLDP neighbor resolution for SNMP-capable devices.
			if snmpSupported && snmpResult != nil {
				neighbors := h.snmpProber.GetLLDPNeighbors(r.IP, snmpResult.Community)
				for _, nb := range neighbors {
					// Find the target device by hostname match in store.
					targetID := h.resolveNeighborID(nb.RemoteName)
					if targetID > 0 && targetID != deviceID {
						if linkErr := h.networkStore.UpsertLink(
							deviceID, targetID,
							nb.LocalPort, nb.RemotePort,
							"lldp", "",
						); linkErr != nil {
							log.Printf("[network/handler] UpsertLink %d->%d error: %v", deviceID, targetID, linkErr)
						}
					}
				}
			}

			allResults = append(allResults, scanResult{IP: r.IP, DeviceID: deviceID})
		}
	}

	h.scanner.FinishScan("completed", "", nil)
	log.Printf("[network/handler] scan completed: %d subnets, %d devices discovered", len(subnets), len(allResults))
}

// resolveNeighborID looks up a device ID by hostname (sysName).
// Returns 0 if not found.
func (h *NetworkHandler) resolveNeighborID(remoteName string) int {
	if remoteName == "" {
		return 0
	}
	devices, err := h.networkStore.ListDevices(0)
	if err != nil {
		return 0
	}
	lowerName := strings.ToLower(remoteName)
	for _, d := range devices {
		if strings.ToLower(d.SysName) == lowerName || strings.ToLower(d.Hostname) == lowerName {
			return d.ID
		}
	}
	return 0
}

// buildServerIPMap returns a map from IP string to server ID for all servers.
// IPAddresses is stored as a JSON array string, so we parse each entry.
func (h *NetworkHandler) buildServerIPMap() map[string]int {
	m := make(map[string]int)
	if h.serverStore == nil {
		return m
	}
	servers, err := h.serverStore.List()
	if err != nil {
		return m
	}
	for _, srv := range servers {
		var ips []string
		if err := json.Unmarshal([]byte(srv.IPAddresses), &ips); err == nil {
			for _, ip := range ips {
				if ip != "" {
					m[ip] = srv.ID
				}
			}
		}
	}
	return m
}

// GetScanStatus godoc
// GET /api/v1/network/scan/status
func (h *NetworkHandler) GetScanStatus(c *gin.Context) {
	c.JSON(http.StatusOK, h.scanner.GetStatus())
}

// CancelScan godoc
// DELETE /api/v1/network/scan
func (h *NetworkHandler) CancelScan(c *gin.Context) {
	h.scanner.Cancel()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Devices ----------------------------------------------------------------

// ListDevices godoc
// GET /api/v1/network/devices?subnet_id=1
func (h *NetworkHandler) ListDevices(c *gin.Context) {
	subnetID := 0
	if raw := c.Query("subnet_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subnet_id"})
			return
		}
		subnetID = id
	}
	devices, err := h.networkStore.ListDevices(subnetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if devices == nil {
		devices = []store.NetworkDevice{}
	}
	c.JSON(http.StatusOK, devices)
}

// GetDevice godoc
// GET /api/v1/network/devices/:id
func (h *NetworkHandler) GetDevice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	device, err := h.networkStore.GetDevice(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	c.JSON(http.StatusOK, device)
}

// UpdateDevice godoc
// PUT /api/v1/network/devices/:id
// Body: { "device_type": "switch", "hostname": "core-sw-01" }
func (h *NetworkHandler) UpdateDevice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		DeviceType string `json:"device_type"`
		Hostname   string `json:"hostname"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.networkStore.UpdateDevice(id, req.DeviceType, req.Hostname); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteDevice godoc
// DELETE /api/v1/network/devices/:id
func (h *NetworkHandler) DeleteDevice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.networkStore.DeleteDevice(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Topology ---------------------------------------------------------------

// GetTopology godoc
// GET /api/v1/network/topology
func (h *NetworkHandler) GetTopology(c *gin.Context) {
	topo, err := h.networkStore.GetTopology()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, topo)
}

// ---- Subnets ----------------------------------------------------------------

// ListSubnets godoc
// GET /api/v1/network/subnets
func (h *NetworkHandler) ListSubnets(c *gin.Context) {
	subnets, err := h.networkStore.ListSubnets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if subnets == nil {
		subnets = []store.NetworkSubnet{}
	}
	c.JSON(http.StatusOK, subnets)
}

// ---- SNMP Config ------------------------------------------------------------

const snmpCommunitiesKey = "network.snmp_communities"

// GetSNMPConfig godoc
// GET /api/v1/network/snmp-config
// Returns the list of SNMP communities stored in settings.
func (h *NetworkHandler) GetSNMPConfig(c *gin.Context) {
	communities := h.snmpProber.GetCommunities()
	c.JSON(http.StatusOK, gin.H{"communities": communities})
}

// UpdateSNMPConfig godoc
// PUT /api/v1/network/snmp-config
// Body: { "communities": ["public", "private"] }
func (h *NetworkHandler) UpdateSNMPConfig(c *gin.Context) {
	var req struct {
		Communities []string `json:"communities" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Communities) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "communities must not be empty"})
		return
	}

	// Persist to settings store via the prober.
	if err := h.snmpProber.SetCommunities(req.Communities); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

