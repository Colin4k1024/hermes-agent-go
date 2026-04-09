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

// DingTalkAdapter implements the gateway.PlatformAdapter interface for DingTalk.
// It uses DingTalk's server-side API for sending and webhook/stream for receiving.
type DingTalkAdapter struct {
	BasePlatformAdapter
	appKey      string
	appSecret   string
	accessToken string
	tokenExpiry time.Time
	httpClient  *http.Client
	cancel      context.CancelFunc
	mu          sync.RWMutex
	webhookPort string
}

const dingtalkAPIBase = "https://oapi.dingtalk.com"

// NewDingTalkAdapter creates a new DingTalk adapter.
func NewDingTalkAdapter(appKey, appSecret string) *DingTalkAdapter {
	if appKey == "" {
		appKey = os.Getenv("DINGTALK_APP_KEY")
	}
	if appSecret == "" {
		appSecret = os.Getenv("DINGTALK_APP_SECRET")
	}
	return &DingTalkAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformDingTalk),
		appKey:              appKey,
		appSecret:           appSecret,
		webhookPort:         envOrDefault("DINGTALK_WEBHOOK_PORT", "9090"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect establishes a connection to DingTalk.
func (d *DingTalkAdapter) Connect(ctx context.Context) error {
	if d.appKey == "" {
		return fmt.Errorf("DINGTALK_APP_KEY not set")
	}
	if d.appSecret == "" {
		return fmt.Errorf("DINGTALK_APP_SECRET not set")
	}

	// Get initial access token.
	if err := d.refreshToken(ctx); err != nil {
		return fmt.Errorf("DingTalk auth failed: %w", err)
	}

	d.connected = true
	slog.Info("DingTalk adapter connected", "app_key", d.appKey)

	connCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	// Start token refresh loop.
	go d.tokenRefreshLoop(connCtx)

	// Start HTTP webhook server for receiving messages.
	go d.startWebhookServer(connCtx)

	return nil
}

// Disconnect cleanly disconnects from DingTalk.
func (d *DingTalkAdapter) Disconnect() error {
	if d.cancel != nil {
		d.cancel()
	}
	d.connected = false
	return nil
}

// Send sends a text message via DingTalk.
func (d *DingTalkAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	token, err := d.getToken()
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": text,
		},
	}

	// Check if sending to a conversation or a user.
	if userID := metadata["user_id"]; userID != "" {
		// Send as a private message to a user.
		payload["touser"] = userID
		payload["agentid"] = d.appKey
	}

	url := fmt.Sprintf("%s/chat/send?access_token=%s", dingtalkAPIBase, token)
	payload["chatid"] = chatID

	return d.doPost(ctx, url, payload)
}

// SendTyping is a no-op for DingTalk.
func (d *DingTalkAdapter) SendTyping(ctx context.Context, chatID string) error {
	return nil
}

// SendImage sends an image via DingTalk.
func (d *DingTalkAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	// DingTalk requires uploading media first, then referencing it.
	// Simplified: send as text with image path.
	text := caption
	if text == "" {
		text = "[Image]"
	}
	return d.Send(ctx, chatID, text+" "+imagePath, metadata)
}

// SendVoice sends a voice message via DingTalk.
func (d *DingTalkAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return d.Send(ctx, chatID, "[Voice] "+audioPath, metadata)
}

// SendDocument sends a document via DingTalk.
func (d *DingTalkAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return d.Send(ctx, chatID, "[Document] "+filePath, metadata)
}

// --- Internal ---

func (d *DingTalkAdapter) refreshToken(ctx context.Context) error {
	url := fmt.Sprintf("%s/gettoken?appkey=%s&appsecret=%s",
		dingtalkAPIBase, d.appKey, d.appSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return fmt.Errorf("DingTalk token error: %s", result.ErrMsg)
	}

	d.mu.Lock()
	d.accessToken = result.AccessToken
	d.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)
	d.mu.Unlock()

	return nil
}

func (d *DingTalkAdapter) getToken() (string, error) {
	d.mu.RLock()
	token := d.accessToken
	expired := time.Now().After(d.tokenExpiry)
	d.mu.RUnlock()

	if expired {
		if err := d.refreshToken(context.Background()); err != nil {
			return "", err
		}
		d.mu.RLock()
		token = d.accessToken
		d.mu.RUnlock()
	}
	return token, nil
}

func (d *DingTalkAdapter) tokenRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(90 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.refreshToken(ctx); err != nil {
				slog.Error("DingTalk token refresh failed", "error", err)
			}
		}
	}
}

func (d *DingTalkAdapter) doPost(ctx context.Context, url string, payload map[string]any) (*gateway.SendResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		respBody, _ := io.ReadAll(resp.Body)
		return &gateway.SendResult{
			Success: false,
			Error:   fmt.Sprintf("decode response: %v, body: %s", err, string(respBody)),
		}, nil
	}

	if result.ErrCode != 0 {
		return &gateway.SendResult{
			Success:   false,
			Error:     result.ErrMsg,
			Retryable: true,
		}, nil
	}

	return &gateway.SendResult{Success: true}, nil
}

// dingtalkCallbackMessage represents an incoming DingTalk webhook message.
type dingtalkCallbackMessage struct {
	MsgType         string `json:"msgtype"`
	Text            *struct {
		Content string `json:"content"`
	} `json:"text"`
	MsgID            string `json:"msgId"`
	ConversationID   string `json:"conversationId"`
	ConversationType string `json:"conversationType"` // "1" = private, "2" = group
	SenderID         string `json:"senderId"`
	SenderNick       string `json:"senderNick"`
	ChatbotUserID    string `json:"chatbotUserId"`
	ConversationTitle string `json:"conversationTitle"`
}

func (d *DingTalkAdapter) startWebhookServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dingtalk/callback", d.handleWebhook)

	server := &http.Server{
		Addr:    ":" + d.webhookPort,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	slog.Info("DingTalk webhook server starting", "port", d.webhookPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("DingTalk webhook server error", "error", err)
	}
}

func (d *DingTalkAdapter) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg dingtalkCallbackMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	chatType := "dm"
	if msg.ConversationType == "2" {
		chatType = "group"
	}

	text := ""
	if msg.Text != nil {
		text = msg.Text.Content
	}

	source := gateway.SessionSource{
		Platform: gateway.PlatformDingTalk,
		ChatID:   msg.ConversationID,
		ChatName: msg.ConversationTitle,
		ChatType: chatType,
		UserID:   msg.SenderID,
		UserName: msg.SenderNick,
	}

	event := &gateway.MessageEvent{
		Text:        text,
		MessageType: gateway.MessageTypeText,
		Source:      source,
		RawMessage:  msg,
	}

	d.EmitMessage(event)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Ensure DingTalkAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*DingTalkAdapter)(nil)
