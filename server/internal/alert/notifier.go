package alert

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"mantisops/server/internal/model"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

type DingtalkConfig struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

type WebhookConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

func SendDingtalk(cfg DingtalkConfig, title, text string) error {
	requestURL := cfg.URL
	if cfg.Secret != "" {
		ts := time.Now().UnixMilli()
		sign := dingtalkSign(ts, cfg.Secret)
		requestURL += fmt.Sprintf("&timestamp=%d&sign=%s", ts, url.QueryEscape(sign))
	}
	body, err := json.Marshal(map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  text,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal dingtalk body: %w", err)
	}
	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("dingtalk returned %d", resp.StatusCode)
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.ErrCode != 0 {
		return fmt.Errorf("dingtalk error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

func dingtalkSign(timestamp int64, secret string) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func SendWebhook(cfg WebhookConfig, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook body: %w", err)
	}
	req, err := http.NewRequest("POST", cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

// FormatFiringDingtalk formats a firing alert as DingTalk markdown
func FormatFiringDingtalk(event *model.AlertEvent) (string, string) {
	levelEmoji := "🟡"
	if event.Level == "critical" {
		levelEmoji = "🔴"
	} else if event.Level == "info" {
		levelEmoji = "🔵"
	}
	title := fmt.Sprintf("%s MantisOps 告警", levelEmoji)
	text := fmt.Sprintf("### %s %s\n- **目标**: %s\n- **当前值**: %.2f\n- **级别**: %s\n- **时间**: %s\n- **详情**: %s",
		levelEmoji, event.RuleName, event.TargetLabel, event.Value, event.Level,
		event.FiredAt.Format("2006-01-02 15:04:05"), event.Message)
	return title, text
}

// FormatResolvedDingtalk formats a resolved alert as DingTalk markdown
func FormatResolvedDingtalk(event *model.AlertEvent) (string, string) {
	title := "🟢 MantisOps 恢复"
	resolvedAt := time.Now().Format("2006-01-02 15:04:05")
	if event.ResolvedAt != nil {
		resolvedAt = event.ResolvedAt.Format("2006-01-02 15:04:05")
	}
	text := fmt.Sprintf("### 🟢 %s 恢复正常\n- **目标**: %s\n- **恢复时间**: %s",
		event.RuleName, event.TargetLabel, resolvedAt)
	return title, text
}

// FormatFiringWebhook formats a firing alert as webhook JSON payload
func FormatFiringWebhook(event *model.AlertEvent) map[string]interface{} {
	return map[string]interface{}{
		"event":     "alert_firing",
		"rule_name": event.RuleName,
		"target":    event.TargetLabel,
		"level":     event.Level,
		"value":     event.Value,
		"message":   event.Message,
		"fired_at":  event.FiredAt.Format(time.RFC3339),
	}
}

// FormatResolvedWebhook formats a resolved alert as webhook JSON payload
func FormatResolvedWebhook(event *model.AlertEvent) map[string]interface{} {
	resolvedAt := time.Now().Format(time.RFC3339)
	if event.ResolvedAt != nil {
		resolvedAt = event.ResolvedAt.Format(time.RFC3339)
	}
	return map[string]interface{}{
		"event":       "alert_resolved",
		"rule_name":   event.RuleName,
		"target":      event.TargetLabel,
		"level":       event.Level,
		"resolved_at": resolvedAt,
	}
}

// SendNotification dispatches to the appropriate channel type
func SendNotification(channel *model.NotificationChannel, event *model.AlertEvent, notifyType string) error {
	switch channel.Type {
	case "dingtalk":
		var cfg DingtalkConfig
		if err := json.Unmarshal([]byte(channel.Config), &cfg); err != nil {
			return fmt.Errorf("invalid dingtalk config: %w", err)
		}
		if notifyType == "resolved" {
			title, text := FormatResolvedDingtalk(event)
			return SendDingtalk(cfg, title, text)
		}
		title, text := FormatFiringDingtalk(event)
		return SendDingtalk(cfg, title, text)
	case "webhook":
		var cfg WebhookConfig
		if err := json.Unmarshal([]byte(channel.Config), &cfg); err != nil {
			return fmt.Errorf("invalid webhook config: %w", err)
		}
		if notifyType == "resolved" {
			return SendWebhook(cfg, FormatResolvedWebhook(event))
		}
		return SendWebhook(cfg, FormatFiringWebhook(event))
	default:
		return fmt.Errorf("unknown channel type: %s", channel.Type)
	}
}
