package platforms

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// WebhookAdapter implements the gateway.PlatformAdapter interface for generic webhooks.
// It runs an HTTP server that receives POST webhooks and emits message events.
type WebhookAdapter struct {
	BasePlatformAdapter
	port      string
	secret    string
	server    *http.Server
	cancel    context.CancelFunc
	responses map[string]chan string // pending response channels keyed by request ID
	mu        sync.Mutex
}

// NewWebhookAdapter creates a new Webhook adapter.
func NewWebhookAdapter(port, secret string) *WebhookAdapter {
	if port == "" {
		port = os.Getenv("WEBHOOK_PORT")
	}
	if port == "" {
		port = "8088"
	}
	if secret == "" {
		secret = os.Getenv("WEBHOOK_SECRET")
	}
	return &WebhookAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformWebhook),
		port:                port,
		secret:              secret,
		responses:           make(map[string]chan string),
	}
}

// Connect starts the webhook HTTP server.
func (w *WebhookAdapter) Connect(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", w.handleWebhook)
	mux.HandleFunc("/health", w.handleHealth)

	w.server = &http.Server{
		Addr:    ":" + w.port,
		Handler: mux,
	}

	connCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	w.connected = true
	slog.Info("Webhook adapter started", "port", w.port)

	go func() {
		<-connCtx.Done()
		w.server.Close()
	}()

	go func() {
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Webhook server error", "error", err)
			w.connected = false
		}
	}()

	return nil
}

// Disconnect stops the webhook HTTP server.
func (w *WebhookAdapter) Disconnect() error {
	if w.cancel != nil {
		w.cancel()
	}
	if w.server != nil {
		w.server.Close()
	}
	w.connected = false
	return nil
}

// Send stores a response for a pending webhook request.
func (w *WebhookAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	w.mu.Lock()
	ch, ok := w.responses[chatID]
	w.mu.Unlock()

	if ok {
		select {
		case ch <- text:
		default:
		}
	}

	return &gateway.SendResult{Success: true}, nil
}

// SendTyping is a no-op for webhooks.
func (w *WebhookAdapter) SendTyping(ctx context.Context, chatID string) error {
	return nil
}

// SendImage is not supported for webhooks; sends as text.
func (w *WebhookAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, caption, metadata)
}

// SendVoice is not supported for webhooks; sends as text.
func (w *WebhookAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, "[voice]", metadata)
}

// SendDocument is not supported for webhooks; sends as text.
func (w *WebhookAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, "[document]", metadata)
}

// --- Internal ---

// webhookPayload is the expected JSON body of an incoming webhook.
type webhookPayload struct {
	RequestID string            `json:"request_id"`
	Text      string            `json:"text"`
	ChatID    string            `json:"chat_id"`
	UserID    string            `json:"user_id"`
	UserName  string            `json:"user_name"`
	ChatType  string            `json:"chat_type"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func (w *WebhookAdapter) handleWebhook(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(rw, "bad request", http.StatusBadRequest)
		return
	}

	// Verify signature if secret is set.
	if w.secret != "" {
		sig := r.Header.Get("X-Webhook-Signature")
		if !w.verifySignature(body, sig) {
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(rw, "bad request: invalid JSON", http.StatusBadRequest)
		return
	}

	if payload.Text == "" {
		http.Error(rw, "bad request: text required", http.StatusBadRequest)
		return
	}

	chatID := payload.ChatID
	if chatID == "" {
		chatID = payload.RequestID
	}
	if chatID == "" {
		chatID = "webhook"
	}

	chatType := payload.ChatType
	if chatType == "" {
		chatType = "dm"
	}

	source := gateway.SessionSource{
		Platform: gateway.PlatformWebhook,
		ChatID:   chatID,
		ChatType: chatType,
		UserID:   payload.UserID,
		UserName: payload.UserName,
	}

	event := &gateway.MessageEvent{
		Text:        payload.Text,
		MessageType: gateway.MessageTypeText,
		Source:      source,
		Metadata:    payload.Metadata,
		RawMessage:  payload,
	}

	w.EmitMessage(event)

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(map[string]any{
		"status":     "accepted",
		"request_id": payload.RequestID,
	})
}

func (w *WebhookAdapter) handleHealth(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]any{
		"status":    "ok",
		"platform":  "webhook",
		"connected": w.connected,
	})
}

func (w *WebhookAdapter) verifySignature(body []byte, signature string) bool {
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(w.secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// Ensure WebhookAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*WebhookAdapter)(nil)
