package transports

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

// CodexTransport implements llm.Transport for the OpenAI Responses API (/v1/responses).
type CodexTransport struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewCodexTransport creates a transport for the OpenAI Responses API.
func NewCodexTransport(model, baseURL, apiKey string) *CodexTransport {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &CodexTransport{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (t *CodexTransport) Name() string { return "codex" }

// --- Responses API types ---

type responsesRequest struct {
	Model           string           `json:"model"`
	Input           []responsesInput `json:"input"`
	Tools           []responsesTool  `json:"tools,omitempty"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	Temperature     *float32         `json:"temperature,omitempty"`
	Stream          bool             `json:"stream,omitempty"`
}

type responsesInput struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Type    string `json:"type,omitempty"`
	// For function_call / function_call_output
	CallID string `json:"call_id,omitempty"`
	Name   string `json:"name,omitempty"`
	Output string `json:"output,omitempty"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type responsesResponse struct {
	ID     string          `json:"id"`
	Output []responsesItem `json:"output"`
	Usage  *responsesUsage `json:"usage,omitempty"`
	Status string          `json:"status"`
}

type responsesItem struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Status    string `json:"status,omitempty"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type responsesStreamEvent struct {
	Type     string             `json:"type"`
	Item     *responsesItem     `json:"item,omitempty"`
	Delta    string             `json:"delta,omitempty"`
	Response *responsesResponse `json:"response,omitempty"`
}

func (t *CodexTransport) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	apiReq := t.buildRequest(req)
	apiReq.Stream = false
	body, _ := json.Marshal(apiReq)

	url := t.baseURL + "/v1/responses"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	t.setHeaders(httpReq)

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("codex request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("codex API error %d: %s", resp.StatusCode, string(b))
	}

	var apiResp responsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("codex response decode: %w", err)
	}

	return t.parseResponse(&apiResp), nil
}

func (t *CodexTransport) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamDelta, <-chan error) {
	deltaCh := make(chan llm.StreamDelta, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(deltaCh)
		defer close(errCh)

		apiReq := t.buildRequest(req)
		apiReq.Stream = true
		body, _ := json.Marshal(apiReq)

		url := t.baseURL + "/v1/responses"
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}
		t.setHeaders(httpReq)

		resp, err := t.client.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("codex stream failed: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("codex API error %d: %s", resp.StatusCode, string(b))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		var toolCalls []llm.ToolCall

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" || data == "" {
				break
			}

			var event responsesStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "response.output_text.delta":
				if event.Delta != "" {
					deltaCh <- llm.StreamDelta{Content: event.Delta}
				}
			case "response.function_call_arguments.delta":
				if event.Delta != "" && len(toolCalls) > 0 {
					last := &toolCalls[len(toolCalls)-1]
					last.Function.Arguments += event.Delta
				}
			case "response.output_item.added":
				if event.Item != nil && event.Item.Type == "function_call" {
					toolCalls = append(toolCalls, llm.ToolCall{
						ID:   event.Item.CallID,
						Type: "function",
						Function: llm.FunctionCall{
							Name: event.Item.Name,
						},
					})
				}
			case "response.completed":
				// Final event
			}
		}

		deltaCh <- llm.StreamDelta{Done: true, ToolCalls: toolCalls}
	}()

	return deltaCh, errCh
}

func (t *CodexTransport) buildRequest(req llm.ChatRequest) responsesRequest {
	apiReq := responsesRequest{
		Model: t.model,
	}

	// Convert messages to Responses API input format
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			apiReq.Input = append(apiReq.Input, responsesInput{
				Role:    "system",
				Content: m.Content,
			})
		case "user":
			apiReq.Input = append(apiReq.Input, responsesInput{
				Role:    "user",
				Content: m.Content,
			})
		case "assistant":
			if m.Content != "" {
				apiReq.Input = append(apiReq.Input, responsesInput{
					Role:    "assistant",
					Content: m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				apiReq.Input = append(apiReq.Input, responsesInput{
					Type:   "function_call",
					CallID: tc.ID,
					Name:   tc.Function.Name,
					Output: tc.Function.Arguments,
				})
			}
		case "tool":
			apiReq.Input = append(apiReq.Input, responsesInput{
				Type:   "function_call_output",
				CallID: m.ToolCallID,
				Output: m.Content,
			})
		}
	}

	// Convert tools
	for _, td := range req.Tools {
		apiReq.Tools = append(apiReq.Tools, responsesTool{
			Type:        "function",
			Name:        td.Name,
			Description: td.Description,
			Parameters:  td.Parameters,
		})
	}

	if req.MaxTokens > 0 {
		apiReq.MaxOutputTokens = req.MaxTokens
	}
	apiReq.Temperature = req.Temperature

	return apiReq
}

func (t *CodexTransport) parseResponse(resp *responsesResponse) *llm.ChatResponse {
	result := &llm.ChatResponse{FinishReason: "stop"}

	if resp.Usage != nil {
		result.Usage = llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			result.Content += item.Text
		case "function_call":
			result.ToolCalls = append(result.ToolCalls, llm.ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
			result.FinishReason = "tool_calls"
		}
	}

	return result
}

func (t *CodexTransport) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
}

var _ llm.Transport = (*CodexTransport)(nil)

func init() {
	slog.Debug("Codex transport registered")
}
