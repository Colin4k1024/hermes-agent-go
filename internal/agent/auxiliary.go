package agent

import (
	"context"
	"log/slog"
	"os"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

// AuxiliaryClient provides secondary LLM clients for tasks like
// vision analysis, web content summarization, and context compression.
type AuxiliaryClient struct {
	visionClient     *llm.Client
	webExtractClient *llm.Client
	summaryClient    *llm.Client
}

// NewAuxiliaryClient creates auxiliary LLM clients from config.
func NewAuxiliaryClient(cfg *config.Config) *AuxiliaryClient {
	aux := &AuxiliaryClient{}

	// Vision client - for image analysis
	if model := os.Getenv("AUXILIARY_VISION_MODEL"); model != "" {
		key := os.Getenv("AUXILIARY_VISION_API_KEY")
		baseURL := os.Getenv("AUXILIARY_VISION_BASE_URL")
		if key == "" {
			key = os.Getenv("OPENROUTER_API_KEY")
		}
		if baseURL == "" {
			baseURL = llm.OpenRouterBaseURL
		}
		if key != "" {
			c, err := llm.NewClientWithParams(model, baseURL, key, "auxiliary-vision")
			if err == nil {
				aux.visionClient = c
				slog.Debug("Auxiliary vision client initialized", "model", model)
			}
		}
	}

	// Web extract client - for summarizing scraped content
	if model := os.Getenv("AUXILIARY_WEB_EXTRACT_MODEL"); model != "" {
		key := os.Getenv("AUXILIARY_WEB_EXTRACT_API_KEY")
		baseURL := os.Getenv("AUXILIARY_WEB_EXTRACT_BASE_URL")
		if key == "" {
			key = os.Getenv("OPENROUTER_API_KEY")
		}
		if baseURL == "" {
			baseURL = llm.OpenRouterBaseURL
		}
		if key != "" {
			c, err := llm.NewClientWithParams(model, baseURL, key, "auxiliary-web")
			if err == nil {
				aux.webExtractClient = c
			}
		}
	}

	return aux
}

// VisionClient returns the vision auxiliary client, or nil.
func (a *AuxiliaryClient) VisionClient() *llm.Client {
	return a.visionClient
}

// WebExtractClient returns the web extract auxiliary client, or nil.
func (a *AuxiliaryClient) WebExtractClient() *llm.Client {
	return a.webExtractClient
}

// Summarize uses the summary/compression auxiliary client to summarize text.
func (a *AuxiliaryClient) Summarize(ctx context.Context, text string, maxWords int) (string, error) {
	client := a.summaryClient
	if client == nil {
		client = a.webExtractClient
	}
	if client == nil {
		return text, nil // No auxiliary client, return original
	}

	prompt := "Summarize the following text concisely"
	if maxWords > 0 {
		prompt += " in under " + string(rune(maxWords)) + " words"
	}
	prompt += ":\n\n" + text

	resp, err := client.CreateChatCompletion(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 2000,
	})
	if err != nil {
		return text, err
	}

	return resp.Content, nil
}
