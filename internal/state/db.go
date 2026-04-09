package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	_ "modernc.org/sqlite"
)

// SessionDB manages session and message persistence in SQLite.
type SessionDB struct {
	mu sync.Mutex
	db *sql.DB
}

// NewSessionDB opens or creates the session database.
func NewSessionDB(dbPath string) (*SessionDB, error) {
	if dbPath == "" {
		dbPath = filepath.Join(config.HermesHome(), "state.db")
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sdb := &SessionDB{db: db}
	if err := sdb.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return sdb, nil
}

func (s *SessionDB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			user_id TEXT,
			model TEXT,
			model_config TEXT,
			system_prompt TEXT,
			parent_session_id TEXT,
			started_at REAL NOT NULL,
			ended_at REAL,
			end_reason TEXT,
			message_count INTEGER DEFAULT 0,
			tool_call_count INTEGER DEFAULT 0,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_read_tokens INTEGER DEFAULT 0,
			cache_write_tokens INTEGER DEFAULT 0,
			reasoning_tokens INTEGER DEFAULT 0,
			billing_provider TEXT,
			billing_base_url TEXT,
			billing_mode TEXT,
			estimated_cost_usd REAL,
			actual_cost_usd REAL,
			cost_status TEXT,
			cost_source TEXT,
			pricing_version TEXT,
			title TEXT,
			FOREIGN KEY (parent_session_id) REFERENCES sessions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			role TEXT NOT NULL,
			content TEXT,
			tool_call_id TEXT,
			tool_calls TEXT,
			tool_name TEXT,
			timestamp REAL NOT NULL,
			token_count INTEGER,
			finish_reason TEXT,
			reasoning TEXT,
			reasoning_details TEXT,
			codex_reasoning_items TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_source ON sessions(source)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_title ON sessions(title)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_parent ON sessions(parent_session_id)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Create FTS5 table if supported
	_, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
		content,
		content=messages,
		content_rowid=id
	)`)
	if err != nil {
		slog.Debug("FTS5 not available", "error", err)
	}

	return nil
}

// CreateSession creates a new session.
func (s *SessionDB) CreateSession(id, source, model string, parentSessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO sessions (id, source, model, parent_session_id, started_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, source, model, nullStr(parentSessionID), float64(time.Now().UnixMilli())/1000.0,
	)
	return err
}

// EndSession marks a session as ended.
func (s *SessionDB) EndSession(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`UPDATE sessions SET ended_at = ?, end_reason = ? WHERE id = ?`,
		float64(time.Now().UnixMilli())/1000.0, reason, id,
	)
	return err
}

// AppendMessage adds a message to a session.
func (s *SessionDB) AppendMessage(sessionID, role, content string, toolCallID, toolName string, toolCalls []map[string]any, reasoning string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var toolCallsJSON *string
	if toolCalls != nil {
		b, _ := json.Marshal(toolCalls)
		str := string(b)
		toolCallsJSON = &str
	}

	result, err := s.db.Exec(
		`INSERT INTO messages (session_id, role, content, tool_call_id, tool_name, tool_calls, timestamp, reasoning)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, role, nullStr(content), nullStr(toolCallID), nullStr(toolName),
		toolCallsJSON, float64(time.Now().UnixMilli())/1000.0, nullStr(reasoning),
	)
	if err != nil {
		return 0, err
	}

	// Update message count
	s.db.Exec(`UPDATE sessions SET message_count = message_count + 1 WHERE id = ?`, sessionID)
	if role == "tool" {
		s.db.Exec(`UPDATE sessions SET tool_call_count = tool_call_count + 1 WHERE id = ?`, sessionID)
	}

	// Update FTS index
	rowID, _ := result.LastInsertId()
	if content != "" {
		s.db.Exec(`INSERT INTO messages_fts(rowid, content) VALUES (?, ?)`, rowID, content)
	}

	return rowID, nil
}

// GetMessages loads all messages for a session.
func (s *SessionDB) GetMessages(sessionID string) ([]map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(
		`SELECT role, content, tool_call_id, tool_calls, tool_name, reasoning, finish_reason
		 FROM messages WHERE session_id = ? ORDER BY timestamp`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []map[string]any
	for rows.Next() {
		var role, content, toolCallID, toolCallsJSON, toolName, reasoning, finishReason sql.NullString
		if err := rows.Scan(&role, &content, &toolCallID, &toolCallsJSON, &toolName, &reasoning, &finishReason); err != nil {
			continue
		}

		msg := map[string]any{
			"role": role.String,
		}
		if content.Valid {
			msg["content"] = content.String
		}
		if toolCallID.Valid {
			msg["tool_call_id"] = toolCallID.String
		}
		if toolName.Valid {
			msg["tool_name"] = toolName.String
		}
		if toolCallsJSON.Valid {
			var tc []map[string]any
			json.Unmarshal([]byte(toolCallsJSON.String), &tc)
			msg["tool_calls"] = tc
		}
		if reasoning.Valid {
			msg["reasoning"] = reasoning.String
		}
		if finishReason.Valid {
			msg["finish_reason"] = finishReason.String
		}

		messages = append(messages, msg)
	}
	return messages, nil
}

// UpdateTokenCounts updates token accounting for a session.
func (s *SessionDB) UpdateTokenCounts(sessionID string, input, output, cacheRead, cacheWrite, reasoning int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`UPDATE sessions SET
			input_tokens = input_tokens + ?,
			output_tokens = output_tokens + ?,
			cache_read_tokens = cache_read_tokens + ?,
			cache_write_tokens = cache_write_tokens + ?,
			reasoning_tokens = reasoning_tokens + ?
		 WHERE id = ?`,
		input, output, cacheRead, cacheWrite, reasoning, sessionID,
	)
	return err
}

