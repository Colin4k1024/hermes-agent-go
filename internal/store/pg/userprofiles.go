package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgUserProfileStore struct {
	pool *pgxpool.Pool
}

func (s *pgUserProfileStore) Get(ctx context.Context, tenantID, userID string) (string, error) {
	var content string
	err := s.pool.QueryRow(ctx,
		`SELECT content FROM user_profiles WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID).Scan(&content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("pg get user profile: %w", err)
	}
	return content, nil
}

func (s *pgUserProfileStore) Upsert(ctx context.Context, tenantID, userID, content string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_profiles (tenant_id, user_id, content, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (tenant_id, user_id)
		 DO UPDATE SET content = $3, updated_at = now()`,
		tenantID, userID, content)
	if err != nil {
		return fmt.Errorf("pg upsert user profile: %w", err)
	}
	return nil
}

func (s *pgUserProfileStore) Delete(ctx context.Context, tenantID, userID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM user_profiles WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID)
	if err != nil {
		return fmt.Errorf("pg delete user profile: %w", err)
	}
	return nil
}

func (s *pgUserProfileStore) DeleteAllByTenant(ctx context.Context, tenantID string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM user_profiles WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return 0, fmt.Errorf("pg delete tenant profiles: %w", err)
	}
	return tag.RowsAffected(), nil
}

var _ store.UserProfileStore = (*pgUserProfileStore)(nil)
