package network

import (
	"strings"
)

// ouiDB maps the first 3 octets of a MAC address (uppercase, colon-separated)
// to the vendor name.  Entries cover common enterprise / datacenter hardware.
var ouiDB = map[string]string{
	// Cisco Systems
	"00:00:0C": "Cisco Systems",
	"00:01:42": "Cisco Systems",
	"00:01:43": "Cisco Systems",
	"00:01:96": "Cisco Systems",
	"00:01:97": "Cisco Systems",
	"00:02:16": "Cisco Systems",
	"00:0A:41": "Cisco Systems",
	"00:1A:A1": "Cisco Systems",
	"58:97:BD": "Cisco Systems",
	"E8:ED:F3": "Cisco Systems",

	// Huawei Technologies
	"00:18:82": "Huawei Technologies",
	"00:E0:FC": "Huawei Technologies",
	"04:BD:70": "Huawei Technologies",
	"20:F3:A3": "Huawei Technologies",
	"48:57:02": "Huawei Technologies",

	// H3C (New H3C Group / HPE Networking)
	"00:0F:E2": "H3C Technologies",
	"00:1E:F7": "H3C Technologies",
	"3C:8C:F8": "H3C Technologies",

	// Aruba Networks (HPE)
	"00:0B:86": "Aruba Networks",
	"00:1A:1E": "Aruba Networks",
	"20:4C:03": "Aruba Networks",

	// TP-Link Technologies
	"00:27:19": "TP-Link Technologies",
	"14:CC:20": "TP-Link Technologies",
	"50:C7:BF": "TP-Link Technologies",
	"A0:F3:C1": "TP-Link Technologies",

	// D-Link Corporation
	"00:05:5D": "D-Link Corporation",
	"00:17:9A": "D-Link Corporation",
	"1C:7E:E5": "D-Link Corporation",

	// Juniper Networks
	"00:10:DB": "Juniper Networks",
	"00:1F:12": "Juniper Networks",
	"2C:6B:F5": "Juniper Networks",
	"3C:61:04": "Juniper Networks",

	// Ubiquiti Networks
	"00:15:6D": "Ubiquiti Networks",
	"00:27:22": "Ubiquiti Networks",
	"04:18:D6": "Ubiquiti Networks",
	"24:A4:3C": "Ubiquiti Networks",
	"DC:9F:DB": "Ubiquiti Networks",
	"F0:9F:C2": "Ubiquiti Networks",

	// MikroTik
	"00:0C:42": "MikroTik",
	"2C:C8:1B": "MikroTik",
	"48:8F:5A": "MikroTik",
	"D4:CA:6D": "MikroTik",

	// Fortinet
	"00:09:0F": "Fortinet",
	"00:0B:0A": "Fortinet",
	"90:6C:AC": "Fortinet",

	// Netgear
	"00:09:5B": "Netgear",
	"00:14:6C": "Netgear",
	"20:E5:2A": "Netgear",
	"A0:21:B7": "Netgear",

	// VMware
	"00:0C:29": "VMware",
	"00:50:56": "VMware",
	"00:05:69": "VMware",

	// QEMU / KVM (virtual)
	"52:54:00": "QEMU/KVM (Virtual)",

	// Dell Technologies
	"00:14:22": "Dell Technologies",
	"00:21:9B": "Dell Technologies",
	"18:66:DA": "Dell Technologies",
	"F8:DB:88": "Dell Technologies",

	// HP / HPE
	"00:02:A5": "HP/HPE",
	"00:11:0A": "HP/HPE",
	"3C:D9:2B": "HP/HPE",
	"9C:8E:99": "HP/HPE",

	// Lenovo
	"00:1E:67": "Lenovo",
	"00:23:AE": "Lenovo",
	"54:EE:75": "Lenovo",
	"F8:BC:12": "Lenovo",

	// Supermicro
	"00:25:90": "Supermicro",
	"0C:C4:7A": "Supermicro",
	"AC:1F:6B": "Supermicro",
}

// LookupVendor returns the vendor name for the given MAC address.
// It accepts the formats XX:XX:XX:XX:XX:XX, XX-XX-XX-XX-XX-XX, and
// XXXXXXXXXXXX (12 hex digits, no separator).  Returns "" when the OUI is
// unknown.
func LookupVendor(mac string) string {
	oui := normalizeOUI(mac)
	if oui == "" {
		return ""
	}
	return ouiDB[oui]
}

// normalizeOUI extracts and normalises the first 3 octets from a MAC address
// into "XX:XX:XX" uppercase format.  Returns "" on invalid input.
func normalizeOUI(mac string) string {
	// Strip known separators and work with raw hex.
	cleaned := strings.ToUpper(mac)
	cleaned = strings.ReplaceAll(cleaned, ":", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, ".", "")

	if len(cleaned) < 6 {
		return ""
	}
	// Validate that the first 6 chars are hex.
	for _, ch := range cleaned[:6] {
		if !((ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'F')) {
			return ""
		}
	}
	return cleaned[0:2] + ":" + cleaned[2:4] + ":" + cleaned[4:6]
}
