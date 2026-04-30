package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgCronJobStore struct {
	pool *pgxpool.Pool
}

func (s *pgCronJobStore) Create(ctx context.Context, job *store.CronJob) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO cron_jobs (id, tenant_id, name, prompt, schedule, deliver, enabled, model, next_run_at, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, COALESCE($10::jsonb, '{}'))`,
		job.ID, job.TenantID, job.Name, job.Prompt, job.Schedule,
		job.Deliver, job.Enabled, job.Model, job.NextRunAt, job.Metadata)
	if err != nil {
		return fmt.Errorf("pg create cron job: %w", err)
	}
	return nil
}

func (s *pgCronJobStore) Get(ctx context.Context, tenantID, jobID string) (*store.CronJob, error) {
	var j store.CronJob
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, prompt, schedule, COALESCE(deliver,''), enabled,
		        COALESCE(model,''), next_run_at, last_run_at, run_count, created_at, COALESCE(metadata::text,'{}')
		 FROM cron_jobs WHERE tenant_id = $1 AND id = $2`,
		tenantID, jobID).Scan(
		&j.ID, &j.TenantID, &j.Name, &j.Prompt, &j.Schedule, &j.Deliver, &j.Enabled,
		&j.Model, &j.NextRunAt, &j.LastRunAt, &j.RunCount, &j.CreatedAt, &j.Metadata)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("cron job not found")
		}
		return nil, fmt.Errorf("pg get cron job: %w", err)
	}
	return &j, nil
}

func (s *pgCronJobStore) Update(ctx context.Context, job *store.CronJob) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE cron_jobs SET name=$3, prompt=$4, schedule=$5, deliver=$6, enabled=$7, model=$8, next_run_at=$9
		 WHERE tenant_id = $1 AND id = $2`,
		job.TenantID, job.ID, job.Name, job.Prompt, job.Schedule,
		job.Deliver, job.Enabled, job.Model, job.NextRunAt)
	if err != nil {
		return fmt.Errorf("pg update cron job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("cron job not found")
	}
	return nil
}

func (s *pgCronJobStore) Delete(ctx context.Context, tenantID, jobID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM cron_jobs WHERE tenant_id = $1 AND id = $2`,
		tenantID, jobID)
	if err != nil {
		return fmt.Errorf("pg delete cron job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("cron job not found")
	}
	return nil
}

func (s *pgCronJobStore) List(ctx context.Context, tenantID string) ([]*store.CronJob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, prompt, schedule, COALESCE(deliver,''), enabled,
		        COALESCE(model,''), next_run_at, last_run_at, run_count, created_at, COALESCE(metadata::text,'{}')
		 FROM cron_jobs WHERE tenant_id = $1 ORDER BY created_at DESC`,
		tenantID)
	if err != nil {
		return nil, fmt.Errorf("pg list cron jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*store.CronJob
	for rows.Next() {
		var j store.CronJob
		if err := rows.Scan(&j.ID, &j.TenantID, &j.Name, &j.Prompt, &j.Schedule, &j.Deliver, &j.Enabled,
			&j.Model, &j.NextRunAt, &j.LastRunAt, &j.RunCount, &j.CreatedAt, &j.Metadata); err != nil {
			return nil, fmt.Errorf("pg scan cron job: %w", err)
		}
		jobs = append(jobs, &j)
	}
	return jobs, rows.Err()
}

// ListDue returns all due cron jobs across tenants (system-level scheduler query).
// tenant_sql_check:skip — intentional cross-tenant query for scheduler dispatch.
func (s *pgCronJobStore) ListDue(ctx context.Context, now time.Time) ([]*store.CronJob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, prompt, schedule, COALESCE(deliver,''), enabled,
		        COALESCE(model,''), next_run_at, last_run_at, run_count, created_at, COALESCE(metadata::text,'{}')
		 FROM cron_jobs WHERE enabled = true AND next_run_at <= $1
		 ORDER BY next_run_at ASC`, now)
	if err != nil {
		return nil, fmt.Errorf("pg list due cron jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*store.CronJob
	for rows.Next() {
		var j store.CronJob
		if err := rows.Scan(&j.ID, &j.TenantID, &j.Name, &j.Prompt, &j.Schedule, &j.Deliver, &j.Enabled,
			&j.Model, &j.NextRunAt, &j.LastRunAt, &j.RunCount, &j.CreatedAt, &j.Metadata); err != nil {
			return nil, fmt.Errorf("pg scan due cron job: %w", err)
		}
		jobs = append(jobs, &j)
	}
	return jobs, rows.Err()
}

var _ store.CronJobStore = (*pgCronJobStore)(nil)
