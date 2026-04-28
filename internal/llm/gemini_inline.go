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

// geminiHTTPTransport is a minimal inline Gemini transport to avoid circular imports.
type geminiHTTPTransport struct {
	apiKey string
	model  string
	client *http.Client
}

func (t *geminiHTTPTransport) Name() string { return "gemini" }

func (t *geminiHTTPTransport) ensureClient() *http.Client {
	if t.client == nil {
		t.client = &http.Client{Timeout: 300 * time.Second}
	}
	return t.client
}

func (t *geminiHTTPTransport) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	gemReq := t.buildRequest(req)
	body, _ := json.Marshal(gemReq)

	model := stripGeminiPrefix(t.model)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, t.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.ensureClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(b))
	}

	var gemResp geminiInlineResponse
	if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
		return nil, fmt.Errorf("gemini decode: %w", err)
	}

	return t.parseResponse(&gemResp)
}

func (t *geminiHTTPTransport) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamDelta, <-chan error) {
	deltaCh := make(chan StreamDelta, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(deltaCh)
		defer close(errCh)

		gemReq := t.buildRequest(req)
		body, _ := json.Marshal(gemReq)

		model := stripGeminiPrefix(t.model)
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", model, t.apiKey)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := t.ensureClient().Do(httpReq)
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
		var toolCalls []ToolCall
		tcIdx := 0

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			var gemResp geminiInlineResponse
			if err := json.Unmarshal([]byte(data), &gemResp); err != nil {
				continue
			}
			if len(gemResp.Candidates) == 0 {
				continue
			}

			for _, part := range gemResp.Candidates[0].Content.Parts {
				if part.Text != "" {
					deltaCh <- StreamDelta{Content: part.Text}
				}
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					toolCalls = append(toolCalls, ToolCall{
						ID:   fmt.Sprintf("call_%d", tcIdx),
						Type: "function",
						Function: FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsJSON),
						},
					})
					tcIdx++
				}
			}
		}

		deltaCh <- StreamDelta{Done: true, ToolCalls: toolCalls}
	}()

	return deltaCh, errCh
}

// --- Gemini API types (inline) ---

type geminiInlineRequest struct {
	Contents          []geminiInlineContent  `json:"contents"`
	Tools             []geminiInlineToolDecl `json:"tools,omitempty"`
	SystemInstruction *geminiInlineContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiInlineGenCfg    `json:"generationConfig,omitempty"`
}

type geminiInlineContent struct {
	Role  string             `json:"role,omitempty"`
	Parts []geminiInlinePart `json:"parts"`
}

type geminiInlinePart struct {
	Text             string                `json:"text,omitempty"`
	FunctionCall     *geminiInlineFuncCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiInlineFuncResp `json:"functionResponse,omitempty"`
}

type geminiInlineFuncCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiInlineFuncResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiInlineToolDecl struct {
	FunctionDeclarations []geminiInlineFuncDecl `json:"functionDeclarations"`
}

type geminiInlineFuncDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiInlineGenCfg struct {
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	Temperature     *float32 `json:"temperature,omitempty"`
}

type geminiInlineResponse struct {
	Candidates    []geminiInlineCandidate `json:"candidates"`
	UsageMetadata *geminiInlineUsage      `json:"usageMetadata,omitempty"`
}

type geminiInlineCandidate struct {
	Content      geminiInlineContent `json:"content"`
	FinishReason string              `json:"finishReason"`
}

type geminiInlineUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func (t *geminiHTTPTransport) buildRequest(req ChatRequest) geminiInlineRequest {
	gemReq := geminiInlineRequest{}

	if len(req.Tools) > 0 {
		var decls []geminiInlineFuncDecl
		for _, td := range req.Tools {
			decls = append(decls, geminiInlineFuncDecl{
				Name: td.Name, Description: td.Description, Parameters: td.Parameters,
			})
		}
		gemReq.Tools = []geminiInlineToolDecl{{FunctionDeclarations: decls}}
	}

	for _, m := range req.Messages {
		if m.Role == "system" {
			gemReq.SystemInstruction = &geminiInlineContent{
				Parts: []geminiInlinePart{{Text: m.Content}},
			}
			continue
		}

		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}

		content := geminiInlineContent{Role: role}

		if m.Role == "tool" {
			var respData map[string]any
			json.Unmarshal([]byte(m.Content), &respData)
			if respData == nil {
				respData = map[string]any{"result": m.Content}
			}
			content.Parts = append(content.Parts, geminiInlinePart{
				FunctionResponse: &geminiInlineFuncResp{Name: m.ToolName, Response: respData},
			})
		} else if m.Content != "" {
			content.Parts = append(content.Parts, geminiInlinePart{Text: m.Content})
		}

		for _, tc := range m.ToolCalls {
			var args map[string]any
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
			content.Parts = append(content.Parts, geminiInlinePart{
				FunctionCall: &geminiInlineFuncCall{Name: tc.Function.Name, Args: args},
			})
		}

		if len(content.Parts) > 0 {
			gemReq.Contents = append(gemReq.Contents, content)
		}
	}

	if req.MaxTokens > 0 || req.Temperature != nil {
		gemReq.GenerationConfig = &geminiInlineGenCfg{}
		if req.MaxTokens > 0 {
			mt := req.MaxTokens
			gemReq.GenerationConfig.MaxOutputTokens = &mt
		}
		gemReq.GenerationConfig.Temperature = req.Temperature
	}

	return gemReq
}

func (t *geminiHTTPTransport) parseResponse(resp *geminiInlineResponse) (*ChatResponse, error) {
	result := &ChatResponse{FinishReason: "stop"}

	if resp.UsageMetadata != nil {
		result.Usage = Usage{
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
	}

	tcIdx := 0
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			result.Content += part.Text
		}
		if part.FunctionCall != nil {
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID: fmt.Sprintf("call_%d", tcIdx), Type: "function",
				Function: FunctionCall{Name: part.FunctionCall.Name, Arguments: string(argsJSON)},
			})
			tcIdx++
			result.FinishReason = "tool_calls"
		}
	}

	return result, nil
}

func stripGeminiPrefix(model string) string {
	for _, prefix := range []string{"google/", "gemini/"} {
		model = strings.TrimPrefix(model, prefix)
	}
	return model
}
