package platforms

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// SMSAdapter implements the gateway.PlatformAdapter interface for SMS via Twilio.
type SMSAdapter struct {
	BasePlatformAdapter
	accountSID  string
	authToken   string
	phoneNumber string
	httpClient  *http.Client
	cancel      context.CancelFunc
	webhookPort string
}

// NewSMSAdapter creates a new SMS/Twilio adapter.
func NewSMSAdapter(accountSID, authToken, phoneNumber string) *SMSAdapter {
	if accountSID == "" {
		accountSID = os.Getenv("TWILIO_ACCOUNT_SID")
	}
	if authToken == "" {
		authToken = os.Getenv("TWILIO_AUTH_TOKEN")
	}
	if phoneNumber == "" {
		phoneNumber = os.Getenv("TWILIO_PHONE_NUMBER")
	}
	return &SMSAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformSMS),
		accountSID:          accountSID,
		authToken:           authToken,
		phoneNumber:         phoneNumber,
		webhookPort:         envOrDefault("TWILIO_WEBHOOK_PORT", "9093"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect starts the SMS adapter.
func (s *SMSAdapter) Connect(ctx context.Context) error {
	if s.accountSID == "" {
		return fmt.Errorf("TWILIO_ACCOUNT_SID not set")
	}
	if s.authToken == "" {
		return fmt.Errorf("TWILIO_AUTH_TOKEN not set")
	}
	if s.phoneNumber == "" {
		return fmt.Errorf("TWILIO_PHONE_NUMBER not set")
	}

	s.connected = true
	slog.Info("SMS/Twilio adapter connected", "phone", s.phoneNumber)

	connCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Start webhook server for incoming SMS.
	go s.startWebhookServer(connCtx)

	return nil
}

// Disconnect stops the SMS adapter.
func (s *SMSAdapter) Disconnect() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.connected = false
	return nil
}

// Send sends an SMS via Twilio.
func (s *SMSAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	twilioURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json",
		s.accountSID)

	// Twilio has a 1600 character limit per SMS segment.
	parts := SplitMessage(text, 1600)
	var lastSID string

	for _, part := range parts {
		data := url.Values{}
		data.Set("To", chatID)
		data.Set("From", s.phoneNumber)
		data.Set("Body", part)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, twilioURL,
			strings.NewReader(data.Encode()))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.SetBasicAuth(s.accountSID, s.authToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return &gateway.SendResult{
				Success:   false,
				Error:     err.Error(),
				Retryable: true,
			}, nil
		}
		defer resp.Body.Close()

		var result struct {
			SID          string `json:"sid"`
			ErrorCode    *int   `json:"error_code"`
			ErrorMessage string `json:"error_message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return &gateway.SendResult{
				Success:   false,
				Error:     fmt.Sprintf("decode response: %v", err),
				Retryable: true,
			}, nil
		}

		if result.ErrorCode != nil {
			return &gateway.SendResult{
				Success:   false,
				Error:     result.ErrorMessage,
				Retryable: true,
			}, nil
		}

		lastSID = result.SID
	}

	return &gateway.SendResult{
		Success:   true,
		MessageID: lastSID,
	}, nil
}

// SendTyping is a no-op for SMS.
func (s *SMSAdapter) SendTyping(ctx context.Context, chatID string) error {
	return nil
}

// SendImage sends an MMS with an image via Twilio.
func (s *SMSAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	// MMS requires a publicly accessible URL for media.
	// Simplified: send caption as text.
	text := caption
	if text == "" {
		text = "[Image]"
	}
	return s.Send(ctx, chatID, text, metadata)
}

// SendVoice sends a voice message reference via SMS.
func (s *SMSAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return s.Send(ctx, chatID, "[Voice message]", metadata)
}

// SendDocument sends a document reference via SMS.
func (s *SMSAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return s.Send(ctx, chatID, "[Document]", metadata)
}

// --- Internal ---

func (s *SMSAdapter) startWebhookServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/sms/incoming", s.handleIncoming)

	server := &http.Server{
		Addr:    ":" + s.webhookPort,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	slog.Info("SMS webhook server starting", "port", s.webhookPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("SMS webhook server error", "error", err)
	}
}

func (s *SMSAdapter) handleIncoming(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	from := r.FormValue("From")
	body := r.FormValue("Body")
	messageSID := r.FormValue("MessageSid")
	numMedia := r.FormValue("NumMedia")

	source := gateway.SessionSource{
		Platform: gateway.PlatformSMS,
		ChatID:   from,
		ChatType: "dm",
		UserID:   from,
	}

	event := &gateway.MessageEvent{
		Text:        body,
		MessageType: gateway.MessageTypeText,
		Source:      source,
		Metadata:    map[string]string{"message_sid": messageSID},
	}

	// Handle MMS media.
	if numMedia != "" && numMedia != "0" {
		event.MessageType = gateway.MessageTypePhoto
		for i := 0; ; i++ {
			mediaURL := r.FormValue(fmt.Sprintf("MediaUrl%d", i))
			if mediaURL == "" {
				break
			}
			event.MediaURLs = append(event.MediaURLs, mediaURL)
		}
	}

	s.EmitMessage(event)

	// Respond with empty TwiML.
	w.Header().Set("Content-Type", "text/xml")
	w.Write([]byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><Response></Response>"))
}

// Ensure SMSAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*SMSAdapter)(nil)
