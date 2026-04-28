package platforms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/gateway"
)

const PlatformMatrix gateway.Platform = "matrix"

// MatrixAdapter implements PlatformAdapter for Matrix using the Client-Server API.
type MatrixAdapter struct {
	BasePlatformAdapter
	homeserver  string
	accessToken string
	userID      string
	client      *http.Client
	since       string
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewMatrixAdapter(homeserver, accessToken, userID string) *MatrixAdapter {
	return &MatrixAdapter{
		BasePlatformAdapter: NewBasePlatformAdapter(PlatformMatrix),
		homeserver:          homeserver,
		accessToken:         accessToken,
		userID:              userID,
		client:              &http.Client{Timeout: 60 * time.Second},
	}
}

func (m *MatrixAdapter) Connect(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.connected = true
	slog.Info("Matrix adapter connected", "homeserver", m.homeserver, "user", m.userID)
	go m.syncLoop()
	return nil
}

func (m *MatrixAdapter) Disconnect() error {
	m.connected = false
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

func (m *MatrixAdapter) Send(ctx context.Context, chatID string, text string, _ map[string]string) (*gateway.SendResult, error) {
	txnID := fmt.Sprintf("hermes_%d", time.Now().UnixNano())
	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s", m.homeserver, chatID, txnID)
	payload := map[string]string{"msgtype": "m.text", "body": text}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
	if err != nil {
		return &gateway.SendResult{Error: err.Error()}, err
	}
	req.Header.Set("Authorization", "Bearer "+m.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return &gateway.SendResult{Error: err.Error(), Retryable: true}, err
	}
	defer resp.Body.Close()

	var result struct {
		EventID string `json:"event_id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return &gateway.SendResult{Success: true, MessageID: result.EventID}, nil
}

func (m *MatrixAdapter) SendTyping(ctx context.Context, chatID string) error {
	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/typing/%s", m.homeserver, chatID, m.userID)
	payload := map[string]any{"typing": true, "timeout": 10000}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+m.accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (m *MatrixAdapter) SendImage(ctx context.Context, chatID string, _ string, caption string, md map[string]string) (*gateway.SendResult, error) {
	return m.Send(ctx, chatID, caption+" [image]", md)
}
func (m *MatrixAdapter) SendVoice(ctx context.Context, chatID string, _ string, md map[string]string) (*gateway.SendResult, error) {
	return m.Send(ctx, chatID, "[voice]", md)
}
func (m *MatrixAdapter) SendDocument(ctx context.Context, chatID string, _ string, md map[string]string) (*gateway.SendResult, error) {
	return m.Send(ctx, chatID, "[document]", md)
}

func (m *MatrixAdapter) syncLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		url := fmt.Sprintf("%s/_matrix/client/v3/sync?timeout=30000", m.homeserver)
		if m.since != "" {
			url += "&since=" + m.since
		}

		req, _ := http.NewRequestWithContext(m.ctx, "GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+m.accessToken)

		resp, err := m.client.Do(req)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var syncResp struct {
			NextBatch string `json:"next_batch"`
			Rooms     struct {
				Join map[string]struct {
					Timeline struct {
						Events []struct {
							Type    string `json:"type"`
							Sender  string `json:"sender"`
							Content struct {
								MsgType string `json:"msgtype"`
								Body    string `json:"body"`
							} `json:"content"`
						} `json:"events"`
					} `json:"timeline"`
				} `json:"join"`
			} `json:"rooms"`
		}

		if err := json.Unmarshal(body, &syncResp); err != nil {
			continue
		}

		m.since = syncResp.NextBatch

		for roomID, room := range syncResp.Rooms.Join {
			for _, evt := range room.Timeline.Events {
				if evt.Type != "m.room.message" || evt.Sender == m.userID {
					continue
				}
				event := &gateway.MessageEvent{
					Text:        evt.Content.Body,
					MessageType: gateway.MessageTypeText,
					Source: gateway.SessionSource{
						Platform: PlatformMatrix,
						ChatID:   roomID,
						ChatType: "group",
						UserID:   evt.Sender,
					},
				}
				m.EmitMessage(event)
			}
		}
	}
}
