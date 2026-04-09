// Package platforms implements messaging platform adapters for the gateway.
package platforms

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// MaxMessageLength is the default maximum message length for platforms.
const MaxMessageLength = 4096

// BasePlatformAdapter provides common functionality for all platform adapters.
type BasePlatformAdapter struct {
	platform       gateway.Platform
	messageHandler func(event *gateway.MessageEvent)
	connected      bool
	maxRetries     int
	retryDelay     time.Duration
}

// NewBasePlatformAdapter creates a base adapter with defaults.
func NewBasePlatformAdapter(platform gateway.Platform) BasePlatformAdapter {
	return BasePlatformAdapter{
		platform:   platform,
		maxRetries: 3,
		retryDelay: 1 * time.Second,
	}
}

// Platform returns the platform identifier.
func (b *BasePlatformAdapter) Platform() gateway.Platform {
	return b.platform
}

// OnMessage registers the incoming message handler.
func (b *BasePlatformAdapter) OnMessage(handler func(event *gateway.MessageEvent)) {
	b.messageHandler = handler
}

// IsConnected returns the connection status.
func (b *BasePlatformAdapter) IsConnected() bool {
	return b.connected
}

// EmitMessage sends a message event to the registered handler.
func (b *BasePlatformAdapter) EmitMessage(event *gateway.MessageEvent) {
	if b.messageHandler != nil {
		b.messageHandler(event)
	}
}

// TruncateMessage truncates a message to the platform's max length.
func TruncateMessage(text string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = MaxMessageLength
	}
	if len(text) <= maxLen {
		return text
	}
	// Try to truncate at a word boundary.
	truncated := text[:maxLen-20]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen/2 {
		truncated = truncated[:lastSpace]
	}
	return truncated + "\n\n...(truncated)"
}

// SplitMessage splits a long message into chunks respecting the max length.
func SplitMessage(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = MaxMessageLength
	}
	if len(text) <= maxLen {
		return []string{text}
	}

	var parts []string
	remaining := text
	for len(remaining) > maxLen {
		// Try to split at a newline.
		splitIdx := maxLen
		for i := maxLen - 1; i > maxLen/2; i-- {
			if remaining[i] == '\n' {
				splitIdx = i + 1
				break
			}
		}
		parts = append(parts, remaining[:splitIdx])
		remaining = remaining[splitIdx:]
	}
	if len(remaining) > 0 {
		parts = append(parts, remaining)
	}
	return parts
}

// ExtractMediaFromResponse extracts MEDIA: tags from a response and returns
// the media file paths along with the cleaned text.
func ExtractMediaFromResponse(text string) (mediaFiles []MediaFile, cleanedText string) {
	lines := strings.Split(text, "\n")
	var cleanLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "MEDIA:") {
			path := strings.TrimPrefix(trimmed, "MEDIA:")
			path = strings.TrimSpace(path)
			if path != "" {
				isVoice := strings.HasSuffix(strings.ToLower(path), ".ogg") ||
					strings.HasSuffix(strings.ToLower(path), ".opus")
				mediaFiles = append(mediaFiles, MediaFile{Path: path, IsVoice: isVoice})
			}
		} else {
			cleanLines = append(cleanLines, line)
		}
	}

	cleanedText = strings.Join(cleanLines, "\n")
	return
}

// MediaFile represents an extracted media file from a response.
type MediaFile struct {
	Path    string
	IsVoice bool
}

// RetryWithBackoff retries an operation with exponential backoff.
func RetryWithBackoff(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error {
	var lastErr error
	delay := baseDelay

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}

		if err := fn(); err != nil {
			lastErr = err
			slog.Debug("Retry attempt failed", "attempt", attempt+1, "error", err)
			continue
		}
		return nil
	}

	return lastErr
}
