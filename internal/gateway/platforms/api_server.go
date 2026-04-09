package platforms

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// APIServerAdapter implements the gateway.PlatformAdapter interface as an
// OpenAI-compatible API server. It exposes a /v1/chat/completions endpoint
// that accepts OpenAI-format requests and routes them to the agent.
type APIServerAdapter struct {
	BasePlatformAdapter
	port      string
	apiKey    string
	server    *http.Server
	cancel    context.CancelFunc
	responses map[string]chan string // pending response channels keyed by request ID
	mu        sync.Mutex
}

// NewAPIServerAdapter creates a new OpenAI-compatible API server adapter.
func NewAPIServerAdapter(port, apiKey string) *APIServerAdapter {
	if port == "" {
		port = os.Getenv("API_SERVER_PORT")
	}
	if port == "" {
		port = "8080"
	}
	if apiKey == "" {
		apiKey = os.Getenv("API_SERVER_KEY")
	}
	return &APIServerAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformAPIServer),
		port:                port,
		apiKey:              apiKey,
		responses:           make(map[string]chan string),
	}
}

// Connect starts the API server.
func (a *APIServerAdapter) Connect(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", a.handleChatCompletions)
	mux.HandleFunc("/v1/models", a.handleModels)
	mux.HandleFunc("/health", a.handleHealth)

	a.server = &http.Server{
		Addr:    ":" + a.port,
		Handler: mux,
	}

	connCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	a.connected = true
	slog.Info("API server adapter started", "port", a.port)

	go func() {
		<-connCtx.Done()
		a.server.Close()
	}()

	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("API server error", "error", err)
			a.connected = false
		}
	}()

	return nil
}

// Disconnect stops the API server.
func (a *APIServerAdapter) Disconnect() error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.server != nil {
		a.server.Close()
	}
	a.connected = false
	return nil
}

// Send delivers a response to a pending API request.
func (a *APIServerAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	a.mu.Lock()
	ch, ok := a.responses[chatID]
	a.mu.Unlock()

	if ok {
		select {
		case ch <- text:
		default:
		}
	}

	return &gateway.SendResult{Success: true}, nil
}

// SendTyping is a no-op for the API server.
func (a *APIServerAdapter) SendTyping(ctx context.Context, chatID string) error {
	return nil
}

// SendImage is not applicable for API server; sends as text.
func (a *APIServerAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	return a.Send(ctx, chatID, caption, metadata)
}

// SendVoice is not applicable for API server; sends as text.
func (a *APIServerAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return a.Send(ctx, chatID, "[voice]", metadata)
}

// SendDocument is not applicable for API server; sends as text.
func (a *APIServerAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return a.Send(ctx, chatID, "[document]", metadata)
}

// --- OpenAI-compatible types ---

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
	User        string              `json:"user,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int               `json:"index"`
		Message      openAIChatMessage `json:"message"`
		FinishReason string            `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// --- Internal ---

func (a *APIServerAdapter) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify API key if set.
	if a.apiKey != "" {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != a.apiKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Invalid API key",
					"type":    "invalid_request_error",
				},
			})
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var req openAIChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "Invalid JSON: " + err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if len(req.Messages) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "messages array is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Extract the last user message.
	var lastUserMessage string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUserMessage = req.Messages[i].Content
			break
		}
	}
	if lastUserMessage == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "no user message found",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Create a unique request ID.
	requestID := uuid.New().String()

	// Set up response channel.
	responseCh := make(chan string, 1)
	a.mu.Lock()
	a.responses[requestID] = responseCh
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		delete(a.responses, requestID)
		a.mu.Unlock()
	}()

	userID := req.User
	if userID == "" {
		userID = "api-user"
	}

	source := gateway.SessionSource{
		Platform: gateway.PlatformAPIServer,
		ChatID:   requestID,
		ChatType: "dm",
		UserID:   userID,
	}

	event := &gateway.MessageEvent{
		Text:        lastUserMessage,
		MessageType: gateway.MessageTypeText,
		Source:      source,
		RawMessage:  req,
	}

	// Emit the message and wait for the response.
	a.EmitMessage(event)

	// Wait for response with timeout.
	timeout := 120 * time.Second
	select {
	case responseText := <-responseCh:
		resp := openAIChatResponse{
			ID:      "chatcmpl-" + requestID[:8],
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []struct {
				Index        int               `json:"index"`
				Message      openAIChatMessage `json:"message"`
				FinishReason string            `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: openAIChatMessage{
						Role:    "assistant",
						Content: responseText,
					},
					FinishReason: "stop",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

	case <-time.After(timeout):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGatewayTimeout)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "Request timed out",
				"type":    "timeout_error",
			},
		})

	case <-r.Context().Done():
		// Client disconnected.
		return
	}
}

func (a *APIServerAdapter) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":       "hermes-agent",
				"object":   "model",
				"created":  time.Now().Unix(),
				"owned_by": "hermes-agent",
			},
		},
	})
}

func (a *APIServerAdapter) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "ok",
		"platform":  "apiserver",
		"connected": a.connected,
	})
}

// Ensure APIServerAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*APIServerAdapter)(nil)

