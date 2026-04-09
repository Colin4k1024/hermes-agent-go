package platforms

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// WeComAdapter implements the gateway.PlatformAdapter interface for WeCom (WeChat Work).
// It uses the WeCom server API for sending and callback API for receiving.
type WeComAdapter struct {
	BasePlatformAdapter
	corpID      string
	agentID     string
	secret      string
	callbackToken  string
	encodingAESKey string
	accessToken string
	tokenExpiry time.Time
	httpClient  *http.Client
	cancel      context.CancelFunc
	mu          sync.RWMutex
	webhookPort string
}

const wecomAPIBase = "https://qyapi.weixin.qq.com/cgi-bin"

// NewWeComAdapter creates a new WeCom adapter.
func NewWeComAdapter(corpID, agentID, secret string) *WeComAdapter {
	if corpID == "" {
		corpID = os.Getenv("WECOM_CORP_ID")
	}
	if agentID == "" {
		agentID = os.Getenv("WECOM_AGENT_ID")
	}
	if secret == "" {
		secret = os.Getenv("WECOM_SECRET")
	}
	return &WeComAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformWeCom),
		corpID:              corpID,
		agentID:             agentID,
		secret:              secret,
		callbackToken:       os.Getenv("WECOM_CALLBACK_TOKEN"),
		encodingAESKey:      os.Getenv("WECOM_ENCODING_AES_KEY"),
		webhookPort:         envOrDefault("WECOM_WEBHOOK_PORT", "9092"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect establishes a connection to WeCom.
func (w *WeComAdapter) Connect(ctx context.Context) error {
	if w.corpID == "" {
		return fmt.Errorf("WECOM_CORP_ID not set")
	}
	if w.agentID == "" {
		return fmt.Errorf("WECOM_AGENT_ID not set")
	}
	if w.secret == "" {
		return fmt.Errorf("WECOM_SECRET not set")
	}

	// Get initial access token.
	if err := w.refreshToken(ctx); err != nil {
		return fmt.Errorf("WeCom auth failed: %w", err)
	}

	w.connected = true
	slog.Info("WeCom adapter connected", "corp_id", w.corpID, "agent_id", w.agentID)

	connCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	// Start token refresh loop.
	go w.tokenRefreshLoop(connCtx)

	// Start callback server.
	go w.startCallbackServer(connCtx)

	return nil
}

// Disconnect cleanly disconnects from WeCom.
func (w *WeComAdapter) Disconnect() error {
	if w.cancel != nil {
		w.cancel()
	}
	w.connected = false
	return nil
}

// Send sends a text message via WeCom.
func (w *WeComAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	token, err := w.getToken()
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"touser":  chatID,
		"msgtype": "text",
		"agentid": w.agentID,
		"text": map[string]string{
			"content": text,
		},
	}

	return w.wecomPost(ctx, fmt.Sprintf("%s/message/send?access_token=%s", wecomAPIBase, token), payload)
}

// SendTyping is a no-op for WeCom.
func (w *WeComAdapter) SendTyping(ctx context.Context, chatID string) error {
	return nil
}

// SendImage sends an image via WeCom (simplified: sends as text).
func (w *WeComAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	text := caption
	if text == "" {
		text = "[Image]"
	}
	return w.Send(ctx, chatID, text, metadata)
}

// SendVoice sends a voice message via WeCom.
func (w *WeComAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, "[Voice message]", metadata)
}

// SendDocument sends a document via WeCom.
func (w *WeComAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return w.Send(ctx, chatID, "[Document] "+filePath, metadata)
}

// --- Internal ---

func (w *WeComAdapter) refreshToken(ctx context.Context) error {
	url := fmt.Sprintf("%s/gettoken?corpid=%s&corpsecret=%s",
		wecomAPIBase, w.corpID, w.secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := w.httpClient.Do(req)
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
		return fmt.Errorf("WeCom token error: %s", result.ErrMsg)
	}

	w.mu.Lock()
	w.accessToken = result.AccessToken
	w.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)
	w.mu.Unlock()

	return nil
}

func (w *WeComAdapter) getToken() (string, error) {
	w.mu.RLock()
	token := w.accessToken
	expired := time.Now().After(w.tokenExpiry)
	w.mu.RUnlock()

	if expired {
		if err := w.refreshToken(context.Background()); err != nil {
			return "", err
		}
		w.mu.RLock()
		token = w.accessToken
		w.mu.RUnlock()
	}
	return token, nil
}

func (w *WeComAdapter) tokenRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(90 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.refreshToken(ctx); err != nil {
				slog.Error("WeCom token refresh failed", "error", err)
			}
		}
	}
}

func (w *WeComAdapter) wecomPost(ctx context.Context, url string, payload map[string]any) (*gateway.SendResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		MsgID   string `json:"msgid"`
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

	return &gateway.SendResult{
		Success:   true,
		MessageID: result.MsgID,
	}, nil
}

// wecomXMLMessage represents an incoming WeCom callback message in XML format.
type wecomXMLMessage struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	MsgID        string   `xml:"MsgId"`
	AgentID      string   `xml:"AgentID"`
	PicURL       string   `xml:"PicUrl"`
	MediaID      string   `xml:"MediaId"`
}

func (w *WeComAdapter) startCallbackServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/wecom/callback", w.handleCallback)

	server := &http.Server{
		Addr:    ":" + w.webhookPort,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	slog.Info("WeCom callback server starting", "port", w.webhookPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("WeCom callback server error", "error", err)
	}
}

func (w *WeComAdapter) handleCallback(rw http.ResponseWriter, r *http.Request) {
	// Handle URL verification (GET request).
	if r.Method == http.MethodGet {
		echoStr := r.URL.Query().Get("echostr")
		// In production, verify the signature here.
		rw.Write([]byte(echoStr))
		return
	}

	if r.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(rw, "bad request", http.StatusBadRequest)
		return
	}

	// In production, decrypt the message body using encodingAESKey.
	// Simplified: parse XML directly.
	var msg wecomXMLMessage
	if err := xml.Unmarshal(body, &msg); err != nil {
		http.Error(rw, "bad request", http.StatusBadRequest)
		return
	}

	msgType := gateway.MessageTypeText
	switch msg.MsgType {
	case "image":
		msgType = gateway.MessageTypePhoto
	case "voice":
		msgType = gateway.MessageTypeVoice
	case "video":
		msgType = gateway.MessageTypeVideo
	case "file":
		msgType = gateway.MessageTypeDocument
	}

	source := gateway.SessionSource{
		Platform: gateway.PlatformWeCom,
		ChatID:   msg.FromUserName,
		ChatType: "dm",
		UserID:   msg.FromUserName,
	}

	event := &gateway.MessageEvent{
		Text:        msg.Content,
		MessageType: msgType,
		Source:      source,
		RawMessage:  msg,
	}

	if msg.PicURL != "" {
		event.MediaURLs = append(event.MediaURLs, msg.PicURL)
	}

	w.EmitMessage(event)

	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte("success"))
}

// Ensure WeComAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*WeComAdapter)(nil)
