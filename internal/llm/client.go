package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"github.com/hermes-agent/hermes-agent-go/internal/observability"
	openai "github.com/sashabaranov/go-openai"
)

type llmTenantKey struct{}

// WithTenantID stores a tenant ID in context for LLM observability.
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, llmTenantKey{}, id)
}

func tenantIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(llmTenantKey{}).(string)
	if v == "" {
		return "local"
	}
	return v
}

// APIMode determines which API protocol to use.
type APIMode string

const (
	APIModeOpenAI    APIMode = "openai"
	APIModeAnthropic APIMode = "anthropic"
	APIModeGemini    APIMode = "gemini"
	APIModeBedrock   APIMode = "bedrock"
	APIModeCodex     APIMode = "codex"
)

// Client wraps an LLM API client using a pluggable Transport.
type Client struct {
	transport Transport
	apiMode   APIMode
	model     string
	provider  string
	baseURL   string
	apiKey    string
}

// NewClient creates a new LLM client from configuration.
func NewClient(cfg *config.Config) (*Client, error) {
	provider, baseURL, apiKey := ResolveProvider(cfg)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured. Run 'hermes setup' or set an API key in %s/.env", config.DisplayHermesHome())
	}

	model := cfg.Model
	if model == "" {
		model = "anthropic/claude-sonnet-4-20250514"
	}

	apiMode := detectAPIMode(cfg.APIMode, provider, baseURL)
	return newClientInternal(model, baseURL, apiKey, provider, apiMode)
}

// NewClientWithParams creates a client with explicit parameters.
func NewClientWithParams(model, baseURL, apiKey, provider string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	apiMode := detectAPIMode("", provider, baseURL)
	return newClientInternal(model, baseURL, apiKey, provider, apiMode)
}

// NewClientWithMode creates a client with explicit API mode.
func NewClientWithMode(model, baseURL, apiKey, provider string, mode APIMode) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	return newClientInternal(model, baseURL, apiKey, provider, mode)
}

// NewClientWithTransport creates a client with a specific transport implementation.
func NewClientWithTransport(model, baseURL, apiKey, provider string, t Transport) *Client {
	return &Client{
		transport: t,
		model:     model,
		provider:  provider,
		baseURL:   baseURL,
		apiKey:    apiKey,
		apiMode:   APIModeOpenAI,
	}
}

func newClientInternal(model, baseURL, apiKey, provider string, mode APIMode) (*Client, error) {
	c := &Client{
		model:    model,
		provider: provider,
		baseURL:  baseURL,
		apiKey:   apiKey,
		apiMode:  mode,
	}

	t := newDefaultTransport(provider, baseURL, apiKey, model, mode)
	if os.Getenv("HERMES_CIRCUIT_BREAKER_DISABLED") != "true" {
		t = NewResilientTransport(t, model)
	}
	c.transport = t
	slog.Info("Using LLM transport", "transport", c.transport.Name(), "model", model, "baseURL", baseURL)
	return c, nil
}

func detectAPIMode(explicit, provider, baseURL string) APIMode {
	switch strings.ToLower(explicit) {
	case "anthropic", "anthropic_messages":
		return APIModeAnthropic
	case "gemini":
		return APIModeGemini
	case "bedrock":
		return APIModeBedrock
	case "codex", "responses":
		return APIModeCodex
	case "openai", "chat_completions", "":
		// fall through to auto-detect
	}

	switch provider {
	case "anthropic":
		return APIModeAnthropic
	case "gemini":
		return APIModeGemini
	case "bedrock":
		return APIModeBedrock
	}

	lower := strings.ToLower(baseURL)
	if strings.Contains(lower, "anthropic.com") {
		return APIModeAnthropic
	}
	if strings.Contains(lower, "generativelanguage.googleapis.com") {
		return APIModeGemini
	}

	return APIModeOpenAI
}

// Model returns the model name.
func (c *Client) Model() string { return c.model }

// Provider returns the provider name.
func (c *Client) Provider() string { return c.provider }

// BaseURL returns the API base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// APIMode returns the API mode.
func (c *Client) APIMode() APIMode { return c.apiMode }

// GetTransport returns the underlying transport.
func (c *Client) GetTransport() Transport { return c.transport }

// ChatRequest represents a chat completion request.
type ChatRequest struct {
	Messages       []Message
	Tools          []ToolDef
	MaxTokens      int
	Temperature    *float32
	Stream         bool
	ReasoningLevel string // "xhigh", "high", "medium", "low", "minimal", ""
}

