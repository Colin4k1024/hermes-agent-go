package llm

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
)

// codexHTTPTransport is a minimal inline Codex/Responses API transport.
type codexHTTPTransport struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func (t *codexHTTPTransport) Name() string { return "codex" }

func (t *codexHTTPTransport) ensureClient() *http.Client {
	if t.client == nil {
		t.client = &http.Client{Timeout: 300 * time.Second}
	}
	return t.client
}

func (t *codexHTTPTransport) resolveBaseURL() string {
	base := t.baseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	base = strings.TrimSuffix(base, "/v1")
	return strings.TrimSuffix(base, "/")
}

// --- Codex API types ---

type codexRequest struct {
	Model           string         `json:"model"`
	Input           []codexInput   `json:"input"`
	Tools           []codexToolDef `json:"tools,omitempty"`
	MaxOutputTokens int            `json:"max_output_tokens,omitempty"`
	Temperature     *float32       `json:"temperature,omitempty"`
	Stream          bool           `json:"stream,omitempty"`
}

type codexInput struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Type    string `json:"type,omitempty"`
	CallID  string `json:"call_id,omitempty"`
	Name    string `json:"name,omitempty"`
	Output  string `json:"output,omitempty"`
}

type codexToolDef struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type codexResponse struct {
	Output []codexOutputItem `json:"output"`
	Usage  *codexUsage       `json:"usage,omitempty"`
}

type codexOutputItem struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type codexUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type codexStreamEvent struct {
	Type  string           `json:"type"`
	Item  *codexOutputItem `json:"item,omitempty"`
	Delta string           `json:"delta,omitempty"`
}

func (t *codexHTTPTransport) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	apiReq := t.buildRequest(req)
	body, _ := json.Marshal(apiReq)

	url := t.resolveBaseURL() + "/v1/responses"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.ensureClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("codex request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("codex API error %d: %s", resp.StatusCode, string(b))
	}

	var apiResp codexResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("codex decode: %w", err)
	}

	return t.parseResponse(&apiResp), nil
}

func (t *codexHTTPTransport) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamDelta, <-chan error) {
	deltaCh := make(chan StreamDelta, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(deltaCh)
		defer close(errCh)

		apiReq := t.buildRequest(req)
		apiReq.Stream = true
		body, _ := json.Marshal(apiReq)

		url := t.resolveBaseURL() + "/v1/responses"
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+t.apiKey)

		resp, err := t.ensureClient().Do(httpReq)
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
		var toolCalls []ToolCall

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" || data == "" {
				break
			}

			var event codexStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "response.output_text.delta":
				if event.Delta != "" {
					deltaCh <- StreamDelta{Content: event.Delta}
				}
			case "response.function_call_arguments.delta":
				if event.Delta != "" && len(toolCalls) > 0 {
					last := &toolCalls[len(toolCalls)-1]
					last.Function.Arguments += event.Delta
				}
			case "response.output_item.added":
				if event.Item != nil && event.Item.Type == "function_call" {
					toolCalls = append(toolCalls, ToolCall{
						ID:       event.Item.CallID,
						Type:     "function",
						Function: FunctionCall{Name: event.Item.Name},
					})
				}
			}
		}

		deltaCh <- StreamDelta{Done: true, ToolCalls: toolCalls}
	}()

	return deltaCh, errCh
}

func (t *codexHTTPTransport) buildRequest(req ChatRequest) codexRequest {
	apiReq := codexRequest{Model: t.model}

	for _, m := range req.Messages {
		switch m.Role {
		case "system", "user":
			apiReq.Input = append(apiReq.Input, codexInput{Role: m.Role, Content: m.Content})
		case "assistant":
			if m.Content != "" {
				apiReq.Input = append(apiReq.Input, codexInput{Role: "assistant", Content: m.Content})
			}
			for _, tc := range m.ToolCalls {
				apiReq.Input = append(apiReq.Input, codexInput{
					Type: "function_call", CallID: tc.ID, Name: tc.Function.Name, Output: tc.Function.Arguments,
				})
			}
		case "tool":
			apiReq.Input = append(apiReq.Input, codexInput{
				Type: "function_call_output", CallID: m.ToolCallID, Output: m.Content,
			})
		}
	}

	for _, td := range req.Tools {
		apiReq.Tools = append(apiReq.Tools, codexToolDef{
			Type: "function", Name: td.Name, Description: td.Description, Parameters: td.Parameters,
		})
	}

	if req.MaxTokens > 0 {
		apiReq.MaxOutputTokens = req.MaxTokens
	}
	apiReq.Temperature = req.Temperature

	return apiReq
}

func (t *codexHTTPTransport) parseResponse(resp *codexResponse) *ChatResponse {
	result := &ChatResponse{FinishReason: "stop"}

	if resp.Usage != nil {
		result.Usage = Usage{
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
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID: item.CallID, Type: "function",
				Function: FunctionCall{Name: item.Name, Arguments: item.Arguments},
			})
			result.FinishReason = "tool_calls"
		}
	}

	return result
}
