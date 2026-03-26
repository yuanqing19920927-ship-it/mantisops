package store

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type VictoriaStore struct {
	baseURL string
	client  *http.Client
}

func NewVictoriaStore(baseURL string) *VictoriaStore {
	return &VictoriaStore{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (v *VictoriaStore) WriteMetrics(lines []string) error {
	body := strings.Join(lines, "\n")
	resp, err := v.client.Post(
		v.baseURL+"/api/v1/import/prometheus",
		"text/plain", strings.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("VM write failed: %d", resp.StatusCode)
	}
	return nil
}

func (v *VictoriaStore) Query(promql string) ([]byte, error) {
	resp, err := v.client.Get(fmt.Sprintf("%s/api/v1/query?query=%s", v.baseURL, url.QueryEscape(promql)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
