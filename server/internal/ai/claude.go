package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mantisops/server/internal/config"
)

const claudeAPIURL = "https://api.anthropic.com/v1/messages"

// ClaudeProvider implements the Provider interface for Anthropic's Claude API.
type ClaudeProvider struct {
	apiKey string
	cfg    config.ClaudeConfig
	client *http.Client
	stream *http.Client
}

// NewClaudeProvider creates a new ClaudeProvider.
func NewClaudeProvider(apiKey string, cfg config.ClaudeConfig) *ClaudeProvider {
	return &ClaudeProvider{
		apiKey: apiKey,
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
		stream: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *ClaudeProvider) Name() string { return "claude" }

// claudeRequest is the request body for the Claude API.
type claudeRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	Messages  []claudeMessage  `json:"messages"`
	Stream    bool             `json:"stream,omitempty"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse is the response body for non-streaming requests.
type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *ClaudeProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = c.cfg.ChatModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.cfg.MaxTokens
	}

	msgs := make([]claudeMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = claudeMessage{Role: m.Role, Content: m.Content}
	}

	body := claudeRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  msgs,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("claude: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("claude: create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("claude: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("claude: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var cr claudeResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return nil, fmt.Errorf("claude: unmarshal response: %w", err)
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("claude: %s", cr.Error.Message)
	}

	content := ""
	if len(cr.Content) > 0 {
		content = cr.Content[0].Text
	}

	return &CompletionResponse{
		Content: content,
		TokenUsage: TokenUsage{
			PromptTokens:     cr.Usage.InputTokens,
			CompletionTokens: cr.Usage.OutputTokens,
			TotalTokens:      cr.Usage.InputTokens + cr.Usage.OutputTokens,
		},
	}, nil
}

func (c *ClaudeProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	model := req.Model
	if model == "" {
		model = c.cfg.ChatModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.cfg.MaxTokens
	}

	msgs := make([]claudeMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = claudeMessage{Role: m.Role, Content: m.Content}
	}

	body := claudeRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  msgs,
		Stream:    true,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("claude: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("claude: create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.stream.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("claude: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("claude: API error %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		c.parseSSE(ctx, resp.Body, ch)
	}()

	return ch, nil
}

func (c *ClaudeProvider) setHeaders(req *http.Request) {
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
}

func (c *ClaudeProvider) parseSSE(ctx context.Context, r io.Reader, ch chan<- StreamChunk) {
	scanner := bufio.NewScanner(r)
	var usage *TokenUsage

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamChunk{Error: ctx.Err(), Done: true}
			return
		default:
		}

		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta.Text != "" {
				ch <- StreamChunk{Content: event.Delta.Text}
			}
		case "message_delta":
			if event.Usage.OutputTokens > 0 {
				usage = &TokenUsage{
					CompletionTokens: event.Usage.OutputTokens,
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamChunk{Error: fmt.Errorf("claude: stream read error: %w", err), Done: true}
		return
	}

	ch <- StreamChunk{Done: true, Usage: usage}
}
