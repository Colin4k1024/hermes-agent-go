package pg

import (
	"context"
	"fmt"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgAPIKeyStore struct {
	pool *pgxpool.Pool
}

func (s *pgAPIKeyStore) Create(ctx context.Context, key *store.APIKey) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO api_keys (id, tenant_id, name, key_hash, prefix, roles, expires_at) VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7)`,
		key.ID, key.TenantID, key.Name, key.KeyHash, key.Prefix, key.Roles, key.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

func (s *pgAPIKeyStore) GetByHash(ctx context.Context, hash string) (*store.APIKey, error) {
	k := &store.APIKey{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, key_hash, prefix, roles, expires_at, revoked_at, created_at FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())`,
		hash,
	).Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyHash, &k.Prefix, &k.Roles, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}
	return k, nil
}

func (s *pgAPIKeyStore) GetByID(ctx context.Context, tenantID, id string) (*store.APIKey, error) {
	k := &store.APIKey{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, key_hash, prefix, roles, expires_at, revoked_at, created_at FROM api_keys WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	).Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyHash, &k.Prefix, &k.Roles, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get api key by id: %w", err)
	}
	return k, nil
}

func (s *pgAPIKeyStore) List(ctx context.Context, tenantID string) ([]*store.APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, prefix, roles, expires_at, revoked_at, created_at FROM api_keys WHERE tenant_id = $1 ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []*store.APIKey
	for rows.Next() {
		k := &store.APIKey{}
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.Prefix, &k.Roles, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api keys: %w", err)
	}
	return keys, nil
}

func (s *pgAPIKeyStore) Revoke(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE api_keys SET revoked_at = now() WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	return nil
}

var _ store.APIKeyStore = (*pgAPIKeyStore)(nil)
