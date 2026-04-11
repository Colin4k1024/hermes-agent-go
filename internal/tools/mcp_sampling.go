package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

// SamplingHandler handles MCP sampling/createMessage requests from MCP servers.
// It converts MCP messages to LLM chat messages, calls the LLM, and returns
// the result in MCP format. It enforces per-server rate limits and loop depth.
type SamplingHandler struct {
	llmClient    *llm.Client
	maxRPM       int
	maxLoopDepth int

	mu         sync.Mutex
	callCounts map[string]int // per-server call counts for rate limiting
	lastReset  time.Time
	depths     map[string]int // per-server current nesting depth
}

// NewSamplingHandler creates a SamplingHandler with the given LLM client.
// If client is nil, sampling requests will be rejected.
func NewSamplingHandler(client *llm.Client) *SamplingHandler {
	return &SamplingHandler{
		llmClient:    client,
		maxRPM:       10,
		maxLoopDepth: 5,
		callCounts:   make(map[string]int),
		lastReset:    time.Now(),
		depths:       make(map[string]int),
	}
}

// mcpSamplingRequest represents the params of a sampling/createMessage request.
type mcpSamplingRequest struct {
	Messages         []mcpSamplingMessage `json:"messages"`
	ModelPreferences *mcpModelPreferences `json:"modelPreferences,omitempty"`
	SystemPrompt     string               `json:"systemPrompt,omitempty"`
	MaxTokens        int                  `json:"maxTokens,omitempty"`
}

// mcpSamplingMessage is a message in the MCP sampling protocol.
type mcpSamplingMessage struct {
	Role    string             `json:"role"`
	Content mcpSamplingContent `json:"content"`
}

// mcpSamplingContent is the content of an MCP sampling message.
type mcpSamplingContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// mcpModelPreferences captures model preferences from the MCP server.
type mcpModelPreferences struct {
	Hints []struct {
		Name string `json:"name,omitempty"`
	} `json:"hints,omitempty"`
}

// mcpSamplingResult is the response to a sampling/createMessage request.
type mcpSamplingResult struct {
	Role    string             `json:"role"`
	Content mcpSamplingContent `json:"content"`
	Model   string             `json:"model"`
}

// HandleRequest processes a sampling/createMessage JSON-RPC request and returns
// the JSON-RPC response. serverName identifies the MCP server for rate limiting.
func (h *SamplingHandler) HandleRequest(ctx context.Context, serverName string, id int64, params json.RawMessage) *jsonRPCResponse {
	if h.llmClient == nil {
		return h.errorResponse(id, -32603, "sampling not available: no LLM client configured")
	}

	// Rate limit check
	if err := h.checkRateLimit(serverName); err != nil {
		slog.Warn("MCP sampling rate limited", "server", serverName, "error", err)
		return h.errorResponse(id, -32000, err.Error())
	}

	// Loop depth check
	if err := h.pushDepth(serverName); err != nil {
		slog.Warn("MCP sampling loop depth exceeded", "server", serverName, "error", err)
		return h.errorResponse(id, -32000, err.Error())
	}
	defer h.popDepth(serverName)

	// Parse request
	var req mcpSamplingRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return h.errorResponse(id, -32602, fmt.Sprintf("invalid sampling params: %v", err))
	}

	if len(req.Messages) == 0 {
		return h.errorResponse(id, -32602, "sampling request must include at least one message")
	}

	// Convert MCP messages to LLM messages
	messages := mcpMessagesToLLM(req.Messages, req.SystemPrompt)

	// Build LLM request
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	chatReq := llm.ChatRequest{
		Messages:  messages,
		MaxTokens: maxTokens,
		Stream:    false,
	}

	// Call LLM
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := h.llmClient.CreateChatCompletion(callCtx, chatReq)
	if err != nil {
		slog.Error("MCP sampling LLM call failed", "server", serverName, "error", err)
		return h.errorResponse(id, -32603, fmt.Sprintf("LLM call failed: %v", err))
	}

	// Convert response back to MCP format
	result := llmResponseToMCP(resp, h.llmClient.Model())

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return h.errorResponse(id, -32603, fmt.Sprintf("marshal result: %v", err))
	}

	slog.Info("MCP sampling request completed", "server", serverName, "model", h.llmClient.Model())

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  resultJSON,
	}
}

// checkRateLimit enforces per-server RPM limits. Resets counts every minute.
func (h *SamplingHandler) checkRateLimit(serverName string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	if now.Sub(h.lastReset) >= time.Minute {
		h.callCounts = make(map[string]int)
		h.lastReset = now
	}

	if h.callCounts[serverName] >= h.maxRPM {
		return fmt.Errorf("sampling rate limit exceeded for server %q: %d/%d RPM", serverName, h.callCounts[serverName], h.maxRPM)
	}

	h.callCounts[serverName]++
	return nil
}

// pushDepth increments the nesting depth for a server and checks the limit.
func (h *SamplingHandler) pushDepth(serverName string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	depth := h.depths[serverName]
	if depth >= h.maxLoopDepth {
		return fmt.Errorf("sampling loop depth exceeded for server %q: %d/%d", serverName, depth, h.maxLoopDepth)
	}

	h.depths[serverName] = depth + 1
	return nil
}

// popDepth decrements the nesting depth for a server.
func (h *SamplingHandler) popDepth(serverName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.depths[serverName] > 0 {
		h.depths[serverName]--
	}
}

// errorResponse builds a JSON-RPC error response.
func (h *SamplingHandler) errorResponse(id int64, code int, message string) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonRPCError{
			Code:    code,
			Message: message,
		},
	}
}

// mcpMessagesToLLM converts MCP sampling messages to LLM messages.
func mcpMessagesToLLM(msgs []mcpSamplingMessage, systemPrompt string) []llm.Message {
	var result []llm.Message

	if systemPrompt != "" {
		result = append(result, llm.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for _, m := range msgs {
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}

		result = append(result, llm.Message{
			Role:    role,
			Content: m.Content.Text,
		})
	}

	return result
}

// llmResponseToMCP converts an LLM response to an MCP sampling result.
func llmResponseToMCP(resp *llm.ChatResponse, model string) mcpSamplingResult {
	return mcpSamplingResult{
		Role: "assistant",
		Content: mcpSamplingContent{
			Type: "text",
			Text: resp.Content,
		},
		Model: model,
	}
}
