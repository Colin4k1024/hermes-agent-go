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

// GeminiTransport implements llm.Transport for Google Gemini (API key only).
type GeminiTransport struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewGeminiTransport creates a transport for Google AI Studio / Gemini.
func NewGeminiTransport(model, apiKey string) *GeminiTransport {
	geminiModel := model
	if strings.HasPrefix(geminiModel, "google/") {
		geminiModel = strings.TrimPrefix(geminiModel, "google/")
	}
	if strings.HasPrefix(geminiModel, "gemini/") {
		geminiModel = strings.TrimPrefix(geminiModel, "gemini/")
	}

	return &GeminiTransport{
		apiKey:  apiKey,
		model:   geminiModel,
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		client:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (t *GeminiTransport) Name() string { return "gemini" }

// --- Gemini API types ---

type geminiRequest struct {
	Contents          []geminiContent      `json:"contents"`
	Tools             []geminiToolDecl     `json:"tools,omitempty"`
	SystemInstruction *geminiContent       `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationCfg `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string              `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResp `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiFunctionResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiToolDecl struct {
	FunctionDeclarations []geminiFuncDecl `json:"functionDeclarations"`
}

type geminiFuncDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiGenerationCfg struct {
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	Temperature     *float32 `json:"temperature,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func (t *GeminiTransport) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	gemReq := t.buildRequest(req)
	body, _ := json.Marshal(gemReq)

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", t.baseURL, t.model, t.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(b))
	}

	var gemResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
		return nil, fmt.Errorf("gemini response decode: %w", err)
	}

	return t.parseResponse(&gemResp)
}

func (t *GeminiTransport) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamDelta, <-chan error) {
	deltaCh := make(chan llm.StreamDelta, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(deltaCh)
		defer close(errCh)

		gemReq := t.buildRequest(req)
		body, _ := json.Marshal(gemReq)

		url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", t.baseURL, t.model, t.apiKey)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := t.client.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("gemini stream failed: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(b))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		var toolCalls []llm.ToolCall
		tcIdx := 0

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "" {
				continue
			}

			var gemResp geminiResponse
			if err := json.Unmarshal([]byte(data), &gemResp); err != nil {
				continue
			}

			if len(gemResp.Candidates) == 0 {
				continue
			}

			candidate := gemResp.Candidates[0]
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					deltaCh <- llm.StreamDelta{Content: part.Text}
				}
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					toolCalls = append(toolCalls, llm.ToolCall{
						ID:   fmt.Sprintf("call_%d", tcIdx),
						Type: "function",
						Function: llm.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsJSON),
						},
					})
					tcIdx++
				}
			}
		}

		deltaCh <- llm.StreamDelta{Done: true, ToolCalls: toolCalls}
	}()

	return deltaCh, errCh
}

func (t *GeminiTransport) buildRequest(req llm.ChatRequest) geminiRequest {
	gemReq := geminiRequest{}

	// Convert tools
	if len(req.Tools) > 0 {
		var decls []geminiFuncDecl
		for _, td := range req.Tools {
			decls = append(decls, geminiFuncDecl{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Parameters,
			})
		}
		gemReq.Tools = []geminiToolDecl{{FunctionDeclarations: decls}}
	}

	// Convert messages
	for _, m := range req.Messages {
		if m.Role == "system" {
			gemReq.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: m.Content}},
			}
			continue
		}

		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}

		content := geminiContent{Role: role}

		if m.Role == "tool" {
			var respData map[string]any
			json.Unmarshal([]byte(m.Content), &respData)
			if respData == nil {
				respData = map[string]any{"result": m.Content}
			}
			content.Parts = append(content.Parts, geminiPart{
				FunctionResponse: &geminiFunctionResp{
					Name:     m.ToolName,
					Response: respData,
				},
			})
		} else if m.Content != "" {
			content.Parts = append(content.Parts, geminiPart{Text: m.Content})
		}

		// Convert tool calls to functionCall parts
		for _, tc := range m.ToolCalls {
			var args map[string]any
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
			content.Parts = append(content.Parts, geminiPart{
				FunctionCall: &geminiFunctionCall{
					Name: tc.Function.Name,
					Args: args,
				},
			})
		}

		if len(content.Parts) > 0 {
			gemReq.Contents = append(gemReq.Contents, content)
		}
	}

	// Generation config
	if req.MaxTokens > 0 || req.Temperature != nil {
		gemReq.GenerationConfig = &geminiGenerationCfg{}
		if req.MaxTokens > 0 {
			mt := req.MaxTokens
			gemReq.GenerationConfig.MaxOutputTokens = &mt
		}
		gemReq.GenerationConfig.Temperature = req.Temperature
	}

	return gemReq
}

func (t *GeminiTransport) parseResponse(resp *geminiResponse) (*llm.ChatResponse, error) {
	result := &llm.ChatResponse{FinishReason: "stop"}

	if resp.UsageMetadata != nil {
		result.Usage = llm.Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
	}

	if len(resp.Candidates) == 0 {
		return result, nil
	}

	candidate := resp.Candidates[0]

	switch candidate.FinishReason {
	case "STOP":
		result.FinishReason = "stop"
	case "MAX_TOKENS":
		result.FinishReason = "length"
	default:
		result.FinishReason = strings.ToLower(candidate.FinishReason)
	}

	tcIdx := 0
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			result.Content += part.Text
		}
		if part.FunctionCall != nil {
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)
			result.ToolCalls = append(result.ToolCalls, llm.ToolCall{
				ID:   fmt.Sprintf("call_%d", tcIdx),
				Type: "function",
				Function: llm.FunctionCall{
					Name:      part.FunctionCall.Name,
					Arguments: string(argsJSON),
				},
			})
			tcIdx++
			result.FinishReason = "tool_calls"
		}
	}

	return result, nil
}

var _ llm.Transport = (*GeminiTransport)(nil)

func init() {
	slog.Debug("Gemini transport registered")
}
