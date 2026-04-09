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

// HomeAssistantAdapter implements the gateway.PlatformAdapter interface for Home Assistant.
// It uses the WebSocket API for event listening and HTTP API for actions.
type HomeAssistantAdapter struct {
	BasePlatformAdapter
	hassURL    string
	token      string
	httpClient *http.Client
	wsConn     *websocket.Conn
	cancel     context.CancelFunc
	msgID      int64
}

// NewHomeAssistantAdapter creates a new Home Assistant adapter.
func NewHomeAssistantAdapter(hassURL, token string) *HomeAssistantAdapter {
	if hassURL == "" {
		hassURL = os.Getenv("HASS_URL")
	}
	if token == "" {
		token = os.Getenv("HASS_TOKEN")
	}
	hassURL = strings.TrimRight(hassURL, "/")

	return &HomeAssistantAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformHomeAssistant),
		hassURL:             hassURL,
		token:               token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect establishes a connection to Home Assistant via WebSocket.
func (h *HomeAssistantAdapter) Connect(ctx context.Context) error {
	if h.hassURL == "" {
		return fmt.Errorf("HASS_URL not set")
	}
	if h.token == "" {
		return fmt.Errorf("HASS_TOKEN not set")
	}

	// Verify HTTP API is reachable.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.hassURL+"/api/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Home Assistant not reachable at %s: %w", h.hassURL, err)
	}
	resp.Body.Close()

	h.connected = true
	slog.Info("Home Assistant adapter connected", "url", h.hassURL)

	connCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel

	go h.connectWebSocket(connCtx)

	return nil
}

// Disconnect cleanly disconnects from Home Assistant.
func (h *HomeAssistantAdapter) Disconnect() error {
	if h.cancel != nil {
		h.cancel()
	}
	if h.wsConn != nil {
		h.wsConn.Close()
	}
	h.connected = false
	return nil
}

// Send sends a notification via Home Assistant (using the notify service).
func (h *HomeAssistantAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	// chatID is the notification target (e.g. "notify.mobile_app_phone").
	service := chatID
	if !strings.Contains(service, ".") {
		service = "notify." + service
	}

	// Convert service domain.action to API path.
	parts := strings.SplitN(service, ".", 2)
	if len(parts) != 2 {
		return &gateway.SendResult{Success: false, Error: "invalid service: " + service}, nil
	}

	url := fmt.Sprintf("%s/api/services/%s/%s", h.hassURL, parts[0], parts[1])

	payload := map[string]any{
		"message": text,
	}
	if title := metadata["title"]; title != "" {
		payload["title"] = title
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.token)

	resp, err := h.httpClient.Do(req)
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
			Error:     fmt.Sprintf("HA API error %d: %s", resp.StatusCode, string(respBody)),
			Retryable: resp.StatusCode >= 500,
		}, nil
	}

	return &gateway.SendResult{Success: true}, nil
}

// SendTyping is a no-op for Home Assistant.
func (h *HomeAssistantAdapter) SendTyping(ctx context.Context, chatID string) error {
	return nil
}

// SendImage sends an image notification.
func (h *HomeAssistantAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	text := caption
	if text == "" {
		text = "Image"
	}
	metadata["image"] = imagePath
	return h.Send(ctx, chatID, text, metadata)
}

// SendVoice sends a voice notification.
func (h *HomeAssistantAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return h.Send(ctx, chatID, "[Voice message]", metadata)
}

// SendDocument sends a document notification.
func (h *HomeAssistantAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return h.Send(ctx, chatID, "[Document] "+filePath, metadata)
}

// --- Internal ---

// hassWSMessage represents a Home Assistant WebSocket message.
type hassWSMessage struct {
	Type    string         `json:"type"`
	ID      int64          `json:"id,omitempty"`
	Success *bool          `json:"success,omitempty"`
	Event   *hassWSEvent   `json:"event,omitempty"`
	AccessToken string     `json:"access_token,omitempty"`
}

type hassWSEvent struct {
	EventType string         `json:"event_type"`
	Data      map[string]any `json:"data"`
}

func (h *HomeAssistantAdapter) connectWebSocket(ctx context.Context) {
	wsURL := strings.Replace(h.hassURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/websocket"

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
		if err != nil {
			slog.Error("Home Assistant WebSocket connect failed", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}
		h.wsConn = conn
		h.msgID = 0

		if err := h.authenticate(conn); err != nil {
			slog.Error("Home Assistant WebSocket auth failed", "error", err)
			conn.Close()
			time.Sleep(5 * time.Second)
			continue
		}

		if err := h.subscribeEvents(conn); err != nil {
			slog.Error("Home Assistant WebSocket subscribe failed", "error", err)
			conn.Close()
			time.Sleep(5 * time.Second)
			continue
		}

		h.readWebSocket(ctx, conn)
		conn.Close()

		if ctx.Err() != nil {
			return
		}
		slog.Info("Home Assistant WebSocket disconnected, reconnecting...")
		time.Sleep(5 * time.Second)
	}
}

func (h *HomeAssistantAdapter) authenticate(conn *websocket.Conn) error {
	// Read auth_required message.
	var authReq hassWSMessage
	if err := conn.ReadJSON(&authReq); err != nil {
		return fmt.Errorf("read auth_required: %w", err)
	}
	if authReq.Type != "auth_required" {
		return fmt.Errorf("expected auth_required, got: %s", authReq.Type)
	}

	// Send auth.
	authMsg := hassWSMessage{
		Type:        "auth",
		AccessToken: h.token,
	}
	if err := conn.WriteJSON(authMsg); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}

	// Read auth result.
	var authResult hassWSMessage
	if err := conn.ReadJSON(&authResult); err != nil {
		return fmt.Errorf("read auth result: %w", err)
	}
	if authResult.Type != "auth_ok" {
		return fmt.Errorf("auth failed: %s", authResult.Type)
	}

	return nil
}

func (h *HomeAssistantAdapter) subscribeEvents(conn *websocket.Conn) error {
	h.msgID++
	subscribeMsg := map[string]any{
		"id":         h.msgID,
		"type":       "subscribe_events",
		"event_type": "hermes_agent_input",
	}
	return conn.WriteJSON(subscribeMsg)
}

func (h *HomeAssistantAdapter) readWebSocket(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			slog.Debug("Home Assistant WebSocket read error", "error", err)
			return
		}

		var msg hassWSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		if msg.Type == "event" && msg.Event != nil {
			h.handleEvent(msg.Event)
		}
	}
}

func (h *HomeAssistantAdapter) handleEvent(event *hassWSEvent) {
	data := event.Data
	text, _ := data["text"].(string)
	if text == "" {
		return
	}

	chatID, _ := data["chat_id"].(string)
	if chatID == "" {
		chatID = "homeassistant"
	}
	userID, _ := data["user_id"].(string)
	userName, _ := data["user_name"].(string)

	source := gateway.SessionSource{
		Platform: gateway.PlatformHomeAssistant,
		ChatID:   chatID,
		ChatType: "dm",
		UserID:   userID,
		UserName: userName,
	}

	msgEvent := &gateway.MessageEvent{
		Text:        text,
		MessageType: gateway.MessageTypeText,
		Source:      source,
		RawMessage:  event,
	}

	h.EmitMessage(msgEvent)
}

// Ensure HomeAssistantAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*HomeAssistantAdapter)(nil)
