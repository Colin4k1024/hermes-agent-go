package pg

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS tenants (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		plan TEXT NOT NULL DEFAULT 'free',
		rate_limit_rpm INT NOT NULL DEFAULT 60,
		max_sessions INT NOT NULL DEFAULT 100,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,

	`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		platform TEXT NOT NULL,
		user_id TEXT NOT NULL,
		model TEXT,
		system_prompt TEXT,
		parent_session_id TEXT,
		title TEXT,
		started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		ended_at TIMESTAMPTZ,
		end_reason TEXT,
		message_count INT DEFAULT 0,
		tool_call_count INT DEFAULT 0,
		input_tokens INT DEFAULT 0,
		output_tokens INT DEFAULT 0,
		cache_read_tokens INT DEFAULT 0,
		cache_write_tokens INT DEFAULT 0,
		estimated_cost_usd NUMERIC(10,6),
		metadata JSONB DEFAULT '{}'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_tenant ON sessions(tenant_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(tenant_id, user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_platform ON sessions(tenant_id, platform)`,

	`CREATE TABLE IF NOT EXISTS messages (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT,
		tool_call_id TEXT,
		tool_calls JSONB,
		tool_name TEXT,
		reasoning TEXT,
		timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
		token_count INT,
		finish_reason TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(tenant_id, session_id)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(tenant_id, session_id, timestamp)`,
	// GIN index for full-text search
	`CREATE INDEX IF NOT EXISTS idx_messages_fts ON messages USING GIN(to_tsvector('english', coalesce(content, '')))`,

	`CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		external_id TEXT NOT NULL,
		username TEXT,
		display_name TEXT,
		role TEXT DEFAULT 'user',
		approved_at TIMESTAMPTZ,
		metadata JSONB DEFAULT '{}'
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_external ON users(tenant_id, external_id)`,

	`CREATE TABLE IF NOT EXISTS audit_logs (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		user_id UUID,
		session_id TEXT,
		action TEXT NOT NULL,
		detail TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_tenant ON audit_logs(tenant_id)`,

	`CREATE TABLE IF NOT EXISTS cron_jobs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		name TEXT NOT NULL,
		prompt TEXT NOT NULL,
		schedule TEXT NOT NULL,
		deliver TEXT,
		enabled BOOLEAN DEFAULT true,
		model TEXT,
		next_run_at TIMESTAMPTZ,
		last_run_at TIMESTAMPTZ,
		run_count INT DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		metadata JSONB DEFAULT '{}'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_cron_tenant ON cron_jobs(tenant_id)`,
	`CREATE INDEX IF NOT EXISTS idx_cron_next ON cron_jobs(next_run_at) WHERE enabled = true`,
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	for i, ddl := range migrations {
		if _, err := pool.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
	}
	slog.Info("PG migrations completed", "count", len(migrations))
	return nil
}
