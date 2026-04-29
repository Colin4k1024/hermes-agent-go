package pg

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgAuditLogStore struct {
	pool *pgxpool.Pool
}

func (s *pgAuditLogStore) Append(ctx context.Context, log *store.AuditLog) error {
	// Pass NULL for user_id if it's empty (e.g., static-token auth uses "static-user" string, not UUID).
	var userID pgtype.UUID
	if log.UserID != "" {
		userID.Valid = true
		userID.Bytes = uuid.MustParse(log.UserID)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_logs (tenant_id, user_id, session_id, action, detail, request_id, status_code, latency_ms) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		log.TenantID, userID, log.SessionID, log.Action, log.Detail, log.RequestID, log.StatusCode, log.LatencyMs,
	)
	if err != nil {
		return fmt.Errorf("append audit log: %w", err)
	}
	return nil
}

func (s *pgAuditLogStore) List(ctx context.Context, tenantID string, opts store.AuditListOptions) ([]*store.AuditLog, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, tenant_id, user_id, session_id, action, detail, request_id, status_code, latency_ms, created_at FROM audit_logs WHERE tenant_id = $1`
	countQuery := `SELECT COUNT(*) FROM audit_logs WHERE tenant_id = $1`
	args := []any{tenantID}
	argIdx := 2

	if opts.Action != "" {
		query += fmt.Sprintf(` AND action = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND action = $%d`, argIdx)
		args = append(args, opts.Action)
		argIdx++
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, opts.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*store.AuditLog
	for rows.Next() {
		l := &store.AuditLog{}
		if err := rows.Scan(&l.ID, &l.TenantID, &l.UserID, &l.SessionID, &l.Action, &l.Detail, &l.RequestID, &l.StatusCode, &l.LatencyMs, &l.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan audit log: %w", err)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate audit logs: %w", err)
	}
	return logs, total, nil
}

var _ store.AuditLogStore = (*pgAuditLogStore)(nil)
