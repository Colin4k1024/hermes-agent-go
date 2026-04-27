package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgSessionStore struct{ pool *pgxpool.Pool }

func (s *pgSessionStore) Create(ctx context.Context, tenantID string, sess *store.Session) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, tenant_id, platform, user_id, model, system_prompt, parent_session_id, title, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		sess.ID, tenantID, sess.Platform, sess.UserID, sess.Model,
		sess.SystemPrompt, sess.ParentSessionID, sess.Title, sess.StartedAt)
	return err
}

func (s *pgSessionStore) Get(ctx context.Context, tenantID, sessionID string) (*store.Session, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, platform, user_id, model, system_prompt, parent_session_id,
		       title, started_at, ended_at, end_reason, message_count, tool_call_count,
		       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, estimated_cost_usd
		FROM sessions WHERE tenant_id = $1 AND id = $2`, tenantID, sessionID)

	sess := &store.Session{}
	err := row.Scan(
		&sess.ID, &sess.TenantID, &sess.Platform, &sess.UserID, &sess.Model,
		&sess.SystemPrompt, &sess.ParentSessionID, &sess.Title, &sess.StartedAt,
		&sess.EndedAt, &sess.EndReason, &sess.MessageCount, &sess.ToolCallCount,
		&sess.InputTokens, &sess.OutputTokens, &sess.CacheReadTokens, &sess.CacheWriteTokens,
		&sess.EstimatedCostUSD)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	return sess, nil
}

func (s *pgSessionStore) End(ctx context.Context, tenantID, sessionID, reason string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET ended_at = $1, end_reason = $2 WHERE tenant_id = $3 AND id = $4`,
		time.Now(), reason, tenantID, sessionID)
	return err
}

func (s *pgSessionStore) List(ctx context.Context, tenantID string, opts store.ListOptions) ([]*store.Session, int, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	countRow := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE tenant_id = $1`, tenantID)
	var total int
	countRow.Scan(&total)

	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, platform, user_id, model, title, started_at, ended_at,
		       message_count, input_tokens, output_tokens, estimated_cost_usd
		FROM sessions WHERE tenant_id = $1
		ORDER BY started_at DESC LIMIT $2 OFFSET $3`, tenantID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sessions []*store.Session
	for rows.Next() {
		s := &store.Session{}
		rows.Scan(&s.ID, &s.TenantID, &s.Platform, &s.UserID, &s.Model, &s.Title,
			&s.StartedAt, &s.EndedAt, &s.MessageCount, &s.InputTokens, &s.OutputTokens,
			&s.EstimatedCostUSD)
		sessions = append(sessions, s)
	}

	return sessions, total, nil
}

func (s *pgSessionStore) Delete(ctx context.Context, tenantID, sessionID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM messages WHERE tenant_id = $1 AND session_id = $2`, tenantID, sessionID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM sessions WHERE tenant_id = $1 AND id = $2`, tenantID, sessionID)
	return err
}

func (s *pgSessionStore) UpdateTokens(ctx context.Context, tenantID, sessionID string, delta store.TokenDelta) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions SET
			input_tokens = input_tokens + $1,
			output_tokens = output_tokens + $2,
			cache_read_tokens = cache_read_tokens + $3,
			cache_write_tokens = cache_write_tokens + $4,
			message_count = message_count + 1
		WHERE tenant_id = $5 AND id = $6`,
		delta.Input, delta.Output, delta.CacheRead, delta.CacheWrite, tenantID, sessionID)
	return err
}

func (s *pgSessionStore) SetTitle(ctx context.Context, tenantID, sessionID, title string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET title = $1 WHERE tenant_id = $2 AND id = $3`,
		title, tenantID, sessionID)
	return err
}
