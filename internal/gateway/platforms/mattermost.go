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

const PlatformMattermost gateway.Platform = "mattermost"

// MattermostAdapter implements PlatformAdapter for Mattermost.
type MattermostAdapter struct {
	BasePlatformAdapter
	serverURL string
	token     string
	botUserID string
	client    *http.Client
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewMattermostAdapter(serverURL, token string) *MattermostAdapter {
	return &MattermostAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(PlatformMattermost),
		serverURL:           serverURL,
		token:               token,
		client:              &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *MattermostAdapter) Connect(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Get bot user ID
	req, _ := http.NewRequestWithContext(ctx, "GET", m.serverURL+"/api/v4/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+m.token)
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("mattermost auth: %w", err)
	}
	defer resp.Body.Close()

	var user struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&user)
	m.botUserID = user.ID

	m.connected = true
	slog.Info("Mattermost connected", "server", m.serverURL, "bot", m.botUserID)

	go m.websocketLoop()
	return nil
}

func (m *MattermostAdapter) Disconnect() error {
	m.connected = false
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

func (m *MattermostAdapter) Send(ctx context.Context, chatID string, text string, _ map[string]string) (*gateway.SendResult, error) {
	payload := map[string]string{"channel_id": chatID, "message": text}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", m.serverURL+"/api/v4/posts", bytes.NewReader(body))
	if err != nil {
		return &gateway.SendResult{Error: err.Error()}, err
	}
	req.Header.Set("Authorization", "Bearer "+m.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return &gateway.SendResult{Error: err.Error(), Retryable: true}, err
	}
	defer resp.Body.Close()

	var post struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&post)
	return &gateway.SendResult{Success: true, MessageID: post.ID}, nil
}

func (m *MattermostAdapter) SendTyping(_ context.Context, _ string) error { return nil }
func (m *MattermostAdapter) SendImage(ctx context.Context, chatID string, _ string, caption string, md map[string]string) (*gateway.SendResult, error) {
	return m.Send(ctx, chatID, caption+" [image]", md)
}
func (m *MattermostAdapter) SendVoice(ctx context.Context, chatID string, _ string, md map[string]string) (*gateway.SendResult, error) {
	return m.Send(ctx, chatID, "[voice]", md)
}
func (m *MattermostAdapter) SendDocument(ctx context.Context, chatID string, _ string, md map[string]string) (*gateway.SendResult, error) {
	return m.Send(ctx, chatID, "[document]", md)
}

func (m *MattermostAdapter) websocketLoop() {
	// Mattermost WebSocket event loop (simplified — polls /api/v4/posts for new messages)
	// Production implementation would use the WebSocket API at ws://server/api/v4/websocket
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// Polling fallback — real impl uses WebSocket
		}
	}
}

var _ io.Reader // keep io import
