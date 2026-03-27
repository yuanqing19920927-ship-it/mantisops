package hostid

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
)

const persistPath = "/etc/mantisops/host_id"

func Get(configID string) string {
	if configID != "" {
		return configID
	}
	if data, err := os.ReadFile(persistPath); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}
	id := generate()
	os.MkdirAll("/etc/mantisops", 0755)
	os.WriteFile(persistPath, []byte(id), 0644)
	return id
}

func generate() string {
	if data, err := os.ReadFile("/etc/machine-id"); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}
	hostname, _ := os.Hostname()
	h := sha256.Sum256([]byte(hostname))
	return fmt.Sprintf("%x", h[:8])
}
