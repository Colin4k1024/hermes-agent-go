package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgMemoryStore struct {
	pool *pgxpool.Pool
}

func (s *pgMemoryStore) Get(ctx context.Context, tenantID, userID, key string) (string, error) {
	var content string
	err := s.pool.QueryRow(ctx,
		`SELECT content FROM memories WHERE tenant_id = $1 AND user_id = $2 AND key = $3`,
		tenantID, userID, key).Scan(&content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("pg get memory: %w", err)
	}
	return content, nil
}

func (s *pgMemoryStore) List(ctx context.Context, tenantID, userID string) ([]store.MemoryEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT tenant_id, user_id, key, content, updated_at FROM memories
		 WHERE tenant_id = $1 AND user_id = $2 ORDER BY updated_at DESC`,
		tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("pg list memories: %w", err)
	}
	defer rows.Close()

	var entries []store.MemoryEntry
	for rows.Next() {
		var e store.MemoryEntry
		if err := rows.Scan(&e.TenantID, &e.UserID, &e.Key, &e.Content, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("pg scan memory: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *pgMemoryStore) Upsert(ctx context.Context, tenantID, userID, key, content string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO memories (tenant_id, user_id, key, content, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (tenant_id, user_id, key)
		 DO UPDATE SET content = $4, updated_at = now()`,
		tenantID, userID, key, content)
	if err != nil {
		return fmt.Errorf("pg upsert memory: %w", err)
	}
	return nil
}

func (s *pgMemoryStore) Delete(ctx context.Context, tenantID, userID, key string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM memories WHERE tenant_id = $1 AND user_id = $2 AND key = $3`,
		tenantID, userID, key)
	if err != nil {
		return fmt.Errorf("pg delete memory: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory key %q not found", key)
	}
	return nil
}

func (s *pgMemoryStore) DeleteAllByUser(ctx context.Context, tenantID, userID string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM memories WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID)
	if err != nil {
		return 0, fmt.Errorf("pg delete user memories: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *pgMemoryStore) DeleteAllByTenant(ctx context.Context, tenantID string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM memories WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return 0, fmt.Errorf("pg delete tenant memories: %w", err)
	}
	return tag.RowsAffected(), nil
}

var _ store.MemoryStore = (*pgMemoryStore)(nil)
