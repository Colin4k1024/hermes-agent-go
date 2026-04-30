package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	cleanupLockID    int64         = 0x48455232 // "HER2"
	defaultRetention time.Duration = 7 * 24 * time.Hour
	defaultInterval  time.Duration = 1 * time.Hour
)

var allowedCascadeTables = map[string]struct{}{
	"messages":      {},
	"sessions":      {},
	"memories":      {},
	"user_profiles": {},
	"api_keys":      {},
	"cron_jobs":     {},
	"users":         {},
	"audit_logs":    {},
}

// TenantCleanupJob purges soft-deleted tenants after the retention window.
type TenantCleanupJob struct {
	pool      *pgxpool.Pool
	tenants   store.TenantStore
	retention time.Duration
	interval  time.Duration
}

type CleanupOption func(*TenantCleanupJob)

func WithRetention(d time.Duration) CleanupOption {
	return func(j *TenantCleanupJob) { j.retention = d }
}

func WithInterval(d time.Duration) CleanupOption {
	return func(j *TenantCleanupJob) { j.interval = d }
}

func NewTenantCleanupJob(pool *pgxpool.Pool, tenants store.TenantStore, opts ...CleanupOption) *TenantCleanupJob {
	j := &TenantCleanupJob{
		pool:      pool,
		tenants:   tenants,
		retention: defaultRetention,
		interval:  defaultInterval,
	}
	for _, o := range opts {
		o(j)
	}
	return j
}

// Run starts the background loop. Blocks until ctx is cancelled.
func (j *TenantCleanupJob) Run(ctx context.Context) {
	slog.Info("tenant_cleanup_started", "retention", j.retention, "interval", j.interval)
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	// Run once immediately.
	j.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("tenant_cleanup_stopped")
			return
		case <-ticker.C:
			j.tick(ctx)
		}
	}
}

func (j *TenantCleanupJob) tick(ctx context.Context) {
	conn, err := j.pool.Acquire(ctx)
	if err != nil {
		slog.Warn("cleanup_acquire_conn_failed", "error", err)
		return
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, cleanupLockID).Scan(&locked); err != nil {
		slog.Warn("cleanup_lock_failed", "error", err)
		return
	}
	if !locked {
		slog.Debug("cleanup_skipped_lock_held")
		return
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, cleanupLockID) //nolint:errcheck

	cutoff := time.Now().Add(-j.retention)
	tenants, err := j.tenants.ListDeleted(ctx, cutoff)
	if err != nil {
		slog.Error("cleanup_list_failed", "error", err)
		return
	}
	if len(tenants) == 0 {
		return
	}

	slog.Info("cleanup_purging", "count", len(tenants))
	for _, t := range tenants {
		if err := j.purgeTenant(ctx, t.ID); err != nil {
			slog.Error("cleanup_purge_failed", "tenant", t.ID, "error", err)
			continue
		}
		slog.Info("cleanup_purged", "tenant", t.ID, "name", t.Name)
	}
}

// purgeTenant deletes all data for a tenant in FK-safe order within a single transaction.
func (j *TenantCleanupJob) purgeTenant(ctx context.Context, tenantID string) error {
	tx, err := j.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	cascadeTables := []string{
		"messages",
		"sessions",
		"memories",
		"user_profiles",
		"api_keys",
		"cron_jobs",
		"users",
		"audit_logs",
	}
	for _, table := range cascadeTables {
		if _, ok := allowedCascadeTables[table]; !ok {
			return fmt.Errorf("delete %s: table not in allowlist", table)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE tenant_id = $1`, table), tenantID); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID); err != nil {
		return fmt.Errorf("hard delete tenant: %w", err)
	}

	return tx.Commit(ctx)
}
