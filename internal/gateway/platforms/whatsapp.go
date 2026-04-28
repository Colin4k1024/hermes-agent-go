package platforms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

const PlatformWhatsApp gateway.Platform = "whatsapp"

// WhatsAppAdapter implements PlatformAdapter for WhatsApp Cloud API.
type WhatsAppAdapter struct {
	BasePlatformAdapter
	token       string
	phoneNumID  string
	webhookPort int
	verifyToken string
	server      *http.Server
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewWhatsAppAdapter(token, phoneNumID, verifyToken string, webhookPort int) *WhatsAppAdapter {
	if webhookPort == 0 {
		webhookPort = 8079
	}
	return &WhatsAppAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(PlatformWhatsApp),
		token:               token,
		phoneNumID:          phoneNumID,
		verifyToken:         verifyToken,
		webhookPort:         webhookPort,
	}
}

func (w *WhatsAppAdapter) Connect(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)
	mux := http.NewServeMux()
	mux.HandleFunc("/whatsapp/webhook", w.handleWebhook)

	w.server = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", w.webhookPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	w.connected = true
	slog.Info("WhatsApp adapter starting", "port", w.webhookPort)

	go func() {
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("WhatsApp server error", "error", err)
		}
	}()
	return nil
}

func (w *WhatsAppAdapter) Disconnect() error {
	w.connected = false
	if w.cancel != nil {
		w.cancel()
	}
	if w.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return w.server.Shutdown(ctx)
	}
	return nil
}

func (w *WhatsAppAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                chatID,
		"type":              "text",
		"text":              map[string]string{"body": text},
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", w.phoneNumID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return &gateway.SendResult{Error: err.Error()}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &gateway.SendResult{Error: err.Error(), Retryable: true}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return &gateway.SendResult{Error: string(b)}, fmt.Errorf("whatsapp error %d", resp.StatusCode)
	}
	return &gateway.SendResult{Success: true}, nil
}

func (w *WhatsAppAdapter) SendTyping(_ context.Context, _ string) error { return nil }
func (w *WhatsAppAdapter) SendImage(ctx context.Context, chatID string, _ string, caption string, md map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, caption+" [image]", md)
}
func (w *WhatsAppAdapter) SendVoice(ctx context.Context, chatID string, _ string, md map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, "[voice]", md)
}
func (w *WhatsAppAdapter) SendDocument(ctx context.Context, chatID string, _ string, md map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, "[document]", md)
}

func (w *WhatsAppAdapter) handleWebhook(rw http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		mode := r.URL.Query().Get("hub.mode")
		token := r.URL.Query().Get("hub.verify_token")
		challenge := r.URL.Query().Get("hub.challenge")
		if mode == "subscribe" && token == w.verifyToken {
			fmt.Fprint(rw, challenge)
			return
		}
		http.Error(rw, "forbidden", http.StatusForbidden)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var payload struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						From string                `json:"from"`
						Text struct{ Body string } `json:"text"`
						Type string                `json:"type"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		rw.WriteHeader(http.StatusOK)
		return
	}

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				if msg.Type != "text" {
					continue
				}
				event := &gateway.MessageEvent{
					Text:        msg.Text.Body,
					MessageType: gateway.MessageTypeText,
					Source: gateway.SessionSource{
						Platform: PlatformWhatsApp,
						ChatID:   msg.From,
						ChatType: "dm",
						UserID:   msg.From,
					},
				}
				w.EmitMessage(event)
			}
		}
	}
	rw.WriteHeader(http.StatusOK)
}
