package pg

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgUserStore struct{ pool *pgxpool.Pool }

func (u *pgUserStore) GetOrCreate(ctx context.Context, tenantID, externalID, username string) (*store.User, error) {
	user := &store.User{}
	err := u.pool.QueryRow(ctx,
		`SELECT id, tenant_id, external_id, username, display_name, role, approved_at
		 FROM users WHERE tenant_id = $1 AND external_id = $2`, tenantID, externalID).
		Scan(&user.ID, &user.TenantID, &user.ExternalID, &user.Username, &user.DisplayName, &user.Role, &user.ApprovedAt)

	if err == nil {
		return user, nil
	}

	user = &store.User{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		ExternalID: externalID,
		Username:   username,
		Role:       "user",
	}
	_, err = u.pool.Exec(ctx,
		`INSERT INTO users (id, tenant_id, external_id, username, role) VALUES ($1, $2, $3, $4, $5)`,
		user.ID, tenantID, externalID, username, user.Role)
	return user, err
}

func (u *pgUserStore) IsApproved(ctx context.Context, tenantID, platform, userID string) (bool, error) {
	externalID := platform + ":" + userID
	var approvedAt *time.Time
	err := u.pool.QueryRow(ctx,
		`SELECT approved_at FROM users WHERE tenant_id = $1 AND external_id = $2`,
		tenantID, externalID).Scan(&approvedAt)
	if err != nil {
		return false, nil
	}
	return approvedAt != nil, nil
}

func (u *pgUserStore) Approve(ctx context.Context, tenantID, platform, userID string) error {
	externalID := platform + ":" + userID
	_, err := u.pool.Exec(ctx,
		`UPDATE users SET approved_at = $1 WHERE tenant_id = $2 AND external_id = $3`,
		time.Now(), tenantID, externalID)
	return err
}

func (u *pgUserStore) Revoke(ctx context.Context, tenantID, platform, userID string) error {
	externalID := platform + ":" + userID
	_, err := u.pool.Exec(ctx,
		`UPDATE users SET approved_at = NULL WHERE tenant_id = $1 AND external_id = $2`,
		tenantID, externalID)
	return err
}

func (u *pgUserStore) ListApproved(ctx context.Context, tenantID, platform string) ([]string, error) {
	prefix := platform + ":"
	rows, err := u.pool.Query(ctx,
		`SELECT external_id FROM users WHERE tenant_id = $1 AND external_id LIKE $2 AND approved_at IS NOT NULL`,
		tenantID, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var eid string
		rows.Scan(&eid)
		ids = append(ids, eid[len(prefix):])
	}
	return ids, nil
}
