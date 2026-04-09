package platforms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// FeishuAdapter implements the gateway.PlatformAdapter interface for Feishu/Lark.
// It uses the Feishu Open API for sending and event subscription for receiving.
type FeishuAdapter struct {
	BasePlatformAdapter
	appID            string
	appSecret        string
	verificationToken string
	encryptKey       string
	tenantAccessToken string
	tokenExpiry      time.Time
	httpClient       *http.Client
	cancel           context.CancelFunc
	mu               sync.RWMutex
	webhookPort      string
}

const feishuAPIBase = "https://open.feishu.cn/open-apis"

// NewFeishuAdapter creates a new Feishu/Lark adapter.
func NewFeishuAdapter(appID, appSecret string) *FeishuAdapter {
	if appID == "" {
		appID = os.Getenv("FEISHU_APP_ID")
	}
	if appSecret == "" {
		appSecret = os.Getenv("FEISHU_APP_SECRET")
	}
	return &FeishuAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformFeishu),
		appID:               appID,
		appSecret:           appSecret,
		verificationToken:   os.Getenv("FEISHU_VERIFICATION_TOKEN"),
		encryptKey:          os.Getenv("FEISHU_ENCRYPT_KEY"),
		webhookPort:         envOrDefault("FEISHU_WEBHOOK_PORT", "9091"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect establishes a connection to Feishu.
func (f *FeishuAdapter) Connect(ctx context.Context) error {
	if f.appID == "" {
		return fmt.Errorf("FEISHU_APP_ID not set")
	}
	if f.appSecret == "" {
		return fmt.Errorf("FEISHU_APP_SECRET not set")
	}

	// Get initial tenant access token.
	if err := f.refreshToken(ctx); err != nil {
		return fmt.Errorf("Feishu auth failed: %w", err)
	}

	f.connected = true
	slog.Info("Feishu adapter connected", "app_id", f.appID)

	connCtx, cancel := context.WithCancel(ctx)
	f.cancel = cancel

	// Start token refresh loop.
	go f.tokenRefreshLoop(connCtx)

	// Start event subscription webhook server.
	go f.startEventServer(connCtx)

	return nil
}

// Disconnect cleanly disconnects from Feishu.
func (f *FeishuAdapter) Disconnect() error {
	if f.cancel != nil {
		f.cancel()
	}
	f.connected = false
	return nil
}

// Send sends a text message via Feishu.
func (f *FeishuAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	token, err := f.getToken()
	if err != nil {
		return nil, err
	}

	content, _ := json.Marshal(map[string]string{"text": text})

	payload := map[string]any{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(content),
	}

	// Determine receive_id_type from metadata or default to chat_id.
	receiveIDType := "chat_id"
	if t := metadata["receive_id_type"]; t != "" {
		receiveIDType = t
	}

	url := fmt.Sprintf("%s/im/v1/messages?receive_id_type=%s", feishuAPIBase, receiveIDType)

	return f.feishuPost(ctx, url, token, payload)
}

// SendTyping is a no-op for Feishu (not supported).
func (f *FeishuAdapter) SendTyping(ctx context.Context, chatID string) error {
	return nil
}

// SendImage sends an image via Feishu.
func (f *FeishuAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	// Simplified: send as text.
	text := caption
	if text == "" {
		text = "[Image]"
	}
	return f.Send(ctx, chatID, text, metadata)
}

// SendVoice sends a voice message via Feishu.
func (f *FeishuAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return f.Send(ctx, chatID, "[Voice message]", metadata)
}

// SendDocument sends a document via Feishu.
func (f *FeishuAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return f.Send(ctx, chatID, "[Document] "+filePath, metadata)
}

// --- Internal ---

func (f *FeishuAdapter) refreshToken(ctx context.Context) error {
	payload := map[string]string{
		"app_id":     f.appID,
		"app_secret": f.appSecret,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		feishuAPIBase+"/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf("Feishu token error: %s", result.Msg)
	}

	f.mu.Lock()
	f.tenantAccessToken = result.TenantAccessToken
	f.tokenExpiry = time.Now().Add(time.Duration(result.Expire-60) * time.Second)
	f.mu.Unlock()

	return nil
}

func (f *FeishuAdapter) getToken() (string, error) {
	f.mu.RLock()
	token := f.tenantAccessToken
	expired := time.Now().After(f.tokenExpiry)
	f.mu.RUnlock()

	if expired {
		if err := f.refreshToken(context.Background()); err != nil {
			return "", err
		}
		f.mu.RLock()
		token = f.tenantAccessToken
		f.mu.RUnlock()
	}
	return token, nil
}

func (f *FeishuAdapter) tokenRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(90 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := f.refreshToken(ctx); err != nil {
				slog.Error("Feishu token refresh failed", "error", err)
			}
		}
	}
}

