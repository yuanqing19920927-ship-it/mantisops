package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"mantisops/server/internal/config"
)

// OllamaProvider implements the Provider interface for Ollama.
type OllamaProvider struct {
	host   string
	cfg    config.OllamaConfig
	client *http.Client
	stream *http.Client
}

// NewOllamaProvider creates a new OllamaProvider.
func NewOllamaProvider(host string, cfg config.OllamaConfig) *OllamaProvider {
	return &OllamaProvider{
		host:   host,
		cfg:    cfg,
		client: &http.Client{Timeout: 5 * time.Minute},
		stream: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (o *OllamaProvider) Name() string { return "ollama" }

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  *ollamaOptions  `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done               bool `json:"done"`
	PromptEvalCount    int  `json:"prompt_eval_count"`
	EvalCount          int  `json:"eval_count"`
}

func (o *OllamaProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = o.cfg.ChatModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = o.cfg.MaxTokens
	}

	msgs := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = ollamaMessage{Role: m.Role, Content: m.Content}
	}

	body := ollamaRequest{
		Model:    model,
		Messages: msgs,
		Stream:   false,
		Options:  &ollamaOptions{NumPredict: maxTokens},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	url := o.host + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var or ollamaResponse
	if err := json.Unmarshal(respBody, &or); err != nil {
		return nil, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	return &CompletionResponse{
		Content: or.Message.Content,
		TokenUsage: TokenUsage{
			PromptTokens:     or.PromptEvalCount,
			CompletionTokens: or.EvalCount,
			TotalTokens:      or.PromptEvalCount + or.EvalCount,
		},
	}, nil
}

func (o *OllamaProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	model := req.Model
	if model == "" {
		model = o.cfg.ChatModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = o.cfg.MaxTokens
	}

	msgs := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = ollamaMessage{Role: m.Role, Content: m.Content}
	}

	body := ollamaRequest{
		Model:    model,
		Messages: msgs,
		Stream:   true,
		Options:  &ollamaOptions{NumPredict: maxTokens},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	url := o.host + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.stream.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: API error %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		o.parseNDJSON(ctx, resp.Body, ch)
	}()

	return ch, nil
}

func (o *OllamaProvider) parseNDJSON(ctx context.Context, r io.Reader, ch chan<- StreamChunk) {
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamChunk{Error: ctx.Err(), Done: true}
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var chunk ollamaResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		if chunk.Message.Content != "" {
			ch <- StreamChunk{Content: chunk.Message.Content}
		}

		if chunk.Done {
			usage := &TokenUsage{
				PromptTokens:     chunk.PromptEvalCount,
				CompletionTokens: chunk.EvalCount,
				TotalTokens:      chunk.PromptEvalCount + chunk.EvalCount,
			}
			ch <- StreamChunk{Done: true, Usage: usage}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamChunk{Error: fmt.Errorf("ollama: stream read error: %w", err), Done: true}
		return
	}

	ch <- StreamChunk{Done: true}
}
