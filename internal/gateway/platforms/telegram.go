package platforms

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// TelegramAdapter implements the gateway.PlatformAdapter interface for Telegram.
type TelegramAdapter struct {
	BasePlatformAdapter
	bot   *tgbotapi.BotAPI
	token string
}

// NewTelegramAdapter creates a new Telegram adapter.
func NewTelegramAdapter(token string) *TelegramAdapter {
	if token == "" {
		token = os.Getenv("TELEGRAM_BOT_TOKEN")
	}
	return &TelegramAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformTelegram),
		token:               token,
	}
}

// Connect establishes a connection to the Telegram Bot API.
func (t *TelegramAdapter) Connect(ctx context.Context) error {
	if t.token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN not set")
	}

	bot, err := tgbotapi.NewBotAPI(t.token)
	if err != nil {
		return fmt.Errorf("create Telegram bot: %w", err)
	}

	t.bot = bot
	t.connected = true
	slog.Info("Telegram bot connected", "username", bot.Self.UserName)

	// Start polling for updates.
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	go func() {
		for {
			select {
			case <-ctx.Done():
				bot.StopReceivingUpdates()
				return
			case update := <-updates:
				if update.Message == nil {
					continue
				}
				t.handleUpdate(update)
			}
		}
	}()

	return nil
}

// Disconnect cleanly disconnects from Telegram.
func (t *TelegramAdapter) Disconnect() error {
	if t.bot != nil {
		t.bot.StopReceivingUpdates()
	}
	t.connected = false
	return nil
}

// Send sends a text message to a Telegram chat.
func (t *TelegramAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	if t.bot == nil {
		return nil, fmt.Errorf("not connected")
	}

	id, err := parseChatID(chatID)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	// Split long messages.
	parts := SplitMessage(text, 4096)
	var lastMsgID int

	for _, part := range parts {
		msg := tgbotapi.NewMessage(id, part)
		msg.ParseMode = "MarkdownV2"

		// Set thread ID if provided.
		if threadID := metadata["thread_id"]; threadID != "" {
			tid, _ := parseChatID(threadID)
			msg.ReplyToMessageID = int(tid)
		}

		// Try with Markdown, fall back to plain text.
		sent, err := t.bot.Send(msg)
		if err != nil {
			// Retry without markdown.
			msg.ParseMode = ""
			sent, err = t.bot.Send(msg)
			if err != nil {
				return &gateway.SendResult{
					Success:   false,
					Error:     err.Error(),
					Retryable: true,
				}, nil
			}
		}
		lastMsgID = sent.MessageID
	}

	return &gateway.SendResult{
		Success:   true,
		MessageID: fmt.Sprintf("%d", lastMsgID),
	}, nil
}

// SendTyping sends a typing indicator to a Telegram chat.
func (t *TelegramAdapter) SendTyping(ctx context.Context, chatID string) error {
	if t.bot == nil {
		return fmt.Errorf("not connected")
	}

	id, err := parseChatID(chatID)
	if err != nil {
		return err
	}

	action := tgbotapi.NewChatAction(id, tgbotapi.ChatTyping)
	_, err = t.bot.Request(action)
	return err
}

// SendImage sends an image to a Telegram chat.
func (t *TelegramAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	if t.bot == nil {
		return nil, fmt.Errorf("not connected")
	}

	id, err := parseChatID(chatID)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	photo := tgbotapi.NewPhoto(id, tgbotapi.FilePath(imagePath))
	if caption != "" {
		photo.Caption = TruncateMessage(caption, 1024)
	}

	sent, err := t.bot.Send(photo)
	if err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}

	return &gateway.SendResult{
		Success:   true,
		MessageID: fmt.Sprintf("%d", sent.MessageID),
	}, nil
}

// SendVoice sends a voice message to a Telegram chat.
func (t *TelegramAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	if t.bot == nil {
		return nil, fmt.Errorf("not connected")
	}

	id, err := parseChatID(chatID)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	voice := tgbotapi.NewVoice(id, tgbotapi.FilePath(audioPath))
	sent, err := t.bot.Send(voice)
	if err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}

	return &gateway.SendResult{
		Success:   true,
		MessageID: fmt.Sprintf("%d", sent.MessageID),
	}, nil
}

// SendDocument sends a document to a Telegram chat.
func (t *TelegramAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	if t.bot == nil {
		return nil, fmt.Errorf("not connected")
	}

	id, err := parseChatID(chatID)
	if err != nil {
		return &gateway.SendResult{Success: false, Error: err.Error()}, nil
	}

	doc := tgbotapi.NewDocument(id, tgbotapi.FilePath(filePath))
	sent, err := t.bot.Send(doc)
	if err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}

	return &gateway.SendResult{
		Success:   true,
		MessageID: fmt.Sprintf("%d", sent.MessageID),
	}, nil
}

// --- Internal ---

func (t *TelegramAdapter) handleUpdate(update tgbotapi.Update) {
	msg := update.Message
	if msg == nil {
		return
	}

	chatType := "dm"
	switch msg.Chat.Type {
	case "group", "supergroup":
		chatType = "group"
	case "channel":
		chatType = "channel"
	}

	source := gateway.SessionSource{
		Platform: gateway.PlatformTelegram,
		ChatID:   fmt.Sprintf("%d", msg.Chat.ID),
		ChatName: msg.Chat.Title,
		ChatType: chatType,
		UserID:   fmt.Sprintf("%d", msg.From.ID),
		UserName: effectiveName(msg.From),
	}

	if msg.Chat.Title == "" && chatType == "dm" {
		source.ChatName = effectiveName(msg.From)
	}

	// Handle reply-to as thread context.
	if msg.ReplyToMessage != nil {
		source.ThreadID = fmt.Sprintf("%d", msg.ReplyToMessage.MessageID)
	}

	event := &gateway.MessageEvent{
		Text:        msg.Text,
		MessageType: gateway.MessageTypeText,
		Source:      source,
		RawMessage:  update,
	}

	// Detect message type.
	if msg.Photo != nil && len(msg.Photo) > 0 {
		event.MessageType = gateway.MessageTypePhoto
		if msg.Caption != "" {
			event.Text = msg.Caption
		}
	} else if msg.Voice != nil {
		event.MessageType = gateway.MessageTypeVoice
	} else if msg.Audio != nil {
		event.MessageType = gateway.MessageTypeAudio
	} else if msg.Video != nil {
		event.MessageType = gateway.MessageTypeVideo
	} else if msg.Document != nil {
		event.MessageType = gateway.MessageTypeDocument
	} else if msg.Sticker != nil {
		event.MessageType = gateway.MessageTypeSticker
	}

	// Detect commands.
	if msg.IsCommand() {
		event.MessageType = gateway.MessageTypeCommand
		event.Text = "/" + msg.Command()
		if msg.CommandArguments() != "" {
			event.Text += " " + msg.CommandArguments()
		}
	}

	t.EmitMessage(event)
}

func effectiveName(user *tgbotapi.User) string {
	if user == nil {
		return ""
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name == "" {
		name = user.UserName
	}
	return name
}

func parseChatID(s string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(s, "%d", &id)
	return id, err
}