// SetSessionTitle sets or updates the session title.
func (s *SessionDB) SetSessionTitle(sessionID, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`UPDATE sessions SET title = ? WHERE id = ?`, title, sessionID)
	return err
}

// GetSessionTitle returns the title of a session.
func (s *SessionDB) GetSessionTitle(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var title sql.NullString
	s.db.QueryRow(`SELECT title FROM sessions WHERE id = ?`, sessionID).Scan(&title)
	return title.String
}

// ListSessions returns recent sessions with previews.
func (s *SessionDB) ListSessions(source string, limit, offset int) ([]map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT s.id, s.source, s.model, s.title, s.started_at, s.ended_at,
		s.message_count, s.input_tokens, s.output_tokens,
		(SELECT content FROM messages WHERE session_id = s.id AND role = 'user' ORDER BY timestamp LIMIT 1) as preview
		FROM sessions s`

	args := []any{}
	if source != "" {
		query += ` WHERE s.source = ?`
		args = append(args, source)
	}
	query += ` ORDER BY s.started_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []map[string]any
	for rows.Next() {
		var id, src, model, title, preview sql.NullString
		var startedAt, endedAt sql.NullFloat64
		var msgCount, inputTokens, outputTokens sql.NullInt64

		if err := rows.Scan(&id, &src, &model, &title, &startedAt, &endedAt, &msgCount, &inputTokens, &outputTokens, &preview); err != nil {
			continue
		}

		session := map[string]any{
			"id":            id.String,
			"source":        src.String,
			"model":         model.String,
			"title":         title.String,
			"started_at":    startedAt.Float64,
			"message_count": msgCount.Int64,
			"input_tokens":  inputTokens.Int64,
			"output_tokens": outputTokens.Int64,
		}
		if preview.Valid {
			p := preview.String
			if len(p) > 100 {
				p = p[:100] + "..."
			}
			session["preview"] = p
		}
		if endedAt.Valid {
			session["ended_at"] = endedAt.Float64
		}

		sessions = append(sessions, session)
	}
	return sessions, nil
}

// SearchMessages performs full-text search across messages.
func (s *SessionDB) SearchMessages(query string, limit int) ([]map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(
		`SELECT m.session_id, m.role, m.content, s.title, s.source
		 FROM messages_fts f
		 JOIN messages m ON m.id = f.rowid
		 JOIN sessions s ON s.id = m.session_id
		 WHERE messages_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var sessionID, role, content, title, source sql.NullString
		if err := rows.Scan(&sessionID, &role, &content, &title, &source); err != nil {
			continue
		}
		results = append(results, map[string]any{
			"session_id": sessionID.String,
			"role":       role.String,
			"content":    content.String,
			"title":      title.String,
			"source":     source.String,
		})
	}
	return results, nil
}

// DeleteSession removes a session and all its messages.
func (s *SessionDB) DeleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID)
	tx.Exec(`DELETE FROM sessions WHERE id = ?`, sessionID)

	return tx.Commit()
}

// Close closes the database connection.
func (s *SessionDB) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Best-effort WAL checkpoint
	s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return s.db.Close()
}

// GetSession returns session metadata.
func (s *SessionDB) GetSession(sessionID string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id, source, model, title, parentID sql.NullString
	var startedAt, endedAt sql.NullFloat64
	var msgCount, inputTokens, outputTokens sql.NullInt64

	err := s.db.QueryRow(
		`SELECT id, source, model, title, parent_session_id, started_at, ended_at,
		 message_count, input_tokens, output_tokens
		 FROM sessions WHERE id = ?`, sessionID,
	).Scan(&id, &source, &model, &title, &parentID, &startedAt, &endedAt, &msgCount, &inputTokens, &outputTokens)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"id":                id.String,
		"source":            source.String,
		"model":             model.String,
		"title":             title.String,
		"parent_session_id": parentID.String,
		"started_at":        startedAt.Float64,
		"message_count":     msgCount.Int64,
		"input_tokens":      inputTokens.Int64,
		"output_tokens":     outputTokens.Int64,
	}
	if endedAt.Valid {
		result["ended_at"] = endedAt.Float64
	}
	return result, nil
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
