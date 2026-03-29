package ai

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"mantisops/server/internal/config"
	"mantisops/server/internal/crypto"
	"mantisops/server/internal/store"
)

// Provider is the unified interface for LLM providers.
type Provider interface {
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
	Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error)
	Name() string
}

// CompletionRequest represents a request to an LLM provider.
type CompletionRequest struct {
	Model       string
	Messages    []Message
	MaxTokens   int
	Temperature float64
}

// Message represents a single message in a conversation.
type Message struct {
	Role    string
	Content string
}

// CompletionResponse represents a non-streaming response from an LLM provider.
type CompletionResponse struct {
	Content    string
	TokenUsage TokenUsage
}

// TokenUsage tracks token consumption for a request.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// StreamChunk represents a single chunk from a streaming response.
type StreamChunk struct {
	Content string
	Done    bool
	Error   error
	Usage   *TokenUsage
}

// ProviderInfo describes a provider's status for API responses.
type ProviderInfo struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Active     bool   `json:"active"`
}

// ProviderManager manages LLM provider instances and selection.
type ProviderManager struct {
	cfg       *config.AIConfig
	settings  *store.SettingsStore
	masterKey []byte
	providers map[string]Provider
	active    string
	mu        sync.RWMutex
}

// NewProviderManager creates a new ProviderManager and initialises all configured providers.
func NewProviderManager(cfg *config.AIConfig, settings *store.SettingsStore, masterKey []byte) *ProviderManager {
	pm := &ProviderManager{
		cfg:       cfg,
		settings:  settings,
		masterKey: masterKey,
		providers: make(map[string]Provider),
		active:    cfg.ActiveProvider,
	}
	pm.initProviders()
	return pm
}

// Reload re-reads settings from DB and re-initialises all providers.
func (pm *ProviderManager) Reload() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.providers = make(map[string]Provider)
	pm.initProviders()
}

// Active returns the currently active provider, or nil if none is configured.
func (pm *ProviderManager) Active() Provider {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.active == "" {
		return nil
	}
	return pm.providers[pm.active]
}

// SetActive switches the active provider. Returns an error if the provider is not registered.
func (pm *ProviderManager) SetActive(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if name == "" {
		pm.active = ""
		return nil
	}
	if _, ok := pm.providers[name]; !ok {
		return fmt.Errorf("provider %q not found", name)
	}
	pm.active = name
	return nil
}

// Get returns a specific provider by name, or nil if not found.
func (pm *ProviderManager) Get(name string) Provider {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.providers[name]
}

// List returns information about all registered providers.
func (pm *ProviderManager) List() []ProviderInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var infos []ProviderInfo
	for name := range pm.providers {
		infos = append(infos, ProviderInfo{
			Name:       name,
			Configured: true,
			Active:     name == pm.active,
		})
	}
	return infos
}

// ReportModel returns the report model name for the active provider,
// reading from DB settings first, then falling back to yaml config.
func (pm *ProviderManager) ReportModel() string {
	pm.mu.RLock()
	name := pm.active
	pm.mu.RUnlock()
	key := fmt.Sprintf("ai.%s.report_model", name)
	var fallback string
	switch name {
	case "claude":
		fallback = pm.cfg.Claude.ReportModel
	case "openai":
		fallback = pm.cfg.OpenAI.ReportModel
	case "ollama":
		fallback = pm.cfg.Ollama.ReportModel
	}
	return pm.resolveSetting(key, fallback)
}

// ChatModel returns the chat model name for the active provider,
// reading from DB settings first, then falling back to yaml config.
func (pm *ProviderManager) ChatModel() string {
	pm.mu.RLock()
	name := pm.active
	pm.mu.RUnlock()
	key := fmt.Sprintf("ai.%s.chat_model", name)
	var fallback string
	switch name {
	case "claude":
		fallback = pm.cfg.Claude.ChatModel
	case "openai":
		fallback = pm.cfg.OpenAI.ChatModel
	case "ollama":
		fallback = pm.cfg.Ollama.ChatModel
	}
	return pm.resolveSetting(key, fallback)
}

// resolveAPIKey tries to load an API key from the settings store (encrypted),
// falling back to the config value.
func (pm *ProviderManager) resolveAPIKey(settingsKey, configValue string) string {
	if pm.settings != nil && len(pm.masterKey) > 0 {
		encrypted, err := pm.settings.Get(settingsKey)
		if err == nil && encrypted != "" {
			plain, err := crypto.Decrypt(pm.masterKey, encrypted)
			if err == nil && len(plain) > 0 {
				return string(plain)
			}
			log.Printf("[ai] failed to decrypt %s from settings: %v", settingsKey, err)
		}
	}
	return configValue
}

// resolveSetting tries to load a plain-text setting from the settings store,
// falling back to the config value.
func (pm *ProviderManager) resolveSetting(settingsKey, configValue string) string {
	if pm.settings != nil {
		if v, err := pm.settings.Get(settingsKey); err == nil && v != "" {
			return v
		}
	}
	return configValue
}

// initProviders creates provider instances for all configured providers.
func (pm *ProviderManager) initProviders() {
	// Resolve active provider from DB, falling back to yaml config
	if active := pm.resolveSetting("ai.active_provider", pm.cfg.ActiveProvider); active != "" {
		pm.active = active
	}

	// Claude
	claudeKey := pm.resolveAPIKey("ai.claude.api_key", pm.cfg.Claude.APIKey)
	if claudeKey != "" {
		claudeCfg := pm.cfg.Claude
		if m := pm.resolveSetting("ai.claude.report_model", ""); m != "" {
			claudeCfg.ReportModel = m
		}
		if m := pm.resolveSetting("ai.claude.chat_model", ""); m != "" {
			claudeCfg.ChatModel = m
		}
		pm.providers["claude"] = NewClaudeProvider(claudeKey, claudeCfg)
		log.Printf("[ai] claude provider initialised")
	}

	// OpenAI
	openaiKey := pm.resolveAPIKey("ai.openai.api_key", pm.cfg.OpenAI.APIKey)
	if openaiKey != "" {
		openaiCfg := pm.cfg.OpenAI
		if u := pm.resolveSetting("ai.openai.base_url", ""); u != "" {
			openaiCfg.BaseURL = u
		}
		if m := pm.resolveSetting("ai.openai.report_model", ""); m != "" {
			openaiCfg.ReportModel = m
		}
		if m := pm.resolveSetting("ai.openai.chat_model", ""); m != "" {
			openaiCfg.ChatModel = m
		}
		pm.providers["openai"] = NewOpenAIProvider(openaiKey, openaiCfg)
		log.Printf("[ai] openai provider initialised")
	}

	// Ollama (no API key needed)
	host := pm.resolveSetting("ai.ollama.host", pm.cfg.Ollama.Host)
	if host != "" {
		host = strings.TrimRight(host, "/")
		ollamaCfg := pm.cfg.Ollama
		ollamaCfg.Host = host
		if m := pm.resolveSetting("ai.ollama.report_model", ""); m != "" {
			ollamaCfg.ReportModel = m
		}
		if m := pm.resolveSetting("ai.ollama.chat_model", ""); m != "" {
			ollamaCfg.ChatModel = m
		}
		pm.providers["ollama"] = NewOllamaProvider(host, ollamaCfg)
		log.Printf("[ai] ollama provider initialised (host=%s)", host)
	}
}
