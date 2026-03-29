package ai

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	"unicode/utf8"

	"mantisops/server/internal/config"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

// ChatEngine manages AI chat conversations including message handling,
// streaming LLM responses via WebSocket, context injection, and auto-title generation.
type ChatEngine struct {
	store         *store.AIStore
	provider      *ProviderManager
	collector     *DataCollector
	hub           *ws.Hub
	maxHistory    int
	maxMsgLen     int
	ctxRefreshAll bool
	streamGates   map[string]chan struct{}
	mu            sync.Mutex
}

// NewChatEngine creates a ChatEngine with the given dependencies and configuration.
func NewChatEngine(aiStore *store.AIStore, provider *ProviderManager, collector *DataCollector, hub *ws.Hub, cfg config.AIChatConfig) *ChatEngine {
	return &ChatEngine{
		store:         aiStore,
		provider:      provider,
		collector:     collector,
		hub:           hub,
		maxHistory:    cfg.MaxHistoryMessages,
		maxMsgLen:     cfg.MaxMessageLength,
		ctxRefreshAll: cfg.SystemCtxRefresh,
		streamGates:   make(map[string]chan struct{}),
	}
}

// SendMessage initiates a new chat message exchange. It creates the user
// message, a placeholder assistant message, and launches a background
// streaming goroutine. The caller must trigger OnStreamSubscribe with the
// returned streamID to ungate the stream.
func (e *ChatEngine) SendMessage(convID int64, content, requestID, user string) (userMsgID, assistantMsgID int64, streamID string, err error) {
	// Idempotency check
	if requestID != "" && convID != 0 {
		existing, findErr := e.store.FindByRequestID(convID, requestID)
		if findErr != nil {
			return 0, 0, "", fmt.Errorf("idempotency check: %w", findErr)
		}
		if existing != nil {
			// Find the corresponding assistant message (next message after user)
			msgs, msgsErr := e.store.GetMessagesByConversation(convID)
			if msgsErr != nil {
				return 0, 0, "", fmt.Errorf("get messages: %w", msgsErr)
			}
			var asstID int64
			for i, m := range msgs {
				if m.ID == existing.ID && i+1 < len(msgs) {
					asstID = msgs[i+1].ID
					break
				}
			}
			return existing.ID, asstID, "", nil
		}
	}

	// Create new conversation if needed
	if convID == 0 {
		p := e.provider.Active()
		providerName := ""
		if p != nil {
			providerName = p.Name()
		}
		conv := &store.AIConversation{
			Title:    "New Chat",
			User:     user,
			Provider: providerName,
			Model:    "",
		}
		newID, createErr := e.store.CreateConversation(conv)
		if createErr != nil {
			return 0, 0, "", fmt.Errorf("create conversation: %w", createErr)
		}
		convID = newID
	}

	// Truncate content if exceeds maxMsgLen runes
	if utf8.RuneCountInString(content) > e.maxMsgLen {
		runes := []rune(content)
		content = string(runes[:e.maxMsgLen])
	}

	// Create user message
	userMsg := &store.AIMessage{
		ConversationID: convID,
		Role:           "user",
		Content:        content,
		Status:         "done",
		RequestID:      requestID,
	}
	userMsgID, err = e.store.CreateMessage(userMsg)
	if err != nil {
		return 0, 0, "", fmt.Errorf("create user message: %w", err)
	}

	// Create assistant placeholder message
	asstMsg := &store.AIMessage{
		ConversationID: convID,
		Role:           "assistant",
		Content:        "",
		Status:         "streaming",
	}
	assistantMsgID, err = e.store.CreateMessage(asstMsg)
	if err != nil {
		return 0, 0, "", fmt.Errorf("create assistant message: %w", err)
	}

	// Generate stream ID
	streamID = fmt.Sprintf("%d-%d", time.Now().UnixNano(), assistantMsgID)

	// Create gate channel
	e.mu.Lock()
	e.streamGates[streamID] = make(chan struct{}, 1)
	e.mu.Unlock()

	// Launch streaming goroutine
	go e.processStream(convID, assistantMsgID, streamID, user)

	return userMsgID, assistantMsgID, streamID, nil
}

