package platforms

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// WhatsAppAdapter implements the gateway.PlatformAdapter interface for WhatsApp
// via a local WhatsApp Web bridge (e.g. Baileys-based HTTP bridge).
type WhatsAppAdapter struct {
	BasePlatformAdapter
	bridgeURL  string
	httpClient *http.Client
	cancel     context.CancelFunc
	mu         sync.Mutex
}

// NewWhatsAppAdapter creates a new WhatsApp adapter.
func NewWhatsAppAdapter(bridgeURL string) *WhatsAppAdapter {
	if bridgeURL == "" {
		bridgeURL = os.Getenv("WHATSAPP_BRIDGE_URL")
	}
	if bridgeURL == "" {
		bridgeURL = "http://localhost:3000"
	}
	return &WhatsAppAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformWhatsApp),
		bridgeURL:           bridgeURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect establishes a connection to the WhatsApp bridge.
func (w *WhatsAppAdapter) Connect(ctx context.Context) error {
	// Verify bridge is reachable.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.bridgeURL+"/status", nil)
	if err != nil {
		return fmt.Errorf("create status request: %w", err)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("WhatsApp bridge not reachable at %s: %w", w.bridgeURL, err)
	}
	resp.Body.Close()

	w.connected = true
	slog.Info("WhatsApp adapter connected", "bridge_url", w.bridgeURL)

	// Start polling for incoming messages.
	pollCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	go w.pollMessages(pollCtx)

	return nil
}

// Disconnect cleanly disconnects from the WhatsApp bridge.
func (w *WhatsAppAdapter) Disconnect() error {
	if w.cancel != nil {
		w.cancel()
	}
	w.connected = false
	return nil
}

// Send sends a text message via the WhatsApp bridge.
func (w *WhatsAppAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	payload := map[string]any{
		"chatId":  chatID,
		"message": text,
	}

	if replyTo := metadata["reply_to"]; replyTo != "" {
		payload["quotedMessageId"] = replyTo
	}

	return w.bridgePost(ctx, "/send/text", payload)
}

// SendTyping sends a typing indicator via the WhatsApp bridge.
func (w *WhatsAppAdapter) SendTyping(ctx context.Context, chatID string) error {
	payload := map[string]any{
		"chatId": chatID,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.bridgeURL+"/send/typing", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// SendImage sends an image via the WhatsApp bridge.
func (w *WhatsAppAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	b64, err := fileToBase64(imagePath)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	payload := map[string]any{
		"chatId":   chatID,
		"file":     b64,
		"filename": filepath.Base(imagePath),
		"caption":  caption,
		"type":     "image",
	}

	return w.bridgePost(ctx, "/send/media", payload)
}

// SendVoice sends a voice message via the WhatsApp bridge.
func (w *WhatsAppAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	b64, err := fileToBase64(audioPath)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	payload := map[string]any{
		"chatId":   chatID,
		"file":     b64,
		"filename": filepath.Base(audioPath),
		"type":     "audio",
		"ptt":      true, // Push-to-talk / voice note
	}

	return w.bridgePost(ctx, "/send/media", payload)
}

// SendDocument sends a document via the WhatsApp bridge.
func (w *WhatsAppAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	b64, err := fileToBase64(filePath)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	payload := map[string]any{
		"chatId":   chatID,
		"file":     b64,
		"filename": filepath.Base(filePath),
		"type":     "document",
	}

	return w.bridgePost(ctx, "/send/media", payload)
}

// --- Internal ---

func (w *WhatsAppAdapter) bridgePost(ctx context.Context, path string, payload map[string]any) (*gateway.SendResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.bridgeURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}
	defer resp.Body.Close()

	var result struct {
		Success   bool   `json:"success"`
		MessageID string `json:"messageId"`
		Error     string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     fmt.Sprintf("decode bridge response: %v", err),
			Retryable: true,
		}, nil
	}

	if !result.Success {
		return &gateway.SendResult{
			Success:   false,
			Error:     result.Error,
			Retryable: true,
		}, nil
	}

	return &gateway.SendResult{
		Success:   true,
		MessageID: result.MessageID,
	}, nil
}

// whatsAppBridgeMessage is the message format from the WhatsApp bridge.
type whatsAppBridgeMessage struct {
	ChatID    string `json:"chatId"`
	Sender    string `json:"sender"`
	SenderName string `json:"senderName"`
	Message   string `json:"message"`
	MessageID string `json:"messageId"`
	IsGroup   bool   `json:"isGroup"`
	GroupName string `json:"groupName"`
	Timestamp int64  `json:"timestamp"`
	HasMedia  bool   `json:"hasMedia"`
	MediaURL  string `json:"mediaUrl"`
	MediaType string `json:"mediaType"`
	QuotedID  string `json:"quotedMessageId"`
}

func (w *WhatsAppAdapter) pollMessages(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.fetchMessages(ctx)
		}
	}
}

func (w *WhatsAppAdapter) fetchMessages(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.bridgeURL+"/messages", nil)
	if err != nil {
		return
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var messages []whatsAppBridgeMessage
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return
	}

	for _, msg := range messages {
		chatType := "dm"
		chatName := msg.SenderName
		if msg.IsGroup {
			chatType = "group"
			chatName = msg.GroupName
		}

		source := gateway.SessionSource{
			Platform: gateway.PlatformWhatsApp,
			ChatID:   msg.ChatID,
			ChatName: chatName,
			ChatType: chatType,
			UserID:   msg.Sender,
			UserName: msg.SenderName,
		}

		event := &gateway.MessageEvent{
			Text:        msg.Message,
			MessageType: gateway.MessageTypeText,
			Source:      source,
			RawMessage:  msg,
		}

		if msg.QuotedID != "" {
			event.ReplyToID = msg.QuotedID
		}

		if msg.HasMedia {
			switch msg.MediaType {
			case "image":
				event.MessageType = gateway.MessageTypePhoto
			case "video":
				event.MessageType = gateway.MessageTypeVideo
			case "audio", "voice", "ptt":
				event.MessageType = gateway.MessageTypeVoice
			case "document":
				event.MessageType = gateway.MessageTypeDocument
			}
			if msg.MediaURL != "" {
				event.MediaURLs = append(event.MediaURLs, msg.MediaURL)
			}
		}

		w.EmitMessage(event)
	}
}

func fileToBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", path, err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// Ensure WhatsAppAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*WhatsAppAdapter)(nil)

