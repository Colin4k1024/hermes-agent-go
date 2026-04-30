package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgTenantStore struct {
	pool *pgxpool.Pool
}

func (s *pgTenantStore) Create(ctx context.Context, t *store.Tenant) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO tenants (id, name, plan, rate_limit_rpm, max_sessions)
		 VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5)
		 RETURNING id, created_at, updated_at`,
		t.ID, t.Name, t.Plan, t.RateLimitRPM, t.MaxSessions,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create tenant: %w", err)
	}
	return nil
}

func (s *pgTenantStore) Get(ctx context.Context, id string) (*store.Tenant, error) {
	t := &store.Tenant{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, plan, rate_limit_rpm, max_sessions, created_at, updated_at
		 FROM tenants WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&t.ID, &t.Name, &t.Plan, &t.RateLimitRPM, &t.MaxSessions, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return t, nil
}

func (s *pgTenantStore) Update(ctx context.Context, t *store.Tenant) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tenants SET name=$2, plan=$3, rate_limit_rpm=$4, max_sessions=$5, updated_at=now()
		 WHERE id=$1 AND deleted_at IS NULL`,
		t.ID, t.Name, t.Plan, t.RateLimitRPM, t.MaxSessions,
	)
	if err != nil {
		return fmt.Errorf("update tenant: %w", err)
	}
	return nil
}

// Delete performs a soft delete by setting deleted_at.
func (s *pgTenantStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tenants SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("soft delete tenant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found or already deleted")
	}
	return nil
}

func (s *pgTenantStore) List(ctx context.Context, opts store.ListOptions) ([]*store.Tenant, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tenants WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tenants: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, plan, rate_limit_rpm, max_sessions, created_at, updated_at
		 FROM tenants WHERE deleted_at IS NULL
		 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, opts.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*store.Tenant
	for rows.Next() {
		t := &store.Tenant{}
		if err := rows.Scan(&t.ID, &t.Name, &t.Plan, &t.RateLimitRPM, &t.MaxSessions, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate tenants: %w", err)
	}
	return tenants, total, nil
}

// ListDeleted returns soft-deleted tenants older than olderThan (for async cleanup).
func (s *pgTenantStore) ListDeleted(ctx context.Context, olderThan time.Time) ([]*store.Tenant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, plan, rate_limit_rpm, max_sessions, created_at, updated_at, deleted_at
		 FROM tenants WHERE deleted_at IS NOT NULL AND deleted_at < $1
		 ORDER BY deleted_at ASC`, olderThan,
	)
	if err != nil {
		return nil, fmt.Errorf("list deleted tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*store.Tenant
	for rows.Next() {
		t := &store.Tenant{}
		if err := rows.Scan(&t.ID, &t.Name, &t.Plan, &t.RateLimitRPM, &t.MaxSessions, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan deleted tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// HardDelete permanently removes a tenant row (used by cleanup job after retention period).
func (s *pgTenantStore) HardDelete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("hard delete tenant: %w", err)
	}
	return nil
}

// Restore clears deleted_at to un-delete a tenant within the retention window.
func (s *pgTenantStore) Restore(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tenants SET deleted_at = NULL WHERE id = $1 AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("restore tenant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found or not deleted")
	}
	return nil
}

var _ store.TenantStore = (*pgTenantStore)(nil)
