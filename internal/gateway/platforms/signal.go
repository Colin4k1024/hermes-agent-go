package platforms

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

const PlatformSignal gateway.Platform = "signal"

// SignalAdapter implements PlatformAdapter using signal-cli subprocess.
type SignalAdapter struct {
	BasePlatformAdapter
	cliPath string
	account string
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewSignalAdapter(cliPath, account string) *SignalAdapter {
	if cliPath == "" {
		cliPath = "signal-cli"
	}
	return &SignalAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(PlatformSignal),
		cliPath:             cliPath,
		account:             account,
	}
}

func (s *SignalAdapter) Connect(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.connected = true
	slog.Info("Signal adapter connected", "account", s.account)
	go s.receiveLoop()
	return nil
}

func (s *SignalAdapter) Disconnect() error {
	s.connected = false
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *SignalAdapter) Send(_ context.Context, chatID string, text string, _ map[string]string) (*gateway.SendResult, error) {
	args := []string{"-a", s.account, "send", "-m", text, chatID}
	cmd := exec.CommandContext(s.ctx, s.cliPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &gateway.SendResult{Error: string(out)}, err
	}
	return &gateway.SendResult{Success: true}, nil
}

func (s *SignalAdapter) SendTyping(_ context.Context, _ string) error { return nil }
func (s *SignalAdapter) SendImage(ctx context.Context, chatID string, imagePath string, caption string, _ map[string]string) (*gateway.SendResult, error) {
	args := []string{"-a", s.account, "send", "-m", caption, "-a", imagePath, chatID}
	cmd := exec.CommandContext(ctx, s.cliPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &gateway.SendResult{Error: string(out)}, err
	}
	return &gateway.SendResult{Success: true}, nil
}
func (s *SignalAdapter) SendVoice(ctx context.Context, chatID string, audioPath string, md map[string]string) (*gateway.SendResult, error) {
	return s.SendImage(ctx, chatID, audioPath, "", md)
}
func (s *SignalAdapter) SendDocument(ctx context.Context, chatID string, filePath string, md map[string]string) (*gateway.SendResult, error) {
	return s.SendImage(ctx, chatID, filePath, "", md)
}

func (s *SignalAdapter) receiveLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		cmd := exec.CommandContext(s.ctx, s.cliPath, "-a", s.account, "receive", "--json")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			slog.Error("Signal receive pipe error", "error", err)
			return
		}

		if err := cmd.Start(); err != nil {
			slog.Error("Signal receive start error", "error", err)
			return
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var msg struct {
				Envelope struct {
					Source      string `json:"source"`
					SourceName  string `json:"sourceName"`
					DataMessage *struct {
						Message   string `json:"message"`
						GroupInfo *struct {
							GroupID string `json:"groupId"`
						} `json:"groupInfo"`
					} `json:"dataMessage"`
				} `json:"envelope"`
			}

			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				continue
			}

			if msg.Envelope.DataMessage == nil || msg.Envelope.DataMessage.Message == "" {
				continue
			}

			chatID := msg.Envelope.Source
			chatType := "dm"
			if msg.Envelope.DataMessage.GroupInfo != nil {
				chatID = msg.Envelope.DataMessage.GroupInfo.GroupID
				chatType = "group"
			}

			event := &gateway.MessageEvent{
				Text:        msg.Envelope.DataMessage.Message,
				MessageType: gateway.MessageTypeText,
				Source: gateway.SessionSource{
					Platform: PlatformSignal,
					ChatID:   chatID,
					ChatType: chatType,
					UserID:   msg.Envelope.Source,
					UserName: msg.Envelope.SourceName,
				},
			}
			s.EmitMessage(event)
		}

		cmd.Wait()
	}
}

// Ensure unused imports don't cause build failures
var _ = os.Getenv
var _ = strings.TrimSpace
var _ = fmt.Sprintf
