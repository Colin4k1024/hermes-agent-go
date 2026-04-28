package platforms

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

// TelegramAdapter implements PlatformAdapter for Telegram Bot API.
type TelegramAdapter struct {
	BasePlatformAdapter
	bot     *tgbotapi.BotAPI
	token   string
	botName string
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewTelegramAdapter(token string) *TelegramAdapter {
	return &TelegramAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformTelegram),
		token:               token,
	}
}

func (t *TelegramAdapter) Connect(ctx context.Context) error {
	bot, err := tgbotapi.NewBotAPI(t.token)
	if err != nil {
		return fmt.Errorf("telegram connect: %w", err)
	}
	t.bot = bot
	t.botName = bot.Self.UserName
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.connected = true

	slog.Info("Telegram connected", "bot", t.botName)

	go t.pollUpdates()
	return nil
}

func (t *TelegramAdapter) Disconnect() error {
	t.connected = false
	if t.cancel != nil {
		t.cancel()
	}
	if t.bot != nil {
		t.bot.StopReceivingUpdates()
	}
	return nil
}

func (t *TelegramAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	id, err := parseChatID(chatID)
	if err != nil {
		return &gateway.SendResult{Error: err.Error()}, err
	}

	msg := tgbotapi.NewMessage(id, text)
	msg.ParseMode = "Markdown"

	if threadID, ok := metadata["thread_id"]; ok {
		if tid, e := parseChatID(threadID); e == nil {
			msg.ReplyToMessageID = int(tid)
		}
	}

	sent, err := t.bot.Send(msg)
	if err != nil {
		return &gateway.SendResult{Error: err.Error(), Retryable: true}, err
	}

	return &gateway.SendResult{
		Success:   true,
		MessageID: fmt.Sprintf("%d", sent.MessageID),
	}, nil
}

func (t *TelegramAdapter) SendTyping(ctx context.Context, chatID string) error {
	id, err := parseChatID(chatID)
	if err != nil {
		return err
	}
	action := tgbotapi.NewChatAction(id, tgbotapi.ChatTyping)
	_, err = t.bot.Send(action)
	return err
}

func (t *TelegramAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	id, err := parseChatID(chatID)
	if err != nil {
		return &gateway.SendResult{Error: err.Error()}, err
	}
	photo := tgbotapi.NewPhoto(id, tgbotapi.FilePath(imagePath))
	photo.Caption = caption
	sent, err := t.bot.Send(photo)
	if err != nil {
		return &gateway.SendResult{Error: err.Error(), Retryable: true}, err
	}
	return &gateway.SendResult{Success: true, MessageID: fmt.Sprintf("%d", sent.MessageID)}, nil
}

func (t *TelegramAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	id, err := parseChatID(chatID)
	if err != nil {
		return &gateway.SendResult{Error: err.Error()}, err
	}
	voice := tgbotapi.NewVoice(id, tgbotapi.FilePath(audioPath))
	sent, err := t.bot.Send(voice)
	if err != nil {
		return &gateway.SendResult{Error: err.Error(), Retryable: true}, err
	}
	return &gateway.SendResult{Success: true, MessageID: fmt.Sprintf("%d", sent.MessageID)}, nil
}

func (t *TelegramAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	id, err := parseChatID(chatID)
	if err != nil {
		return &gateway.SendResult{Error: err.Error()}, err
	}
	doc := tgbotapi.NewDocument(id, tgbotapi.FilePath(filePath))
	sent, err := t.bot.Send(doc)
	if err != nil {
		return &gateway.SendResult{Error: err.Error(), Retryable: true}, err
	}
	return &gateway.SendResult{Success: true, MessageID: fmt.Sprintf("%d", sent.MessageID)}, nil
}

func (t *TelegramAdapter) pollUpdates() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := t.bot.GetUpdatesChan(u)

	for {
		select {
		case <-t.ctx.Done():
			return
		case update := <-updates:
			if update.Message == nil {
				continue
			}

			msg := update.Message
			chatType := "dm"
			if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
				chatType = "group"
				// Group mention gating: only respond to @bot messages
				if t.botName != "" && !strings.Contains(msg.Text, "@"+t.botName) && msg.ReplyToMessage == nil {
					continue
				}
			}

			text := msg.Text
			if text == "" && msg.Caption != "" {
				text = msg.Caption
			}

			event := &gateway.MessageEvent{
				Text:        text,
				MessageType: gateway.MessageTypeText,
				Source: gateway.SessionSource{
					Platform: gateway.PlatformTelegram,
					ChatID:   fmt.Sprintf("%d", msg.Chat.ID),
					ChatName: msg.Chat.Title,
					ChatType: chatType,
					UserID:   fmt.Sprintf("%d", msg.From.ID),
					UserName: msg.From.UserName,
				},
			}

			if msg.IsCommand() {
				event.MessageType = gateway.MessageTypeCommand
			}

			t.EmitMessage(event)
		}
	}
}

func parseChatID(s string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(s, "%d", &id)
	return id, err
}
