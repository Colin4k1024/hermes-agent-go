package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/state"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

func init() {
	store.RegisterDriver("sqlite", func(ctx context.Context, cfg store.StoreConfig) (store.Store, error) {
		url := cfg.URL
		if url == "" {
			url = ""
		}
		return New(url)
	})
}

// SQLiteStore wraps the existing state.SessionDB as a store.Store for local dev.
type SQLiteStore struct {
	db *state.SessionDB
}

// New creates a SQLiteStore wrapping the existing SessionDB.
func New(dbPath string) (*SQLiteStore, error) {
	db, err := state.NewSessionDB(dbPath)
	if err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Sessions() store.SessionStore { return &sqliteSessions{db: s.db} }
func (s *SQLiteStore) Messages() store.MessageStore { return &sqliteMessages{db: s.db} }
func (s *SQLiteStore) Users() store.UserStore       { return &sqliteUsers{} }
func (s *SQLiteStore) Close() error                 { return s.db.Close() }
func (s *SQLiteStore) Migrate(_ context.Context) error { return nil } // SQLite migrations handled by SessionDB

var _ store.Store = (*SQLiteStore)(nil)

// --- Session adapter ---

type sqliteSessions struct{ db *state.SessionDB }

func (ss *sqliteSessions) Create(_ context.Context, _ string, sess *store.Session) error {
	return ss.db.CreateSession(sess.ID, sess.Platform, sess.Model, sess.ParentSessionID)
}

func (ss *sqliteSessions) Get(_ context.Context, _, sessionID string) (*store.Session, error) {
	raw, err := ss.db.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	return rawToSession(raw), nil
}

func (ss *sqliteSessions) End(_ context.Context, _, sessionID, reason string) error {
	return ss.db.EndSession(sessionID, reason)
}

func (ss *sqliteSessions) List(_ context.Context, _ string, opts store.ListOptions) ([]*store.Session, int, error) {
	source := opts.Platform
	raws, err := ss.db.ListSessions(source, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, err
	}
	var sessions []*store.Session
	for _, raw := range raws {
		sessions = append(sessions, rawToSession(raw))
	}
	return sessions, len(sessions), nil
}

func (ss *sqliteSessions) Delete(_ context.Context, _, sessionID string) error {
	return ss.db.DeleteSession(sessionID)
}

func (ss *sqliteSessions) UpdateTokens(_ context.Context, _, sessionID string, delta store.TokenDelta) error {
	return ss.db.UpdateTokenCounts(sessionID, delta.Input, delta.Output, delta.CacheRead, delta.CacheWrite, delta.Reasoning)
}

func (ss *sqliteSessions) SetTitle(_ context.Context, _, sessionID, title string) error {
	return ss.db.SetSessionTitle(sessionID, title)
}

// --- Message adapter ---

type sqliteMessages struct{ db *state.SessionDB }

func (sm *sqliteMessages) Append(_ context.Context, _, sessionID string, msg *store.Message) (int64, error) {
	var toolCalls []map[string]any
	if msg.ToolCalls != "" {
		json.Unmarshal([]byte(msg.ToolCalls), &toolCalls)
	}
	return sm.db.AppendMessage(sessionID, msg.Role, msg.Content, msg.ToolCallID, msg.ToolName, toolCalls, msg.Reasoning)
}

func (sm *sqliteMessages) List(_ context.Context, _, sessionID string, limit, _ int) ([]*store.Message, error) {
	raws, err := sm.db.GetMessages(sessionID)
	if err != nil {
		return nil, err
	}
	var msgs []*store.Message
	for _, raw := range raws {
		msgs = append(msgs, rawToMessage(raw))
	}
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return msgs, nil
}

func (sm *sqliteMessages) Search(_ context.Context, _, query string, limit int) ([]*store.SearchResult, error) {
	raws, err := sm.db.SearchMessages(query, limit)
	if err != nil {
		return nil, err
	}
	var results []*store.SearchResult
	for _, raw := range raws {
		results = append(results, &store.SearchResult{
			SessionID: strVal(raw, "session_id"),
			Content:   strVal(raw, "content"),
		})
	}
	return results, nil
}

func (sm *sqliteMessages) CountBySession(_ context.Context, _, sessionID string) (int, error) {
	msgs, err := sm.db.GetMessages(sessionID)
	return len(msgs), err
}

// --- User adapter (stub for SQLite — no user table in current schema) ---

type sqliteUsers struct{}

func (su *sqliteUsers) GetOrCreate(_ context.Context, _, externalID, username string) (*store.User, error) {
	return &store.User{ID: externalID, ExternalID: externalID, Username: username, Role: "user"}, nil
}
func (su *sqliteUsers) IsApproved(_ context.Context, _, _, _ string) (bool, error) { return true, nil }
func (su *sqliteUsers) Approve(_ context.Context, _, _, _ string) error            { return nil }
func (su *sqliteUsers) Revoke(_ context.Context, _, _, _ string) error             { return nil }
func (su *sqliteUsers) ListApproved(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

// --- helpers ---

func rawToSession(raw map[string]any) *store.Session {
	s := &store.Session{
		ID:       strVal(raw, "id"),
		Platform: strVal(raw, "source"),
		Model:    strVal(raw, "model"),
		Title:    strVal(raw, "title"),
	}
	if ts, ok := raw["started_at"].(float64); ok {
		s.StartedAt = time.Unix(int64(ts), 0)
	}
	if mc, ok := raw["message_count"].(int64); ok {
		s.MessageCount = int(mc)
	}
	if it, ok := raw["input_tokens"].(int64); ok {
		s.InputTokens = int(it)
	}
	if ot, ok := raw["output_tokens"].(int64); ok {
		s.OutputTokens = int(ot)
	}
	return s
}

func rawToMessage(raw map[string]any) *store.Message {
	m := &store.Message{
		Role:       strVal(raw, "role"),
		Content:    strVal(raw, "content"),
		ToolCallID: strVal(raw, "tool_call_id"),
		ToolName:   strVal(raw, "tool_name"),
		Reasoning:  strVal(raw, "reasoning"),
	}
	if ts, ok := raw["timestamp"].(float64); ok {
		m.Timestamp = time.Unix(int64(ts), 0)
	}
	if id, ok := raw["id"].(int64); ok {
		m.ID = id
	}
	return m
}

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
