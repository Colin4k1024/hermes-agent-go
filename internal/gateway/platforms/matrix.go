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

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// MatrixAdapter implements the gateway.PlatformAdapter interface for Matrix
// using the Matrix client-server HTTP API.
type MatrixAdapter struct {
	BasePlatformAdapter
	homeserver  string
	accessToken string
	userID      string
	httpClient  *http.Client
	cancel      context.CancelFunc
	nextBatch   string // sync token for /sync polling
}

// NewMatrixAdapter creates a new Matrix adapter.
func NewMatrixAdapter(homeserver, accessToken string) *MatrixAdapter {
	if homeserver == "" {
		homeserver = os.Getenv("MATRIX_HOMESERVER")
	}
	if accessToken == "" {
		accessToken = os.Getenv("MATRIX_ACCESS_TOKEN")
	}
	// Ensure homeserver URL doesn't have trailing slash.
	homeserver = strings.TrimRight(homeserver, "/")

	return &MatrixAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformMatrix),
		homeserver:          homeserver,
		accessToken:         accessToken,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Connect establishes a connection to the Matrix homeserver.
func (m *MatrixAdapter) Connect(ctx context.Context) error {
	if m.homeserver == "" {
		return fmt.Errorf("MATRIX_HOMESERVER not set")
	}
	if m.accessToken == "" {
		return fmt.Errorf("MATRIX_ACCESS_TOKEN not set")
	}

	// Verify connection by fetching our own user ID.
	userID, err := m.whoAmI(ctx)
	if err != nil {
		return fmt.Errorf("Matrix whoami failed: %w", err)
	}
	m.userID = userID

	m.connected = true
	slog.Info("Matrix adapter connected", "homeserver", m.homeserver, "user_id", m.userID)

	// Start sync loop.
	syncCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	go m.syncLoop(syncCtx)

	return nil
}

// Disconnect cleanly disconnects from Matrix.
func (m *MatrixAdapter) Disconnect() error {
	if m.cancel != nil {
		m.cancel()
	}
	m.connected = false
	return nil
}

// Send sends a text message to a Matrix room.
func (m *MatrixAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	txnID := fmt.Sprintf("%d", time.Now().UnixNano())

	body := map[string]string{
		"msgtype": "m.text",
		"body":    text,
	}

	// If text looks like it has markdown formatting, use m.text with format.
	if strings.ContainsAny(text, "*_`~[") {
		body["format"] = "org.matrix.custom.html"
		body["formatted_body"] = text
	}

	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		m.homeserver, chatID, txnID)

	result, err := m.matrixPUT(ctx, url, body)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SendTyping sends a typing indicator to a Matrix room.
func (m *MatrixAdapter) SendTyping(ctx context.Context, chatID string) error {
	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/typing/%s",
		m.homeserver, chatID, m.userID)

	body := map[string]any{
		"typing":  true,
		"timeout": 10000,
	}

	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.accessToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// SendImage sends an image to a Matrix room (uploads via media API then sends event).
func (m *MatrixAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	mxcURI, err := m.uploadMedia(ctx, imagePath, "image/png")
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	txnID := fmt.Sprintf("%d", time.Now().UnixNano())
	body := map[string]any{
		"msgtype": "m.image",
		"body":    caption,
		"url":     mxcURI,
	}

	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		m.homeserver, chatID, txnID)

	return m.matrixPUT(ctx, url, body)
}

// SendVoice sends a voice message to a Matrix room.
func (m *MatrixAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	mxcURI, err := m.uploadMedia(ctx, audioPath, "audio/ogg")
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	txnID := fmt.Sprintf("%d", time.Now().UnixNano())
	body := map[string]any{
		"msgtype": "m.audio",
		"body":    "voice message",
		"url":     mxcURI,
	}

	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		m.homeserver, chatID, txnID)

	return m.matrixPUT(ctx, url, body)
}

// SendDocument sends a document to a Matrix room.
func (m *MatrixAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	mxcURI, err := m.uploadMedia(ctx, filePath, "application/octet-stream")
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	txnID := fmt.Sprintf("%d", time.Now().UnixNano())
	body := map[string]any{
		"msgtype":  "m.file",
		"body":     filePath,
		"url":      mxcURI,
		"filename": filePath,
	}

	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		m.homeserver, chatID, txnID)

	return m.matrixPUT(ctx, url, body)
}

