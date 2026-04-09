package platforms

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// SlackAdapter implements the gateway.PlatformAdapter interface for Slack.
type SlackAdapter struct {
	BasePlatformAdapter
	api       *slack.Client
	socket    *socketmode.Client
	botToken  string
	appToken  string
	botUserID string
}

// NewSlackAdapter creates a new Slack adapter.
func NewSlackAdapter(botToken, appToken string) *SlackAdapter {
	if botToken == "" {
		botToken = os.Getenv("SLACK_BOT_TOKEN")
	}
	if appToken == "" {
		appToken = os.Getenv("SLACK_APP_TOKEN")
	}
	return &SlackAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(gateway.PlatformSlack),
		botToken:            botToken,
		appToken:            appToken,
	}
}

// Connect establishes a connection to Slack via Socket Mode.
func (s *SlackAdapter) Connect(ctx context.Context) error {
	if s.botToken == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN not set")
	}
	if s.appToken == "" {
		return fmt.Errorf("SLACK_APP_TOKEN not set")
	}

	api := slack.New(
		s.botToken,
		slack.OptionAppLevelToken(s.appToken),
	)

	// Get bot user ID.
	authResp, err := api.AuthTest()
	if err != nil {
		return fmt.Errorf("Slack auth test failed: %w", err)
	}
	s.botUserID = authResp.UserID
	slog.Info("Slack bot authenticated", "user_id", s.botUserID, "team", authResp.Team)

	socketClient := socketmode.New(api)
	s.api = api
	s.socket = socketClient
	s.connected = true

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-socketClient.Events:
				s.handleEvent(evt)
			}
		}
	}()

	go func() {
		if err := socketClient.RunContext(ctx); err != nil {
			slog.Error("Slack socket mode error", "error", err)
			s.connected = false
		}
	}()

	return nil
}

// Disconnect cleanly disconnects from Slack.
func (s *SlackAdapter) Disconnect() error {
	s.connected = false
	return nil
}

// Send sends a text message to a Slack channel.
func (s *SlackAdapter) Send(ctx context.Context, chatID string, text string, metadata map[string]string) (*gateway.SendResult, error) {
	if s.api == nil {
		return nil, fmt.Errorf("not connected")
	}

	opts := []slack.MsgOption{
		slack.MsgOptionText(text, false),
	}

	// Thread support.
	if threadTS := metadata["thread_id"]; threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}

	_, timestamp, err := s.api.PostMessage(chatID, opts...)
	if err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}

	return &gateway.SendResult{
		Success:   true,
		MessageID: timestamp,
	}, nil
}

// SendTyping sends a typing indicator (not natively supported by Slack).
func (s *SlackAdapter) SendTyping(ctx context.Context, chatID string) error {
	// Slack doesn't have a typing indicator API for bots.
	return nil
}

// SendImage sends an image to a Slack channel.
func (s *SlackAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, metadata map[string]string) (*gateway.SendResult, error) {
	return s.uploadFile(chatID, imagePath, caption, metadata)
}

// SendVoice sends a voice file to a Slack channel.
func (s *SlackAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, metadata map[string]string) (*gateway.SendResult, error) {
	return s.uploadFile(chatID, audioPath, "", metadata)
}

// SendDocument sends a document to a Slack channel.
func (s *SlackAdapter) SendDocument(ctx context.Context, chatID string, filePath string, metadata map[string]string) (*gateway.SendResult, error) {
	return s.uploadFile(chatID, filePath, "", metadata)
}

// --- Internal ---

func (s *SlackAdapter) uploadFile(chatID, filePath, initialComment string, metadata map[string]string) (*gateway.SendResult, error) {
	if s.api == nil {
		return nil, fmt.Errorf("not connected")
	}

	params := slack.FileUploadParameters{
		File:           filePath,
		Channels:       []string{chatID},
		InitialComment: initialComment,
	}

	if threadTS := metadata["thread_id"]; threadTS != "" {
		params.ThreadTimestamp = threadTS
	}

	_, err := s.api.UploadFile(params)
	if err != nil {
		return &gateway.SendResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, nil
	}

	return &gateway.SendResult{Success: true}, nil
}

func (s *SlackAdapter) handleEvent(evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}

		s.socket.Ack(*evt.Request)

		switch innerEvent := eventsAPIEvent.InnerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			// Ignore bot's own messages.
			if innerEvent.User == s.botUserID {
				return
			}
			// Ignore subtypes (message_changed, etc.) except for regular messages.
			if innerEvent.SubType != "" {
				return
			}

			s.handleMessage(innerEvent)

		case *slackevents.AppMentionEvent:
			s.handleMention(innerEvent)
		}

	case socketmode.EventTypeSlashCommand:
		cmd, ok := evt.Data.(slack.SlashCommand)
		if !ok {
			return
		}
		s.socket.Ack(*evt.Request)
		s.handleSlashCommand(cmd)
	}
}

func (s *SlackAdapter) handleMessage(msg *slackevents.MessageEvent) {
	chatType := "dm"
	if msg.Channel != "" && (msg.Channel[0] == 'C' || msg.Channel[0] == 'G') {
		chatType = "channel"
	}

	source := gateway.SessionSource{
		Platform: gateway.PlatformSlack,
		ChatID:   msg.Channel,
		ChatType: chatType,
		UserID:   msg.User,
	}

	// Resolve thread.
	if msg.ThreadTimeStamp != "" {
		source.ThreadID = msg.ThreadTimeStamp
	}

	// Get user name.
	user, err := s.api.GetUserInfo(msg.User)
	if err == nil {
		source.UserName = user.RealName
		if source.UserName == "" {
			source.UserName = user.Name
		}
	}

	// Get channel name.
	ch, err := s.api.GetConversationInfo(&slack.GetConversationInfoInput{ChannelID: msg.Channel})
	if err == nil {
		source.ChatName = ch.Name
		source.ChatTopic = ch.Topic.Value
	}

	event := &gateway.MessageEvent{
		Text:        msg.Text,
		MessageType: gateway.MessageTypeText,
		Source:      source,
		RawMessage:  msg,
	}

	s.EmitMessage(event)
}

func (s *SlackAdapter) handleMention(mention *slackevents.AppMentionEvent) {
	source := gateway.SessionSource{
		Platform: gateway.PlatformSlack,
		ChatID:   mention.Channel,
		ChatType: "channel",
		UserID:   mention.User,
	}

	if mention.ThreadTimeStamp != "" {
		source.ThreadID = mention.ThreadTimeStamp
	}

	event := &gateway.MessageEvent{
		Text:        mention.Text,
		MessageType: gateway.MessageTypeText,
		Source:      source,
		RawMessage:  mention,
	}

	s.EmitMessage(event)
}

func (s *SlackAdapter) handleSlashCommand(cmd slack.SlashCommand) {
	source := gateway.SessionSource{
		Platform: gateway.PlatformSlack,
		ChatID:   cmd.ChannelID,
		ChatType: "channel",
		UserID:   cmd.UserID,
		UserName: cmd.UserName,
	}

	event := &gateway.MessageEvent{
		Text:        cmd.Text,
		MessageType: gateway.MessageTypeCommand,
		Source:      source,
		RawMessage:  cmd,
	}

	s.EmitMessage(event)
}