func (f *FeishuAdapter) feishuPost(ctx context.Context, url, token string, payload map[string]any) (*gateway.SendResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		respBody, _ := io.ReadAll(resp.Body)
		return &gateway.SendResult{
			Success: false,
			Error:   fmt.Sprintf("decode response: %v, body: %s", err, string(respBody)),
		}, nil
	}

	if result.Code != 0 {
		return &gateway.SendResult{
			Success:   false,
			Error:     result.Msg,
			Retryable: true,
		}, nil
	}

	return &gateway.SendResult{
		Success:   true,
		MessageID: result.Data.MessageID,
	}, nil
}

// feishuEventCallback represents an incoming Feishu event callback.
type feishuEventCallback struct {
	Schema string `json:"schema"`
	// URL verification challenge.
	Challenge string `json:"challenge"`
	Token     string `json:"token"`
	Type      string `json:"type"`
	// Event data.
	Header *struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
	} `json:"header"`
	Event *struct {
		Sender struct {
			SenderID struct {
				OpenID  string `json:"open_id"`
				UserID  string `json:"user_id"`
				UnionID string `json:"union_id"`
			} `json:"sender_id"`
			SenderType string `json:"sender_type"`
		} `json:"sender"`
		Message struct {
			MessageID   string `json:"message_id"`
			ChatID      string `json:"chat_id"`
			ChatType    string `json:"chat_type"` // "p2p" or "group"
			MessageType string `json:"message_type"`
			Content     string `json:"content"`
		} `json:"message"`
	} `json:"event"`
}

func (f *FeishuAdapter) startEventServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/feishu/event", f.handleEvent)

	server := &http.Server{
		Addr:    ":" + f.webhookPort,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	slog.Info("Feishu event server starting", "port", f.webhookPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Feishu event server error", "error", err)
	}
}

func (f *FeishuAdapter) handleEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var callback feishuEventCallback
	if err := json.NewDecoder(r.Body).Decode(&callback); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Handle URL verification challenge.
	if callback.Type == "url_verification" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"challenge": callback.Challenge})
		return
	}

	// Process message events.
	if callback.Header != nil && callback.Header.EventType == "im.message.receive_v1" && callback.Event != nil {
		chatType := "dm"
		if callback.Event.Message.ChatType == "group" {
			chatType = "group"
		}

		// Parse message content.
		var content struct {
			Text string `json:"text"`
		}
		json.Unmarshal([]byte(callback.Event.Message.Content), &content)

		source := gateway.SessionSource{
			Platform: gateway.PlatformFeishu,
			ChatID:   callback.Event.Message.ChatID,
			ChatType: chatType,
			UserID:   callback.Event.Sender.SenderID.OpenID,
		}

		msgType := gateway.MessageTypeText
		switch callback.Event.Message.MessageType {
		case "image":
			msgType = gateway.MessageTypePhoto
		case "audio":
			msgType = gateway.MessageTypeVoice
		case "file":
			msgType = gateway.MessageTypeDocument
		}

		event := &gateway.MessageEvent{
			Text:        content.Text,
			MessageType: msgType,
			Source:      source,
			RawMessage:  callback,
		}

		f.EmitMessage(event)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Ensure FeishuAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*FeishuAdapter)(nil)
