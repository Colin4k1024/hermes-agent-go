package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

var errSQLiteUnsupported = fmt.Errorf("not supported in SQLite mode — use PostgreSQL for SaaS features")

// --- TenantStore no-op ---

type noopTenantStore struct{}

func (n *noopTenantStore) Create(_ context.Context, _ *store.Tenant) error {
	return errSQLiteUnsupported
}
func (n *noopTenantStore) Get(_ context.Context, _ string) (*store.Tenant, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopTenantStore) Update(_ context.Context, _ *store.Tenant) error {
	return errSQLiteUnsupported
}
func (n *noopTenantStore) Delete(_ context.Context, _ string) error { return errSQLiteUnsupported }
func (n *noopTenantStore) List(_ context.Context, _ store.ListOptions) ([]*store.Tenant, int, error) {
	return nil, 0, errSQLiteUnsupported
}
func (n *noopTenantStore) ListDeleted(_ context.Context, _ time.Time) ([]*store.Tenant, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopTenantStore) HardDelete(_ context.Context, _ string) error {
	return errSQLiteUnsupported
}
func (n *noopTenantStore) Restore(_ context.Context, _ string) error { return errSQLiteUnsupported }

var _ store.TenantStore = (*noopTenantStore)(nil)

// --- AuditLogStore no-op ---

type noopAuditLogStore struct{}

func (n *noopAuditLogStore) Append(_ context.Context, _ *store.AuditLog) error {
	return errSQLiteUnsupported
}
func (n *noopAuditLogStore) List(_ context.Context, _ string, _ store.AuditListOptions) ([]*store.AuditLog, int, error) {
	return nil, 0, errSQLiteUnsupported
}
func (n *noopAuditLogStore) DeleteByTenant(_ context.Context, _ string) (int64, error) {
	return 0, errSQLiteUnsupported
}

var _ store.AuditLogStore = (*noopAuditLogStore)(nil)

// --- APIKeyStore no-op ---

type noopAPIKeyStore struct{}

func (n *noopAPIKeyStore) Create(_ context.Context, _ *store.APIKey) error {
	return errSQLiteUnsupported
}
func (n *noopAPIKeyStore) GetByHash(_ context.Context, _ string) (*store.APIKey, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopAPIKeyStore) GetByID(_ context.Context, _, _ string) (*store.APIKey, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopAPIKeyStore) List(_ context.Context, _ string) ([]*store.APIKey, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopAPIKeyStore) Revoke(_ context.Context, _, _ string) error { return errSQLiteUnsupported }

var _ store.APIKeyStore = (*noopAPIKeyStore)(nil)
