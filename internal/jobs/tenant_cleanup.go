package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/objstore"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	cleanupLockID    int64         = 0x48455232 // "HER2"
	defaultRetention time.Duration = 7 * 24 * time.Hour
	defaultInterval  time.Duration = 1 * time.Hour
)

var allowedCascadeTables = map[string]struct{}{
	"role_permissions": {},
	"roles":            {},
	"messages":         {},
	"sessions":         {},
	"memories":         {},
	"user_profiles":    {},
	"api_keys":         {},
	"cron_jobs":        {},
	"users":            {},
	"audit_logs":       {},
}

// TenantCleanupJob purges soft-deleted tenants after the retention window.
type TenantCleanupJob struct {
	pool      *pgxpool.Pool
	tenants   store.TenantStore
	minio     *objstore.MinIOClient
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

func WithMinIO(mc *objstore.MinIOClient) CleanupOption {
	return func(j *TenantCleanupJob) { j.minio = mc }
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

// purgeTenant deletes all data for a tenant in FK-safe order, cleans MinIO, and records audit.
func (j *TenantCleanupJob) purgeTenant(ctx context.Context, tenantID string) error {
	start := time.Now()
	var totalRows int64

	tx, err := j.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// FK-safe cascade order: child tables first.
	cascadeTables := []string{
		"role_permissions",
		"roles",
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
		// role_permissions doesn't have tenant_id directly; delete via roles FK cascade.
		if table == "role_permissions" {
			tag, err := tx.Exec(ctx,
				`DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles WHERE tenant_id = $1)`, tenantID)
			if err != nil {
				return fmt.Errorf("delete %s: %w", table, err)
			}
			totalRows += tag.RowsAffected()
			continue
		}
		tag, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE tenant_id = $1`, table), tenantID)
		if err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
		totalRows += tag.RowsAffected()
	}

	if _, err := tx.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID); err != nil {
		return fmt.Errorf("hard delete tenant: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// MinIO cleanup (best-effort, after PG commit).
	minioDeleted := 0
	if j.minio != nil {
		prefixes := []string{tenantID + "/"}
		for _, prefix := range prefixes {
			objs, err := j.minio.ListObjects(ctx, prefix)
			if err != nil {
				slog.Warn("purge_minio_list_failed", "tenant", tenantID, "prefix", prefix, "error", err)
				continue
			}
			for _, obj := range objs {
				if err := j.minio.DeleteObject(ctx, obj); err != nil {
					slog.Warn("purge_minio_delete_failed", "tenant", tenantID, "object", obj, "error", err)
				} else {
					minioDeleted++
				}
			}
		}
	}

	// Record purge event in independent audit table (not subject to tenant RLS).
	durationMs := int(time.Since(start).Milliseconds())
	_, err = j.pool.Exec(ctx,
		`INSERT INTO purge_audit_logs (tenant_id, action, detail, rows_deleted, minio_objects_deleted, duration_ms)
		 VALUES ($1, 'tenant_purge', $2, $3, $4, $5)`,
		tenantID,
		fmt.Sprintf("purged %d rows from %d tables", totalRows, len(cascadeTables)+1),
		totalRows, minioDeleted, durationMs)
	if err != nil {
		slog.Warn("purge_audit_write_failed", "tenant", tenantID, "error", err)
	}

	return nil
}
