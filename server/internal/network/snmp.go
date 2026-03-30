package network

import (
	"context"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// OID constants used for basic system information.
const (
	oidSysDescr    = "1.3.6.1.2.1.1.1.0"
	oidSysName     = "1.3.6.1.2.1.1.5.0"
	oidSysObjectID = "1.3.6.1.2.1.1.2.0"

	// LLDP remote system name table (lldpRemSysName).
	oidLLDPRemSysName = "1.0.8802.1.1.2.1.4.1.1.9"
)

// SNMPResult holds the outcome of probing a device via SNMP.
type SNMPResult struct {
	Supported   bool   `json:"supported"`
	Community   string `json:"community"`
	SysDescr    string `json:"sys_descr"`
	SysName     string `json:"sys_name"`
	SysObjectID string `json:"sys_object_id"`
	Model       string `json:"model"`
	DeviceType  string `json:"device_type"`
}

// LLDPNeighbor represents a neighbour discovered via LLDP.
type LLDPNeighbor struct {
	LocalPort  string `json:"local_port"`
	RemoteIP   string `json:"remote_ip"`
	RemotePort string `json:"remote_port"`
	RemoteName string `json:"remote_name"`
}

// SNMPProber probes devices using SNMP v2c.
type SNMPProber struct {
	communities []string
	timeoutMs   int
}

// NewSNMPProber constructs a prober that tries each community in order.
func NewSNMPProber(communities []string, timeoutMs int) *SNMPProber {
	if timeoutMs <= 0 {
		timeoutMs = 2000
	}
	c := make([]string, len(communities))
	copy(c, communities)
	return &SNMPProber{communities: c, timeoutMs: timeoutMs}
}

// Probe tries each configured community against ip and returns the first
// successful result.  Returns nil if the device does not respond to any.
func (p *SNMPProber) Probe(ctx context.Context, ip string) *SNMPResult {
	for _, community := range p.communities {
		// Respect context cancellation between retries.
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if result := p.tryProbe(ip, community); result != nil {
			return result
		}
	}
	return nil
}

// tryProbe opens a single SNMP v2c session and fetches basic system MIB objects.
func (p *SNMPProber) tryProbe(ip, community string) *SNMPResult {
	g := &gosnmp.GoSNMP{
		Target:    ip,
		Port:      161,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(p.timeoutMs) * time.Millisecond,
		Retries:   0,
	}
	if err := g.Connect(); err != nil {
		return nil
	}
	defer g.Conn.Close()

	oids := []string{oidSysDescr, oidSysName, oidSysObjectID}
	result, err := g.Get(oids)
	if err != nil || result == nil {
		return nil
	}

	out := &SNMPResult{
		Supported: true,
		Community: community,
	}
	for _, pdu := range result.Variables {
		val := pduString(pdu)
		switch pdu.Name {
		case "." + oidSysDescr, oidSysDescr:
			out.SysDescr = val
		case "." + oidSysName, oidSysName:
			out.SysName = val
		case "." + oidSysObjectID, oidSysObjectID:
			out.SysObjectID = val
		}
	}

	out.DeviceType = p.inferDeviceType(out.SysDescr, out.SysObjectID)
	out.Model = p.inferModel(out.SysDescr)
	return out
}

// GetLLDPNeighbors walks the LLDP remote system-name table and returns
// the names found.  LocalPort / RemoteIP / RemotePort are left empty because
// they require additional OID walks that are beyond the scope of this function.
func (p *SNMPProber) GetLLDPNeighbors(ip, community string) []LLDPNeighbor {
	g := &gosnmp.GoSNMP{
		Target:    ip,
		Port:      161,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(p.timeoutMs) * time.Millisecond,
		Retries:   0,
	}
	if err := g.Connect(); err != nil {
		return nil
	}
	defer g.Conn.Close()

	pdus, err := g.BulkWalkAll(oidLLDPRemSysName)
	if err != nil || len(pdus) == 0 {
		return nil
	}

	var neighbors []LLDPNeighbor
	for _, pdu := range pdus {
		name := pduString(pdu)
		if name == "" {
			continue
		}
		neighbors = append(neighbors, LLDPNeighbor{
			RemoteName: name,
		})
	}
	return neighbors
}

// inferDeviceType performs keyword matching on sysDescr and sysObjectID to
// classify the device.
func (p *SNMPProber) inferDeviceType(sysDescr, sysObjID string) string {
	combined := strings.ToLower(sysDescr + " " + sysObjID)

	switch {
	case containsAny(combined, "firewall", "fortigate", "paloalto", "checkpoint", "asa", "srx"):
		return "firewall"
	case containsAny(combined, "wireless", " ap ", "access point", "wlan", "802.11", "aironet", "aruba"):
		return "ap"
	case containsAny(combined, "router", "nexus", "asr", "isr", "routing"):
		return "router"
	case containsAny(combined, "switch", "catalyst", "procurve", "s5700", "s2700", "s3700"):
		return "switch"
	case containsAny(combined, "printer", "laserjet", "officejet", "ricoh", "xerox", "kyocera"):
		return "printer"
	default:
		return "unknown"
	}
}

// inferModel extracts a model string from sysDescr: either the first line or
// the first 80 characters, whichever is shorter.
func (p *SNMPProber) inferModel(sysDescr string) string {
	if sysDescr == "" {
		return ""
	}
	line := strings.SplitN(sysDescr, "\n", 2)[0]
	line = strings.TrimSpace(line)
	if len(line) > 80 {
		return line[:80]
	}
	return line
}

// ---- helpers ----------------------------------------------------------------

// pduString converts a PDU value to a plain string.
func pduString(pdu gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	default:
		return ""
	}
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