// --- Internal ---

func (m *MatrixAdapter) whoAmI(ctx context.Context) (string, error) {
	url := m.homeserver + "/_matrix/client/v3/account/whoami"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+m.accessToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("whoami returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.UserID, nil
}

func (m *MatrixAdapter) matrixPUT(ctx context.Context, url string, body any) (*gateway.SendResult, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.accessToken)

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
			Error:     fmt.Sprintf("Matrix API error %d: %s", resp.StatusCode, string(respBody)),
			Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429,
		}, nil
	}

	var result struct {
		EventID string `json:"event_id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	return &gateway.SendResult{
		Success:   true,
		MessageID: result.EventID,
	}, nil
}

func (m *MatrixAdapter) uploadMedia(ctx context.Context, filePath string, contentType string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	url := fmt.Sprintf("%s/_matrix/media/v3/upload?filename=%s", m.homeserver, filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+m.accessToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ContentURI string `json:"content_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ContentURI, nil
}

// matrixSyncResponse represents the response from the /sync endpoint.
type matrixSyncResponse struct {
	NextBatch string `json:"next_batch"`
	Rooms     struct {
		Join map[string]struct {
			Timeline struct {
				Events []matrixEvent `json:"events"`
			} `json:"timeline"`
		} `json:"join"`
	} `json:"rooms"`
}

type matrixEvent struct {
	Type     string `json:"type"`
	EventID  string `json:"event_id"`
	Sender   string `json:"sender"`
	Content  struct {
		MsgType       string `json:"msgtype"`
		Body          string `json:"body"`
		URL           string `json:"url"`
		Format        string `json:"format"`
		FormattedBody string `json:"formatted_body"`
	} `json:"content"`
	OriginServerTS int64 `json:"origin_server_ts"`
}

func (m *MatrixAdapter) syncLoop(ctx context.Context) {
	// Do an initial sync to get the next_batch token (filter to avoid old messages).
	m.doSync(ctx, true)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			m.doSync(ctx, false)
		}
	}
}

func (m *MatrixAdapter) doSync(ctx context.Context, initial bool) {
	url := fmt.Sprintf("%s/_matrix/client/v3/sync?timeout=30000", m.homeserver)
	if m.nextBatch != "" {
		url += "&since=" + m.nextBatch
	}
	if initial {
		// On initial sync, filter to get no old messages.
		url += "&filter={\"room\":{\"timeline\":{\"limit\":0}}}"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+m.accessToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			slog.Debug("Matrix sync error", "error", err)
			time.Sleep(5 * time.Second)
		}
		return
	}
	defer resp.Body.Close()

	var syncResp matrixSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return
	}

	m.nextBatch = syncResp.NextBatch

	if initial {
		return
	}

	// Process new messages.
	for roomID, room := range syncResp.Rooms.Join {
		for _, event := range room.Timeline.Events {
			if event.Type != "m.room.message" {
				continue
			}
			// Skip our own messages.
			if event.Sender == m.userID {
				continue
			}
			m.handleMatrixEvent(roomID, event)
		}
	}
}

func (m *MatrixAdapter) handleMatrixEvent(roomID string, event matrixEvent) {
	source := gateway.SessionSource{
		Platform: gateway.PlatformMatrix,
		ChatID:   roomID,
		ChatType: "group", // Matrix rooms are group-like by default.
		UserID:   event.Sender,
		UserName: event.Sender,
	}

	msgType := gateway.MessageTypeText
	switch event.Content.MsgType {
	case "m.image":
		msgType = gateway.MessageTypePhoto
	case "m.audio":
		msgType = gateway.MessageTypeVoice
	case "m.video":
		msgType = gateway.MessageTypeVideo
	case "m.file":
		msgType = gateway.MessageTypeDocument
	}

	msgEvent := &gateway.MessageEvent{
		Text:        event.Content.Body,
		MessageType: msgType,
		Source:      source,
		RawMessage:  event,
	}

	if event.Content.URL != "" {
		msgEvent.MediaURLs = append(msgEvent.MediaURLs, event.Content.URL)
	}

	m.EmitMessage(msgEvent)
}

// Ensure MatrixAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*MatrixAdapter)(nil)
