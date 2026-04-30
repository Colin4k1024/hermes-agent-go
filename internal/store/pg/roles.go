package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgRoleStore struct {
	pool *pgxpool.Pool
}

func (s *pgRoleStore) Create(ctx context.Context, role *store.Role) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, description, is_system)
		 VALUES ($1, $2, $3, $4, $5)`,
		role.ID, role.TenantID, role.Name, role.Description, role.IsSystem)
	if err != nil {
		return fmt.Errorf("pg create role: %w", err)
	}
	return nil
}

func (s *pgRoleStore) Get(ctx context.Context, tenantID, roleID string) (*store.Role, error) {
	var r store.Role
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, COALESCE(description,''), is_system, created_at
		 FROM roles WHERE tenant_id = $1 AND id = $2`,
		tenantID, roleID).Scan(&r.ID, &r.TenantID, &r.Name, &r.Description, &r.IsSystem, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("role not found")
		}
		return nil, fmt.Errorf("pg get role: %w", err)
	}
	return &r, nil
}

func (s *pgRoleStore) GetByName(ctx context.Context, tenantID, name string) (*store.Role, error) {
	var r store.Role
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, COALESCE(description,''), is_system, created_at
		 FROM roles WHERE tenant_id = $1 AND name = $2`,
		tenantID, name).Scan(&r.ID, &r.TenantID, &r.Name, &r.Description, &r.IsSystem, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("role %q not found", name)
		}
		return nil, fmt.Errorf("pg get role by name: %w", err)
	}
	return &r, nil
}

func (s *pgRoleStore) List(ctx context.Context, tenantID string) ([]*store.Role, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, COALESCE(description,''), is_system, created_at
		 FROM roles WHERE tenant_id = $1 ORDER BY name`,
		tenantID)
	if err != nil {
		return nil, fmt.Errorf("pg list roles: %w", err)
	}
	defer rows.Close()

	var roles []*store.Role
	for rows.Next() {
		var r store.Role
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Description, &r.IsSystem, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("pg scan role: %w", err)
		}
		roles = append(roles, &r)
	}
	return roles, rows.Err()
}

func (s *pgRoleStore) Delete(ctx context.Context, tenantID, roleID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM roles WHERE tenant_id = $1 AND id = $2 AND is_system = false`,
		tenantID, roleID)
	if err != nil {
		return fmt.Errorf("pg delete role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("role not found or is a system role")
	}
	return nil
}

func (s *pgRoleStore) AddPermission(ctx context.Context, tenantID, roleName, resource, action string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO role_permissions (role_id, resource, action)
		 SELECT r.id, $3, $4 FROM roles r WHERE r.tenant_id = $1 AND r.name = $2
		 ON CONFLICT (role_id, resource, action) DO NOTHING`,
		tenantID, roleName, resource, action)
	if err != nil {
		return fmt.Errorf("pg add permission: %w", err)
	}
	return nil
}

func (s *pgRoleStore) RemovePermission(ctx context.Context, tenantID, roleName, resource, action string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM role_permissions
		 WHERE role_id = (SELECT id FROM roles WHERE tenant_id = $1 AND name = $2)
		   AND resource = $3 AND action = $4`,
		tenantID, roleName, resource, action)
	if err != nil {
		return fmt.Errorf("pg remove permission: %w", err)
	}
	return nil
}

func (s *pgRoleStore) ListPermissions(ctx context.Context, tenantID, roleName string) ([]*store.RolePermission, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT rp.id, rp.role_id, rp.resource, rp.action, rp.created_at
		 FROM role_permissions rp
		 JOIN roles r ON rp.role_id = r.id
		 WHERE r.tenant_id = $1 AND r.name = $2
		 ORDER BY rp.resource, rp.action`,
		tenantID, roleName)
	if err != nil {
		return nil, fmt.Errorf("pg list permissions: %w", err)
	}
	defer rows.Close()

	var perms []*store.RolePermission
	for rows.Next() {
		var p store.RolePermission
		if err := rows.Scan(&p.ID, &p.RoleID, &p.Resource, &p.Action, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("pg scan permission: %w", err)
		}
		perms = append(perms, &p)
	}
	return perms, rows.Err()
}

func (s *pgRoleStore) HasPermission(ctx context.Context, roles []string, tenantID, resource, action string) (bool, error) {
	if len(roles) == 0 {
		return false, nil
	}
	for _, r := range roles {
		if r == "admin" {
			return true, nil
		}
	}

	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM role_permissions rp
			JOIN roles r ON rp.role_id = r.id
			WHERE r.tenant_id = $1 AND r.name = ANY($2)
			  AND rp.resource = $3 AND rp.action = $4
		)`, tenantID, roles, resource, action).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("pg has permission: %w", err)
	}
	return exists, nil
}

var _ store.RoleStore = (*pgRoleStore)(nil)
