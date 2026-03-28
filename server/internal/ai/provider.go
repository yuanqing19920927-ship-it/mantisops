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

// initProviders creates provider instances for all configured providers.
func (pm *ProviderManager) initProviders() {
	// Claude
	claudeKey := pm.resolveAPIKey("ai.claude.api_key", pm.cfg.Claude.APIKey)
	if claudeKey != "" {
		pm.providers["claude"] = NewClaudeProvider(claudeKey, pm.cfg.Claude)
		log.Printf("[ai] claude provider initialised")
	}

	// OpenAI
	openaiKey := pm.resolveAPIKey("ai.openai.api_key", pm.cfg.OpenAI.APIKey)
	if openaiKey != "" {
		pm.providers["openai"] = NewOpenAIProvider(openaiKey, pm.cfg.OpenAI)
		log.Printf("[ai] openai provider initialised")
	}

	// Ollama (no API key needed)
	host := pm.cfg.Ollama.Host
	if host != "" {
		host = strings.TrimRight(host, "/")
		pm.providers["ollama"] = NewOllamaProvider(host, pm.cfg.Ollama)
		log.Printf("[ai] ollama provider initialised")
	}
}
