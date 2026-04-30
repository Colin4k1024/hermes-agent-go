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

// --- MemoryStore no-op ---

type noopMemoryStore struct{}

func (n *noopMemoryStore) Get(_ context.Context, _, _, _ string) (string, error) {
	return "", errSQLiteUnsupported
}
func (n *noopMemoryStore) List(_ context.Context, _, _ string) ([]store.MemoryEntry, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopMemoryStore) Upsert(_ context.Context, _, _, _, _ string) error {
	return errSQLiteUnsupported
}
func (n *noopMemoryStore) Delete(_ context.Context, _, _, _ string) error {
	return errSQLiteUnsupported
}
func (n *noopMemoryStore) DeleteAllByUser(_ context.Context, _, _ string) (int64, error) {
	return 0, errSQLiteUnsupported
}
func (n *noopMemoryStore) DeleteAllByTenant(_ context.Context, _ string) (int64, error) {
	return 0, errSQLiteUnsupported
}

var _ store.MemoryStore = (*noopMemoryStore)(nil)

// --- UserProfileStore no-op ---

type noopUserProfileStore struct{}

func (n *noopUserProfileStore) Get(_ context.Context, _, _ string) (string, error) {
	return "", errSQLiteUnsupported
}
func (n *noopUserProfileStore) Upsert(_ context.Context, _, _, _ string) error {
	return errSQLiteUnsupported
}
func (n *noopUserProfileStore) Delete(_ context.Context, _, _ string) error {
	return errSQLiteUnsupported
}
func (n *noopUserProfileStore) DeleteAllByTenant(_ context.Context, _ string) (int64, error) {
	return 0, errSQLiteUnsupported
}

var _ store.UserProfileStore = (*noopUserProfileStore)(nil)

// --- CronJobStore no-op ---

type noopCronJobStore struct{}

func (n *noopCronJobStore) Create(_ context.Context, _ *store.CronJob) error {
	return errSQLiteUnsupported
}
func (n *noopCronJobStore) Get(_ context.Context, _, _ string) (*store.CronJob, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopCronJobStore) Update(_ context.Context, _ *store.CronJob) error {
	return errSQLiteUnsupported
}
func (n *noopCronJobStore) Delete(_ context.Context, _, _ string) error {
	return errSQLiteUnsupported
}
func (n *noopCronJobStore) List(_ context.Context, _ string) ([]*store.CronJob, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopCronJobStore) ListDue(_ context.Context, _ time.Time) ([]*store.CronJob, error) {
	return nil, errSQLiteUnsupported
}

var _ store.CronJobStore = (*noopCronJobStore)(nil)

// --- RoleStore no-op ---

type noopRoleStore struct{}

func (n *noopRoleStore) Create(_ context.Context, _ *store.Role) error { return errSQLiteUnsupported }
func (n *noopRoleStore) Get(_ context.Context, _, _ string) (*store.Role, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopRoleStore) GetByName(_ context.Context, _, _ string) (*store.Role, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopRoleStore) List(_ context.Context, _ string) ([]*store.Role, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopRoleStore) Delete(_ context.Context, _, _ string) error { return errSQLiteUnsupported }
func (n *noopRoleStore) AddPermission(_ context.Context, _, _, _, _ string) error {
	return errSQLiteUnsupported
}
func (n *noopRoleStore) RemovePermission(_ context.Context, _, _, _, _ string) error {
	return errSQLiteUnsupported
}
func (n *noopRoleStore) ListPermissions(_ context.Context, _, _ string) ([]*store.RolePermission, error) {
	return nil, errSQLiteUnsupported
}
func (n *noopRoleStore) HasPermission(_ context.Context, _ []string, _, _, _ string) (bool, error) {
	return false, errSQLiteUnsupported
}

var _ store.RoleStore = (*noopRoleStore)(nil)
