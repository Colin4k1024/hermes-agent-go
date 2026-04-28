package platforms

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// DingTalkAdapter implements PlatformAdapter for DingTalk.
type DingTalkAdapter struct {
	BasePlatformAdapter
	token       string
	secret      string
	webhookPort int
	server      *http.Server
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewDingTalkAdapter(token, secret string, webhookPort int) *DingTalkAdapter {
	if webhookPort == 0 {
		webhookPort = 8075
	}
	return &DingTalkAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformDingTalk),
		token:               token,
		secret:              secret,
		webhookPort:         webhookPort,
	}
}

func (d *DingTalkAdapter) Connect(ctx context.Context) error {
	d.ctx, d.cancel = context.WithCancel(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/dingtalk/callback", d.handleCallback)

	d.server = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", d.webhookPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	d.connected = true
	slog.Info("DingTalk adapter starting", "port", d.webhookPort)

	go func() {
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("DingTalk server error", "error", err)
		}
	}()

	return nil
}

func (d *DingTalkAdapter) Disconnect() error {
	d.connected = false
	if d.cancel != nil {
		d.cancel()
	}
	if d.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return d.server.Shutdown(ctx)
	}
	return nil
}

func (d *DingTalkAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	payload := map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": text},
	}
	return d.postWebhook(ctx, chatID, payload)
}

func (d *DingTalkAdapter) SendTyping(_ context.Context, _ string) error { return nil }

func (d *DingTalkAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	return d.Send(ctx, chatID, caption+" [image: "+imagePath+"]", metadata)
}

func (d *DingTalkAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return d.Send(ctx, chatID, "[voice: "+audioPath+"]", metadata)
}

func (d *DingTalkAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return d.Send(ctx, chatID, "[file: "+filePath+"]", metadata)
}

func (d *DingTalkAdapter) postWebhook(ctx context.Context, webhookURL string, payload map[string]any) (*gateway.SendResult, error) {
	body, _ := json.Marshal(payload)
	url := webhookURL
	if d.secret != "" {
		ts := fmt.Sprintf("%d", time.Now().UnixMilli())
		sign := d.computeSign(ts)
		url += fmt.Sprintf("&timestamp=%s&sign=%s", ts, sign)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return &gateway.SendResult{Error: err.Error()}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &gateway.SendResult{Error: err.Error(), Retryable: true}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return &gateway.SendResult{Error: string(b)}, fmt.Errorf("dingtalk API error %d", resp.StatusCode)
	}

	return &gateway.SendResult{Success: true}, nil
}

func (d *DingTalkAdapter) computeSign(timestamp string) string {
	stringToSign := timestamp + "\n" + d.secret
	mac := hmac.New(sha256.New, []byte(d.secret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

type dingtalkCallbackPayload struct {
	MsgType           string                        `json:"msgtype"`
	Text              struct{ Content string }      `json:"text"`
	SenderID          string                        `json:"senderId"`
	SenderNick        string                        `json:"senderNick"`
	ConversationID    string                        `json:"conversationId"`
	ConversationType  string                        `json:"conversationType"` // "1"=dm, "2"=group
	AtUsers           []struct{ DingtalkID string } `json:"atUsers"`
	ConversationTitle string                        `json:"conversationTitle"`
}

func (d *DingTalkAdapter) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	var payload dingtalkCallbackPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	chatType := "dm"
	if payload.ConversationType == "2" {
		chatType = "group"
	}

	event := &gateway.MessageEvent{
		Text:        payload.Text.Content,
		MessageType: gateway.MessageTypeText,
		Source: gateway.SessionSource{
			Platform: gateway.PlatformDingTalk,
			ChatID:   payload.ConversationID,
			ChatName: payload.ConversationTitle,
			ChatType: chatType,
			UserID:   payload.SenderID,
			UserName: payload.SenderNick,
		},
	}

	d.EmitMessage(event)
	w.WriteHeader(http.StatusOK)
}
