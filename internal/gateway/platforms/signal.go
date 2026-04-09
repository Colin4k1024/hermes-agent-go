package platforms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// SignalAdapter implements the gateway.PlatformAdapter interface for Signal
// via the signal-cli REST API.
type SignalAdapter struct {
	BasePlatformAdapter
	apiURL      string
	phoneNumber string
	httpClient  *http.Client
	cancel      context.CancelFunc
}

// NewSignalAdapter creates a new Signal adapter.
func NewSignalAdapter(apiURL, phoneNumber string) *SignalAdapter {
	if apiURL == "" {
		apiURL = os.Getenv("SIGNAL_CLI_URL")
	}
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	if phoneNumber == "" {
		phoneNumber = os.Getenv("SIGNAL_PHONE_NUMBER")
	}
	return &SignalAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformSignal),
		apiURL:              apiURL,
		phoneNumber:         phoneNumber,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect establishes a connection to the signal-cli REST API.
func (s *SignalAdapter) Connect(ctx context.Context) error {
	if s.phoneNumber == "" {
		return fmt.Errorf("SIGNAL_PHONE_NUMBER not set")
	}

	// Verify API is reachable.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiURL+"/v1/about", nil)
	if err != nil {
		return fmt.Errorf("create status request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("signal-cli REST API not reachable at %s: %w", s.apiURL, err)
	}
	resp.Body.Close()

	s.connected = true
	slog.Info("Signal adapter connected", "api_url", s.apiURL, "phone", s.phoneNumber)

	// Start polling for incoming messages.
	pollCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	go s.pollMessages(pollCtx)

	return nil
}

// Disconnect cleanly disconnects from Signal.
func (s *SignalAdapter) Disconnect() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.connected = false
	return nil
}

// Send sends a text message via signal-cli REST API.
func (s *SignalAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	payload := map[string]any{
		"message":    text,
		"number":     s.phoneNumber,
		"recipients": []string{chatID},
	}

	return s.apiPost(ctx, fmt.Sprintf("/v2/send"), payload)
}

// SendTyping sends a typing indicator.
func (s *SignalAdapter) SendTyping(ctx context.Context, chatID string) error {
	// signal-cli does not have a typing indicator endpoint; no-op.
	return nil
}

// SendImage sends an image via signal-cli REST API.
func (s *SignalAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	return s.sendWithAttachment(ctx, chatID, caption, imagePath)
}

// SendVoice sends a voice message via signal-cli REST API.
func (s *SignalAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return s.sendWithAttachment(ctx, chatID, "", audioPath)
}

// SendDocument sends a document via signal-cli REST API.
func (s *SignalAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return s.sendWithAttachment(ctx, chatID, "", filePath)
}

// --- Internal ---

func (s *SignalAdapter) apiPost(ctx context.Context, path string, payload map[string]any) (*gateway.SendResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
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
			Error:     fmt.Sprintf("signal-cli API error %d: %s", resp.StatusCode, string(respBody)),
			Retryable: resp.StatusCode >= 500,
		}, nil
	}

	var result struct {
		Timestamp string `json:"timestamp"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	return &gateway.SendResult{
		Success:   true,
		MessageID: result.Timestamp,
	}, nil
}

func (s *SignalAdapter) sendWithAttachment(ctx context.Context, chatID string, message string, attachmentPath string) (*gateway.SendResult, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add recipients.
	_ = writer.WriteField("recipients", chatID)
	_ = writer.WriteField("number", s.phoneNumber)
	if message != "" {
		_ = writer.WriteField("message", message)
	}

	// Add attachment file.
	f, err := os.Open(attachmentPath)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: fmt.Sprintf("open file: %v", err)}, nil
	}
	defer f.Close()

	part, err := writer.CreateFormFile("attachments", filepath.Base(attachmentPath))
	if err != nil {
		return &gateway.SendResult{Success: false, Error: fmt.Sprintf("create form file: %v", err)}, nil
	}
	if _, err := io.Copy(part, f); err != nil {
		return &gateway.SendResult{Success: false, Error: fmt.Sprintf("copy file: %v", err)}, nil
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL+"/v2/send", &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.httpClient.Do(req)
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
			Error:     fmt.Sprintf("signal-cli API error %d: %s", resp.StatusCode, string(respBody)),
			Retryable: resp.StatusCode >= 500,
		}, nil
	}

	return &gateway.SendResult{Success: true}, nil
}

// signalMessage represents a message from the signal-cli REST API.
type signalMessage struct {
	Envelope struct {
		Source     string `json:"source"`
		SourceName string `json:"sourceName"`
		SourceUUID string `json:"sourceUuid"`
		Timestamp  int64  `json:"timestamp"`
		DataMessage *struct {
			Message     string `json:"message"`
			Timestamp   int64  `json:"timestamp"`
			GroupInfo   *struct {
				GroupID   string `json:"groupId"`
				GroupName string `json:"groupName"`
			} `json:"groupInfo"`
			Attachments []struct {
				ContentType string `json:"contentType"`
				Filename    string `json:"filename"`
				ID          string `json:"id"`
			} `json:"attachments"`
		} `json:"dataMessage"`
	} `json:"envelope"`
}

func (s *SignalAdapter) pollMessages(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.fetchMessages(ctx)
		}
	}
}

func (s *SignalAdapter) fetchMessages(ctx context.Context) {
	url := fmt.Sprintf("%s/v1/receive/%s", s.apiURL, s.phoneNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var messages []signalMessage
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return
	}

	for _, msg := range messages {
		dm := msg.Envelope.DataMessage
		if dm == nil {
			continue
		}

		chatType := "dm"
		chatID := msg.Envelope.Source
		chatName := msg.Envelope.SourceName

		if dm.GroupInfo != nil {
			chatType = "group"
			chatID = dm.GroupInfo.GroupID
			chatName = dm.GroupInfo.GroupName
		}

		source := gateway.SessionSource{
			Platform:  gateway.PlatformSignal,
			ChatID:    chatID,
			ChatName:  chatName,
			ChatType:  chatType,
			UserID:    msg.Envelope.Source,
			UserName:  msg.Envelope.SourceName,
			UserIDAlt: msg.Envelope.SourceUUID,
		}

		if dm.GroupInfo != nil {
			source.ChatIDAlt = dm.GroupInfo.GroupID
		}

		event := &gateway.MessageEvent{
			Text:        dm.Message,
			MessageType: gateway.MessageTypeText,
			Source:      source,
			RawMessage:  msg,
		}

		if len(dm.Attachments) > 0 {
			for _, att := range dm.Attachments {
				event.MediaURLs = append(event.MediaURLs, att.ID)
			}
			event.MessageType = gateway.MessageTypeDocument
		}

		s.EmitMessage(event)
	}
}

// Ensure SignalAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*SignalAdapter)(nil)
