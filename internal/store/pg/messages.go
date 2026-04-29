package pg

import (
	"context"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgMessageStore struct{ pool *pgxpool.Pool }

func (m *pgMessageStore) Append(ctx context.Context, tenantID, sessionID string, msg *store.Message) (int64, error) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	// tool_calls is jsonb — pass nil if empty to avoid "invalid input syntax for type json".
	var toolCalls any = nil
	if msg.ToolCalls != "" {
		toolCalls = []byte(msg.ToolCalls)
	}
	var id int64
	err := m.pool.QueryRow(ctx, `
		INSERT INTO messages (tenant_id, session_id, role, content, tool_call_id, tool_calls, tool_name, reasoning, timestamp, token_count, finish_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		tenantID, sessionID, msg.Role, msg.Content,
		nullStr(msg.ToolCallID),
		toolCalls,
		nullStr(msg.ToolName),
		nullStr(msg.Reasoning),
		msg.Timestamp,
		nullInt(msg.TokenCount),
		nullStr(msg.FinishReason)).Scan(&id)
	return id, err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func (m *pgMessageStore) List(ctx context.Context, tenantID, sessionID string, limit, offset int) ([]*store.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.pool.Query(ctx, `
		SELECT id, tenant_id, session_id, role, content, tool_call_id, tool_calls, tool_name,
		       reasoning, timestamp, token_count, finish_reason
		FROM messages WHERE tenant_id = $1 AND session_id = $2
		ORDER BY timestamp ASC LIMIT $3 OFFSET $4`, tenantID, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*store.Message
	for rows.Next() {
		msg := &store.Message{}
		var toolCallID, toolName, reasoning, finishReason any
		rows.Scan(&msg.ID, &msg.TenantID, &msg.SessionID, &msg.Role, &msg.Content,
			&toolCallID, &msg.ToolCalls, &toolName, &reasoning,
			&msg.Timestamp, &msg.TokenCount, &finishReason)
		if toolCallID != nil {
			if v, ok := toolCallID.(string); ok {
				msg.ToolCallID = v
			}
		}
		if toolName != nil {
			if v, ok := toolName.(string); ok {
				msg.ToolName = v
			}
		}
		if reasoning != nil {
			if v, ok := reasoning.(string); ok {
				msg.Reasoning = v
			}
		}
		if finishReason != nil {
			if v, ok := finishReason.(string); ok {
				msg.FinishReason = v
			}
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func (m *pgMessageStore) Search(ctx context.Context, tenantID, query string, limit int) ([]*store.SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := m.pool.Query(ctx, `
		SELECT session_id, id, content,
		       ts_headline('english', content, plainto_tsquery('english', $2)) AS snippet,
		       ts_rank(to_tsvector('english', content), plainto_tsquery('english', $2)) AS rank
		FROM messages
		WHERE tenant_id = $1 AND to_tsvector('english', content) @@ plainto_tsquery('english', $2)
		ORDER BY rank DESC LIMIT $3`, tenantID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*store.SearchResult
	for rows.Next() {
		r := &store.SearchResult{}
		rows.Scan(&r.SessionID, &r.MessageID, &r.Content, &r.Snippet, &r.Rank)
		results = append(results, r)
	}
	return results, nil
}

func (m *pgMessageStore) CountBySession(ctx context.Context, tenantID, sessionID string) (int, error) {
	var count int
	err := m.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM messages WHERE tenant_id = $1 AND session_id = $2`,
		tenantID, sessionID).Scan(&count)
	return count, err
}