// Message represents a chat message in OpenAI format.
type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	ToolName         string     `json:"tool_name,omitempty"`
	FinishReason     string     `json:"finish_reason,omitempty"`
	Reasoning        string     `json:"reasoning,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ImageURLs        []string   `json:"image_urls,omitempty"`
}

// ToolCall represents a tool call from the assistant.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents the function details in a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatResponse represents a chat completion response.
type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Reasoning    string
	Usage        Usage
}

// Usage tracks token usage.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CacheReadTokens  int
	CacheWriteTokens int
	ReasoningTokens  int
}

// StreamDelta represents a streaming chunk.
type StreamDelta struct {
	Content   string
	ToolCalls []ToolCall
	Reasoning string
	Done      bool
}

// CreateChatCompletion sends a non-streaming chat completion request.
func (c *Client) CreateChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	start := time.Now()
	resp, err := c.transport.Chat(ctx, req)
	elapsed := time.Since(start)

	tenantID := tenantIDFromContext(ctx)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	attrs := []any{
		"model", c.model,
		"tenant_id", tenantID,
		"latency_ms", elapsed.Milliseconds(),
		"error", errMsg,
	}
	if resp != nil {
		attrs = append(attrs,
			"input_tokens", resp.Usage.PromptTokens,
			"output_tokens", resp.Usage.CompletionTokens,
			"cache_read_tokens", resp.Usage.CacheReadTokens,
		)
		llmTokensTotal.WithLabelValues(c.model, tenantID, "input").Add(float64(resp.Usage.PromptTokens))
		llmTokensTotal.WithLabelValues(c.model, tenantID, "output").Add(float64(resp.Usage.CompletionTokens))
	}
	llmRequestDuration.WithLabelValues(c.model, tenantID).Observe(elapsed.Seconds())
	observability.ContextLogger(ctx).Info("llm_call", attrs...)

	return resp, err
}

// CreateChatCompletionStream sends a streaming chat completion request.
// Wraps the underlying transport stream with timing and token metrics.
func (c *Client) CreateChatCompletionStream(ctx context.Context, req ChatRequest) (<-chan StreamDelta, <-chan error) {
	start := time.Now()
	tenantID := tenantIDFromContext(ctx)
	deltaCh, errCh := c.transport.ChatStream(ctx, req)

	if deltaCh == nil && errCh == nil {
		ch := make(chan StreamDelta)
		close(ch)
		ech := make(chan error)
		close(ech)
		return ch, ech
	}

	bufSize := 0
	if deltaCh != nil {
		bufSize = cap(deltaCh)
	}
	wrappedDelta := make(chan StreamDelta, bufSize)
	wrappedErr := make(chan error, 1)

	go func() {
		defer close(wrappedDelta)
		defer close(wrappedErr)
		var lastErr error
		if deltaCh != nil {
			for d := range deltaCh {
				wrappedDelta <- d
			}
		}
		if errCh != nil {
			for e := range errCh {
				lastErr = e
				wrappedErr <- e
			}
		}
		elapsed := time.Since(start)
		llmRequestDuration.WithLabelValues(c.model, tenantID).Observe(elapsed.Seconds())

		errMsg := ""
		if lastErr != nil {
			errMsg = lastErr.Error()
		}
		observability.ContextLogger(ctx).Info("llm_call_stream",
			"model", c.model,
			"tenant_id", tenantID,
			"latency_ms", elapsed.Milliseconds(),
			"error", errMsg,
		)
	}()

	return wrappedDelta, wrappedErr
}

// --- Internal transport implementations (kept in this file to avoid circular imports) ---

func newDefaultTransport(provider, baseURL, apiKey, model string, mode APIMode) Transport {
	switch mode {
	case APIModeAnthropic:
		return &anthropicTransportImpl{client: NewAnthropicClient(model, baseURL, apiKey, provider)}
	case APIModeGemini:
		return &geminiTransportImpl{apiKey: apiKey, model: model}
	case APIModeBedrock:
		return &bedrockTransportImpl{model: model}
	case APIModeCodex:
		return &codexTransportImpl{apiKey: apiKey, model: model, baseURL: baseURL}
	default:
		cfg := openai.DefaultConfig(apiKey)
		cfg.BaseURL = baseURL
		return &openaiTransportImpl{
			client: openai.NewClientWithConfig(cfg),
			model:  model,
		}
	}
}

// geminiTransportImpl is a lazy-init wrapper for Gemini.
type geminiTransportImpl struct {
	apiKey string
	model  string
}

func (g *geminiTransportImpl) Name() string { return "gemini" }
func (g *geminiTransportImpl) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Lazy import to avoid pulling in HTTP client at init time
	t := g.transport()
	return t.Chat(ctx, req)
}
func (g *geminiTransportImpl) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamDelta, <-chan error) {
	return g.transport().ChatStream(ctx, req)
}
func (g *geminiTransportImpl) transport() Transport {
	// Uses the transports sub-package indirectly via a simple HTTP-based impl
	return &geminiHTTPTransport{apiKey: g.apiKey, model: g.model}
}

// bedrockTransportImpl is a lazy-init wrapper for Bedrock.
type bedrockTransportImpl struct{ model string }

func (b *bedrockTransportImpl) Name() string { return "bedrock" }
func (b *bedrockTransportImpl) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("bedrock transport requires initialization via transports.NewBedrockTransport()")
}
func (b *bedrockTransportImpl) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamDelta, <-chan error) {
	errCh := make(chan error, 1)
	errCh <- fmt.Errorf("bedrock transport requires initialization via transports.NewBedrockTransport()")
	close(errCh)
	return nil, errCh
}

// codexTransportImpl wraps the Codex/Responses API.
type codexTransportImpl struct {
	apiKey  string
	model   string
	baseURL string
}

func (c *codexTransportImpl) Name() string { return "codex" }
func (c *codexTransportImpl) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return c.transport().Chat(ctx, req)
}
func (c *codexTransportImpl) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamDelta, <-chan error) {
	return c.transport().ChatStream(ctx, req)
}
func (c *codexTransportImpl) transport() Transport {
	return &codexHTTPTransport{apiKey: c.apiKey, model: c.model, baseURL: c.baseURL}
}

// anthropicTransportImpl adapts AnthropicClient to Transport.
type anthropicTransportImpl struct{ client *AnthropicClient }

func (a *anthropicTransportImpl) Name() string { return "anthropic" }
func (a *anthropicTransportImpl) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return a.client.CreateChatCompletion(ctx, req)
}
func (a *anthropicTransportImpl) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamDelta, <-chan error) {
	return a.client.CreateChatCompletionStream(ctx, req)
}

// openaiTransportImpl adapts the go-openai SDK to Transport.
type openaiTransportImpl struct {
	client *openai.Client
	model  string
}

func (o *openaiTransportImpl) Name() string { return "openai" }

func (o *openaiTransportImpl) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	apiReq := BuildOpenAIRequest(o.model, req)
	apiReq.Stream = false

	resp, err := o.client.CreateChatCompletion(ctx, apiReq)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return &ChatResponse{FinishReason: "stop"}, nil
	}

	choice := resp.Choices[0]
	result := &ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:   tc.ID,
			Type: string(tc.Type),
			Function: FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	return result, nil
}

func (o *openaiTransportImpl) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamDelta, <-chan error) {
	deltaCh := make(chan StreamDelta, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(deltaCh)
		defer close(errCh)

		apiReq := BuildOpenAIRequest(o.model, req)
		apiReq.Stream = true

		stream, err := o.client.CreateChatCompletionStream(ctx, apiReq)
		if err != nil {
			errCh <- fmt.Errorf("stream creation failed: %w", err)
			return
		}
		defer stream.Close()

		toolCalls := make(map[int]*ToolCall)

		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				var finalCalls []ToolCall
				for _, tc := range toolCalls {
					finalCalls = append(finalCalls, *tc)
				}
				deltaCh <- StreamDelta{Done: true, ToolCalls: finalCalls}
				return
			}
			if err != nil {
				errCh <- err
				return
			}

			if len(resp.Choices) == 0 {
				continue
			}

			delta := resp.Choices[0].Delta

			if delta.Content != "" {
				deltaCh <- StreamDelta{Content: delta.Content}
			}

			for _, tc := range delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				existing, ok := toolCalls[idx]
				if !ok {
					existing = &ToolCall{
						ID:   tc.ID,
						Type: string(tc.Type),
						Function: FunctionCall{
							Name: tc.Function.Name,
						},
					}
					toolCalls[idx] = existing
				}
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				existing.Function.Arguments += tc.Function.Arguments
			}
		}
	}()

	return deltaCh, errCh
}

// BuildOpenAIRequest converts a ChatRequest to an openai.ChatCompletionRequest.
// Exported for use by the transports sub-package.
func BuildOpenAIRequest(model string, req ChatRequest) openai.ChatCompletionRequest {
	var msgs []openai.ChatCompletionMessage
	for _, m := range req.Messages {
		msg := openai.ChatCompletionMessage{Role: m.Role}

		if len(m.ImageURLs) > 0 {
			var parts []openai.ChatMessagePart
			if m.Content != "" {
				parts = append(parts, openai.ChatMessagePart{
					Type: openai.ChatMessagePartTypeText,
					Text: m.Content,
				})
			}
			for _, imgURL := range m.ImageURLs {
				parts = append(parts, openai.ChatMessagePart{
					Type: openai.ChatMessagePartTypeImageURL,
					ImageURL: &openai.ChatMessageImageURL{
						URL:    imgURL,
						Detail: openai.ImageURLDetailAuto,
					},
				})
			}
			msg.MultiContent = parts
		} else {
			msg.Content = m.Content
		}

		if m.ToolCallID != "" {
			msg.ToolCallID = m.ToolCallID
		}

		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolType(tc.Type),
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}

		msgs = append(msgs, msg)
	}

	var oaiTools []openai.Tool
	for _, td := range req.Tools {
		oaiTools = append(oaiTools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.ToRawParameters(),
			},
		})
	}

	apiReq := openai.ChatCompletionRequest{
		Model:    model,
		Messages: msgs,
		Tools:    oaiTools,
	}

	if req.MaxTokens > 0 {
		apiReq.MaxTokens = req.MaxTokens
	}
	if req.Temperature != nil {
		apiReq.Temperature = *req.Temperature
	}

	return apiReq
}

// ParseToolArgs parses a JSON string of tool arguments into a map.
func ParseToolArgs(argsJSON string) (map[string]any, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}
	return args, nil
}

func init() {
	if os.Getenv("HERMES_DEBUG") != "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}
}
