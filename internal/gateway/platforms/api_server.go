package platforms

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

// APIServerAdapter implements PlatformAdapter as an OpenAI-compatible API server.
type APIServerAdapter struct {
	BasePlatformAdapter
	port      int
	authKey   string
	server    *http.Server
	ctx       context.Context
	cancel    context.CancelFunc
	pendingMu sync.Mutex
	pending   map[string]chan string // sessionID → response channel
}

func NewAPIServerAdapter(port int, authKey string) *APIServerAdapter {
	return &APIServerAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformAPI),
		port:                port,
		authKey:             authKey,
		pending:             make(map[string]chan string),
	}
}

func (a *APIServerAdapter) Connect(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", a.handleChatCompletions)
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})

	var handler http.Handler = mux
	handler = gatewayCORS(handler)

	a.server = &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", a.port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	a.connected = true
	slog.Info("API server starting", "port", a.port)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("API server error", "error", err)
		}
	}()

	return nil
}

func (a *APIServerAdapter) Disconnect() error {
	a.connected = false
	if a.cancel != nil {
		a.cancel()
	}
	if a.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.server.Shutdown(ctx)
	}
	return nil
}

func (a *APIServerAdapter) Send(_ context.Context, chatID string, text string, _ map[string]string) (*gateway.SendResult, error) {
	a.pendingMu.Lock()
	ch, ok := a.pending[chatID]
	a.pendingMu.Unlock()

	if ok {
		select {
		case ch <- text:
			return &gateway.SendResult{Success: true}, nil
		default:
			return &gateway.SendResult{Error: "response channel full"}, nil
		}
	}
	return &gateway.SendResult{Error: "no pending request for session"}, nil
}

func (a *APIServerAdapter) SendTyping(_ context.Context, _ string) error { return nil }

func (a *APIServerAdapter) SendImage(_ context.Context, chatID string, _ string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	return a.Send(context.Background(), chatID, caption, metadata)
}

func (a *APIServerAdapter) SendVoice(_ context.Context, chatID string, _ string, metadata map[string]string) (*gateway.SendResult, error) {
	return a.Send(context.Background(), chatID, "[voice message]", metadata)
}

func (a *APIServerAdapter) SendDocument(_ context.Context, chatID string, _ string, metadata map[string]string) (*gateway.SendResult, error) {
	return a.Send(context.Background(), chatID, "[document]", metadata)
}

func gatewayCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Hermes-Session-Id, X-Hermes-Tenant-Id")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- OpenAI-compatible chat completions endpoint ---

type chatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []chatCompletionMessage `json:"messages"`
	Stream   bool                    `json:"stream"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
}

type chatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      chatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

func (a *APIServerAdapter) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if a.authKey != "" {
		expected := []byte("Bearer " + a.authKey)
		actual := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(expected, actual) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body error", http.StatusBadRequest)
		return
	}

	var req chatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Extract system prompt, conversation history, and last user message.
	var systemParts []string
	var history []llm.Message
	var lastUserMsg string
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			if msg.Content != "" {
				systemParts = append(systemParts, msg.Content)
			}
		case "user", "assistant":
			if msg.Role == "user" {
				lastUserMsg = msg.Content
			}
			history = append(history, llm.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}
	// Remove the last user message from history — it will be passed as event.Text
	// and appended by RunConversation.
	if len(history) > 0 && history[len(history)-1].Role == "user" {
		history = history[:len(history)-1]
	}

	sessionID := r.Header.Get("X-Hermes-Session-Id")
	if sessionID == "" {
		sessionID = fmt.Sprintf("api_%d", time.Now().UnixNano())
	}

	tenantID := r.Header.Get("X-Hermes-Tenant-Id")
	if tenantID == "" {
		tenantID = "default"
	}

	// Create response channel
	responseCh := make(chan string, 1)
	a.pendingMu.Lock()
	a.pending[sessionID] = responseCh
	a.pendingMu.Unlock()

	defer func() {
		a.pendingMu.Lock()
		delete(a.pending, sessionID)
		a.pendingMu.Unlock()
	}()

	// Emit as gateway message
	event := &gateway.MessageEvent{
		Text:         lastUserMsg,
		SystemPrompt: strings.Join(systemParts, "\n\n"),
		MessageType:  gateway.MessageTypeText,
		Source: gateway.SessionSource{
			Platform: gateway.PlatformAPI,
			ChatID:   sessionID,
			ChatType: "dm",
			UserID:   "api",
			UserName: "api",
			TenantID: tenantID,
		},
		History: history,
		Metadata: map[string]string{
			"tenant_id": tenantID,
		},
	}
	a.EmitMessage(event)

	// Wait for response
	select {
	case response := <-responseCh:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:      sessionID,
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []chatCompletionChoice{{
				Index:        0,
				Message:      chatCompletionMessage{Role: "assistant", Content: response},
				FinishReason: "stop",
			}},
		})
	case <-time.After(300 * time.Second):
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	case <-r.Context().Done():
		return
	}
}