// OnStreamSubscribe is called by Hub when a client sends ai_stream_subscribe.
// It signals the gate channel to allow processStream to proceed.
func (e *ChatEngine) OnStreamSubscribe(streamID string) {
	e.mu.Lock()
	ch, ok := e.streamGates[streamID]
	e.mu.Unlock()
	if ok {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// processStream handles the background streaming of an AI response.
func (e *ChatEngine) processStream(convID, assistantMsgID int64, streamID, user string) {
	defer func() {
		e.mu.Lock()
		delete(e.streamGates, streamID)
		e.mu.Unlock()
		e.hub.CleanupAIStream(streamID)
	}()

	// Wait for client subscription with 5s timeout
	e.mu.Lock()
	gate := e.streamGates[streamID]
	e.mu.Unlock()

	select {
	case <-gate:
		// Client subscribed, proceed
	case <-time.After(5 * time.Second):
		errMsg := "stream subscription timeout"
		_ = e.store.UpdateMessageFailed(assistantMsgID, errMsg)
		e.hub.BroadcastAIStreamJSON(streamID, map[string]interface{}{
			"type":            "ai_chat_error",
			"stream_id":       streamID,
			"conversation_id": convID,
			"message_id":      assistantMsgID,
			"error":           errMsg,
		})
		return
	}

	// Collect system context
	systemCtx := e.collector.CollectChatContext()

	// Build message list
	messages, err := e.buildMessages(convID, systemCtx)
	if err != nil {
		errMsg := fmt.Sprintf("build messages: %v", err)
		_ = e.store.UpdateMessageFailed(assistantMsgID, errMsg)
		e.hub.BroadcastAIStreamJSON(streamID, map[string]interface{}{
			"type":            "ai_chat_error",
			"stream_id":       streamID,
			"conversation_id": convID,
			"message_id":      assistantMsgID,
			"error":           errMsg,
		})
		return
	}

	// Get active provider
	provider := e.provider.Active()
	if provider == nil {
		errMsg := "no active AI provider configured"
		_ = e.store.UpdateMessageFailed(assistantMsgID, errMsg)
		e.hub.BroadcastAIStreamJSON(streamID, map[string]interface{}{
			"type":            "ai_chat_error",
			"stream_id":       streamID,
			"conversation_id": convID,
			"message_id":      assistantMsgID,
			"error":           errMsg,
		})
		return
	}

	// Start streaming
	ctx := context.Background()
	chunks, err := provider.Stream(ctx, &CompletionRequest{
		Model:    e.provider.ChatModel(),
		Messages: messages,
	})
	if err != nil {
		errMsg := fmt.Sprintf("stream request: %v", err)
		_ = e.store.UpdateMessageFailed(assistantMsgID, errMsg)
		e.hub.BroadcastAIStreamJSON(streamID, map[string]interface{}{
			"type":            "ai_chat_error",
			"stream_id":       streamID,
			"conversation_id": convID,
			"message_id":      assistantMsgID,
			"error":           errMsg,
		})
		return
	}

	// Read chunks
	var fullContent string
	var lastUsage *TokenUsage
	for chunk := range chunks {
		if chunk.Error != nil {
			errMsg := chunk.Error.Error()
			_ = e.store.UpdateMessageFailed(assistantMsgID, errMsg)
			e.hub.BroadcastAIStreamJSON(streamID, map[string]interface{}{
				"type":            "ai_chat_error",
				"stream_id":       streamID,
				"conversation_id": convID,
				"message_id":      assistantMsgID,
				"error":           errMsg,
			})
			return
		}

		fullContent += chunk.Content
		if chunk.Usage != nil {
			lastUsage = chunk.Usage
		}

		if !chunk.Done {
			e.hub.BroadcastAIStreamJSON(streamID, map[string]interface{}{
				"type":            "ai_chat_chunk",
				"stream_id":       streamID,
				"conversation_id": convID,
				"message_id":      assistantMsgID,
				"content":         chunk.Content,
				"done":            false,
			})
		}
	}

	// Build final chunk payload
	finalPayload := map[string]interface{}{
		"type":            "ai_chat_chunk",
		"stream_id":       streamID,
		"conversation_id": convID,
		"message_id":      assistantMsgID,
		"content":         "",
		"done":            true,
	}
	if lastUsage != nil {
		finalPayload["token_usage"] = map[string]interface{}{
			"prompt_tokens":     lastUsage.PromptTokens,
			"completion_tokens": lastUsage.CompletionTokens,
			"total_tokens":      lastUsage.TotalTokens,
		}
	}
	e.hub.BroadcastAIStreamJSON(streamID, finalPayload)

	// Update message as completed
	promptTokens := 0
	completionTokens := 0
	if lastUsage != nil {
		promptTokens = lastUsage.PromptTokens
		completionTokens = lastUsage.CompletionTokens
	}
	_ = e.store.UpdateMessageCompleted(assistantMsgID, fullContent, promptTokens, completionTokens)

	// Auto-title for first exchange (user + assistant = 2 messages)
	conv, convErr := e.store.GetConversation(convID)
	if convErr == nil && conv.MessageCount <= 2 {
		// Get the user message content for title generation
		msgs, msgsErr := e.store.GetMessagesByConversation(convID)
		if msgsErr == nil && len(msgs) > 0 {
			var userContent string
			for _, m := range msgs {
				if m.Role == "user" {
					userContent = m.Content
					break
				}
			}
			if userContent != "" {
				go e.autoTitle(convID, userContent, fullContent)
			}
		}
	}
}

// buildMessages constructs the message list for the LLM request, including
// system prompt with context and conversation history.
func (e *ChatEngine) buildMessages(convID int64, systemCtx string) ([]Message, error) {
	msgs, err := e.store.GetMessagesByConversation(convID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	// Filter to user/assistant roles only
	var filtered []store.AIMessage
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			filtered = append(filtered, m)
		}
	}

	// Trim history if too long: keep first 2 + last (maxHistory-2)
	if len(filtered) > e.maxHistory {
		first := filtered[:2]
		last := filtered[len(filtered)-(e.maxHistory-2):]
		filtered = append(first, last...)
	}

	// Build final message list with system prompt prepended
	result := make([]Message, 0, len(filtered)+1)
	result = append(result, Message{
		Role:    "system",
		Content: ChatSystemPrompt(systemCtx),
	})
	for _, m := range filtered {
		result = append(result, Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	return result, nil
}

// autoTitle generates a conversation title using the LLM and updates the conversation.
func (e *ChatEngine) autoTitle(convID int64, userMsg, assistantMsg string) {
	provider := e.provider.Active()
	if provider == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, &CompletionRequest{
		Messages: []Message{
			{Role: "user", Content: GenerateTitlePrompt(userMsg, assistantMsg)},
		},
	})
	if err != nil {
		log.Printf("[chat] auto-title failed for conv %d: %v", convID, err)
		return
	}

	title := resp.Content
	// Truncate to 20 runes max
	if utf8.RuneCountInString(title) > 20 {
		title = string([]rune(title)[:20])
	}

	if err := e.store.UpdateConversationTitle(convID, title); err != nil {
		log.Printf("[chat] update title failed for conv %d: %v", convID, err)
	}
}
