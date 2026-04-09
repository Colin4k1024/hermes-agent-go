package platforms

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// EmailAdapter implements the gateway.PlatformAdapter interface for Email.
// It uses IMAP for receiving and SMTP for sending.
type EmailAdapter struct {
	BasePlatformAdapter

	// IMAP settings
	imapHost string
	imapPort string
	imapUser string
	imapPass string
	imapTLS  bool

	// SMTP settings
	smtpHost string
	smtpPort string
	smtpUser string
	smtpPass string
	smtpFrom string

	cancel context.CancelFunc
	mu     sync.Mutex
}

// NewEmailAdapter creates a new Email adapter.
func NewEmailAdapter() *EmailAdapter {
	return &EmailAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformEmail),
		imapHost:            envOrDefault("EMAIL_IMAP_HOST", ""),
		imapPort:            envOrDefault("EMAIL_IMAP_PORT", "993"),
		imapUser:            envOrDefault("EMAIL_IMAP_USER", ""),
		imapPass:            envOrDefault("EMAIL_IMAP_PASS", ""),
		imapTLS:             envOrDefault("EMAIL_IMAP_TLS", "true") == "true",
		smtpHost:            envOrDefault("EMAIL_SMTP_HOST", ""),
		smtpPort:            envOrDefault("EMAIL_SMTP_PORT", "587"),
		smtpUser:            envOrDefault("EMAIL_SMTP_USER", ""),
		smtpPass:            envOrDefault("EMAIL_SMTP_PASS", ""),
		smtpFrom:            envOrDefault("EMAIL_SMTP_FROM", ""),
	}
}

// Connect starts the email adapter (IMAP polling loop).
func (e *EmailAdapter) Connect(ctx context.Context) error {
	if e.imapHost == "" {
		return fmt.Errorf("EMAIL_IMAP_HOST not set")
	}
	if e.smtpHost == "" {
		return fmt.Errorf("EMAIL_SMTP_HOST not set")
	}
	if e.smtpFrom == "" {
		e.smtpFrom = e.imapUser
	}

	e.connected = true
	slog.Info("Email adapter connected", "imap_host", e.imapHost, "smtp_host", e.smtpHost)

	pollCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	go e.pollIMAP(pollCtx)

	return nil
}

// Disconnect stops the email adapter.
func (e *EmailAdapter) Disconnect() error {
	if e.cancel != nil {
		e.cancel()
	}
	e.connected = false
	return nil
}

// Send sends an email reply.
func (e *EmailAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	subject := metadata["subject"]
	if subject == "" {
		subject = "Re: message"
	}
	if !strings.HasPrefix(subject, "Re: ") {
		subject = "Re: " + subject
	}

	toAddr := chatID // chatID is the recipient email address

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n",
		e.smtpFrom, toAddr, subject)

	if messageID := metadata["message_id"]; messageID != "" {
		msg += fmt.Sprintf("In-Reply-To: %s\r\nReferences: %s\r\n", messageID, messageID)
	}

	msg += "\r\n" + text

	if err := e.sendSMTP(toAddr, []byte(msg)); err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}

	return &gateway.SendResult{Success: true}, nil
}

// SendTyping is a no-op for email.
func (e *EmailAdapter) SendTyping(ctx context.Context, chatID string) error {
	return nil
}

// SendImage sends an email with an image attachment (simplified: sends as text with path reference).
func (e *EmailAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	text := caption
	if text == "" {
		text = "Image attached: " + imagePath
	}
	return e.Send(ctx, chatID, text, metadata)
}

// SendVoice sends an email with a voice note reference.
func (e *EmailAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return e.Send(ctx, chatID, "Voice message: "+audioPath, metadata)
}

// SendDocument sends an email with a document reference.
func (e *EmailAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return e.Send(ctx, chatID, "Document: "+filePath, metadata)
}

// --- Internal ---

func (e *EmailAdapter) sendSMTP(to string, msg []byte) error {
	addr := e.smtpHost + ":" + e.smtpPort

	auth := smtp.PlainAuth("", e.smtpUser, e.smtpPass, e.smtpHost)

	// Use TLS for port 465, STARTTLS for others.
	if e.smtpPort == "465" {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: e.smtpHost})
		if err != nil {
			return fmt.Errorf("TLS dial: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, e.smtpHost)
		if err != nil {
			return fmt.Errorf("SMTP client: %w", err)
		}
		defer client.Close()

		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
		if err := client.Mail(e.smtpFrom); err != nil {
			return fmt.Errorf("SMTP mail: %w", err)
		}
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("SMTP rcpt: %w", err)
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("SMTP data: %w", err)
		}
		if _, err := w.Write(msg); err != nil {
			return fmt.Errorf("SMTP write: %w", err)
		}
		return w.Close()
	}

	return smtp.SendMail(addr, auth, e.smtpFrom, []string{to}, msg)
}

// pollIMAP polls the IMAP server for new messages.
// This is a simplified polling approach. A production implementation would
// use a proper IMAP library with IDLE support.
func (e *EmailAdapter) pollIMAP(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Placeholder: In production, connect to IMAP, fetch UNSEEN messages,
			// parse them, and emit MessageEvents. This requires an IMAP library
			// (e.g. github.com/emersion/go-imap) which is not included to avoid
			// heavy external dependencies.
			slog.Debug("Email IMAP poll cycle", "host", e.imapHost)
		}
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// Ensure EmailAdapter implements PlatformAdapter.
var _ gateway.PlatformAdapter = (*EmailAdapter)(nil)
