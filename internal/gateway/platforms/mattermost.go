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
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// MattermostAdapter implements the gateway.PlatformAdapter interface for Mattermost.
// It uses WebSocket for receiving events and HTTP API for sending messages.
type MattermostAdapter struct {
	BasePlatformAdapter
	serverURL  string
	token      string
	botUserID  string
	httpClient *http.Client
	wsConn     *websocket.Conn
	cancel     context.CancelFunc
}

// NewMattermostAdapter creates a new Mattermost adapter.
func NewMattermostAdapter(serverURL, token string) *MattermostAdapter {
	if serverURL == "" {
		serverURL = os.Getenv("MATTERMOST_URL")
	}
	if token == "" {
		token = os.Getenv("MATTERMOST_TOKEN")
	}
	serverURL = strings.TrimRight(serverURL, "/")

	return &MattermostAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformMattermost),
		serverURL:           serverURL,
		token:               token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect establishes a connection to Mattermost.
func (m *MattermostAdapter) Connect(ctx context.Context) error {
	if m.serverURL == "" {
		return fmt.Errorf("MATTERMOST_URL not set")
	}
	if m.token == "" {
		return fmt.Errorf("MATTERMOST_TOKEN not set")
	}

	// Get bot user info.
	userID, err := m.getMe(ctx)
	if err != nil {
		return fmt.Errorf("Mattermost auth failed: %w", err)
	}
	m.botUserID = userID

	m.connected = true
	slog.Info("Mattermost adapter connected", "server", m.serverURL, "user_id", m.botUserID)

	// Connect WebSocket for events.
	wsCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	go m.connectWebSocket(wsCtx)

	return nil
}

// Disconnect cleanly disconnects from Mattermost.
func (m *MattermostAdapter) Disconnect() error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.wsConn != nil {
		m.wsConn.Close()
	}
	m.connected = false
	return nil
}

// Send sends a text message to a Mattermost channel.
func (m *MattermostAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	payload := map[string]any{
		"channel_id": chatID,
		"message":    text,
	}

	if rootID := metadata["thread_id"]; rootID != "" {
		payload["root_id"] = rootID
	}

	return m.apiPost(ctx, "/api/v4/posts", payload)
}

// SendTyping sends a typing indicator via WebSocket.
func (m *MattermostAdapter) SendTyping(ctx context.Context, chatID string) error {
	if m.wsConn == nil {
		return nil
	}

	msg := map[string]any{
		"action": "user_typing",
		"data": map[string]any{
			"channel_id": chatID,
		},
	}

	data, _ := json.Marshal(msg)
	return m.wsConn.WriteMessage(websocket.TextMessage, data)
}

// SendImage sends an image to a Mattermost channel.
func (m *MattermostAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	fileID, err := m.uploadFile(ctx, chatID, imagePath)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	payload := map[string]any{
		"channel_id": chatID,
		"message":    caption,
		"file_ids":   []string{fileID},
	}

	return m.apiPost(ctx, "/api/v4/posts", payload)
}

// SendVoice sends a voice file to a Mattermost channel.
func (m *MattermostAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return m.SendDocument(ctx, chatID, audioPath, metadata)
}

// SendDocument sends a document to a Mattermost channel.
func (m *MattermostAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	fileID, err := m.uploadFile(ctx, chatID, filePath)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	payload := map[string]any{
		"channel_id": chatID,
		"file_ids":   []string{fileID},
	}

	return m.apiPost(ctx, "/api/v4/posts", payload)
}

// --- Internal ---

func (m *MattermostAdapter) getMe(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.serverURL+"/api/v4/users/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+m.token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var user struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}
	return user.ID, nil
}

func (m *MattermostAdapter) apiPost(ctx context.Context, path string, payload map[string]any) (*gateway.SendResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.serverURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return &gateway.SendResult{
			Success:   false,
			Error:     fmt.Sprintf("Mattermost API error %d: %s", resp.StatusCode, string(respBody)),
			Retryable: resp.StatusCode >= 500,
		}, nil
	}

	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	return &gateway.SendResult{
		Success:   true,
		MessageID: result.ID,
	}, nil
}

func (m *MattermostAdapter) uploadFile(ctx context.Context, channelID, filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	url := fmt.Sprintf("%s/api/v4/files?channel_id=%s&filename=%s",
		m.serverURL, channelID, filePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+m.token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		FileInfos []struct {
			ID string `json:"id"`
		} `json:"file_infos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.FileInfos) == 0 {
		return "", fmt.Errorf("no file info returned")
	}
	return result.FileInfos[0].ID, nil
}

// mattermostWSEvent is a WebSocket event from Mattermost.
type mattermostWSEvent struct {
	Event     string                 `json:"event"`
	Data      map[string]any         `json:"data"`
	Broadcast map[string]any         `json:"broadcast"`
	Seq       int64                  `json:"seq"`
}

func (m *MattermostAdapter) connectWebSocket(ctx context.Context) {
	wsURL := strings.Replace(m.serverURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/v4/websocket"

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		header := http.Header{}
		header.Set("Authorization", "Bearer "+m.token)

		conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
		if err != nil {
			slog.Error("Mattermost WebSocket connect failed", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}
		m.wsConn = conn

		// Send auth challenge.
		authMsg := map[string]any{
			"seq":    1,
			"action": "authentication_challenge",
			"data": map[string]any{
				"token": m.token,
			},
		}
		authData, _ := json.Marshal(authMsg)
		conn.WriteMessage(websocket.TextMessage, authData)

		m.readWebSocket(ctx, conn)
		conn.Close()

		if ctx.Err() != nil {
			return
		}
		slog.Info("Mattermost WebSocket disconnected, reconnecting...")
		time.Sleep(5 * time.Second)
	}
}

func (m *MattermostAdapter) readWebSocket(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			slog.Debug("Mattermost WebSocket read error", "error", err)
			return
		}

		var event mattermostWSEvent
		if err := json.Unmarshal(message, &event); err != nil {
			continue
		}

		if event.Event == "posted" {
			m.handlePostedEvent(event)
		}
	}
}

func (m *MattermostAdapter) handlePostedEvent(event mattermostWSEvent) {
	postStr, ok := event.Data["post"].(string)
	if !ok {
		return
	}

	var post struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
		UserID    string `json:"user_id"`
		RootID    string `json:"root_id"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal([]byte(postStr), &post); err != nil {
		return
	}

	// Skip own messages.
	if post.UserID == m.botUserID {
		return
	}

	channelType, _ := event.Data["channel_type"].(string)
	chatType := "channel"
	if channelType == "D" {
		chatType = "dm"
	} else if channelType == "G" {
		chatType = "group"
	}

	channelName, _ := event.Data["channel_display_name"].(string)
	senderName, _ := event.Data["sender_name"].(string)

	source := gateway.SessionSource{
		Platform: gateway.PlatformMattermost,
		ChatID:   post.ChannelID,
		ChatName: channelName,
		ChatType: chatType,
		UserID:   post.UserID,
		UserName: strings.TrimPrefix(senderName, "@"),
		ThreadID: post.RootID,
	}

	msgEvent := &gateway.MessageEvent{
		Text:        post.Message,
		MessageType: gateway.MessageTypeText,
		Source:      source,
		RawMessage:  event,
	}

	m.EmitMessage(msgEvent)
}

// Ensure MattermostAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*MattermostAdapter)(nil)
