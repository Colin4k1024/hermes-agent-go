package pg

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{1, `CREATE TABLE IF NOT EXISTS tenants (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		plan TEXT NOT NULL DEFAULT 'free',
		rate_limit_rpm INT NOT NULL DEFAULT 60,
		max_sessions INT NOT NULL DEFAULT 100,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`},

	{2, `CREATE TABLE IF NOT EXISTS sessions (
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
	)`},
	{3, `CREATE INDEX IF NOT EXISTS idx_sessions_tenant ON sessions(tenant_id)`},
	{4, `CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(tenant_id, user_id)`},
	{5, `CREATE INDEX IF NOT EXISTS idx_sessions_platform ON sessions(tenant_id, platform)`},

	{6, `CREATE TABLE IF NOT EXISTS messages (
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
	)`},
	{7, `CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(tenant_id, session_id)`},
	{8, `CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(tenant_id, session_id, timestamp)`},
	{9, `CREATE INDEX IF NOT EXISTS idx_messages_fts ON messages USING GIN(to_tsvector('english', coalesce(content, '')))`},

	{10, `CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		external_id TEXT NOT NULL,
		username TEXT,
		display_name TEXT,
		role TEXT DEFAULT 'user',
		approved_at TIMESTAMPTZ,
		metadata JSONB DEFAULT '{}'
	)`},
	{11, `CREATE UNIQUE INDEX IF NOT EXISTS idx_users_external ON users(tenant_id, external_id)`},

	{12, `CREATE TABLE IF NOT EXISTS audit_logs (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		user_id UUID,
		session_id TEXT,
		action TEXT NOT NULL,
		detail TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`},
	{13, `CREATE INDEX IF NOT EXISTS idx_audit_tenant ON audit_logs(tenant_id)`},

	{14, `CREATE TABLE IF NOT EXISTS cron_jobs (
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
	)`},
	{15, `CREATE INDEX IF NOT EXISTS idx_cron_tenant ON cron_jobs(tenant_id)`},
	{16, `CREATE INDEX IF NOT EXISTS idx_cron_next ON cron_jobs(next_run_at) WHERE enabled = true`},

	// P1: API keys table
	{17, `CREATE TABLE IF NOT EXISTS api_keys (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		name TEXT NOT NULL,
		key_hash TEXT NOT NULL,
		prefix TEXT NOT NULL,
		roles TEXT[] DEFAULT '{user}',
		expires_at TIMESTAMPTZ,
		revoked_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`},
	{18, `CREATE UNIQUE INDEX IF NOT EXISTS idx_apikeys_hash ON api_keys(key_hash)`},
	{19, `CREATE INDEX IF NOT EXISTS idx_apikeys_tenant ON api_keys(tenant_id)`},

	// P2: Gateway session key + memory tables (v20-v23)
	{20, `ALTER TABLE sessions ADD COLUMN IF NOT EXISTS session_key TEXT`},
	{21, `CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_key ON sessions(session_key) WHERE session_key IS NOT NULL`},

	{22, `CREATE TABLE IF NOT EXISTS memories (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		user_id TEXT NOT NULL,
		key TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE(tenant_id, user_id, key)
	)`},
	{23, `CREATE TABLE IF NOT EXISTS user_profiles (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		user_id TEXT NOT NULL,
		content TEXT NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE(tenant_id, user_id)
	)`},

	// P3: Audit trail enrichment
	{24, `ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS request_id TEXT`},
	{25, `ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS status_code INT`},
	{26, `ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS latency_ms INT`},
	{27, `CREATE INDEX IF NOT EXISTS idx_audit_request ON audit_logs(request_id)`},

	// P4: Soft delete + cascade + GDPR hardening
	{28, `ALTER TABLE tenants ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ`},
	{29, `CREATE INDEX IF NOT EXISTS idx_tenants_active ON tenants(id) WHERE deleted_at IS NULL`},
	{30, `CREATE INDEX IF NOT EXISTS idx_tenants_deleted ON tenants(deleted_at) WHERE deleted_at IS NOT NULL`},

	// Make audit_logs.tenant_id nullable (auth failure events have no tenant context).
	{31, `ALTER TABLE audit_logs ALTER COLUMN tenant_id DROP NOT NULL`},

	// Add source_ip, error_code, user_agent columns to audit_logs.
	{32, `ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS source_ip TEXT`},
	{33, `ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS error_code TEXT`},
	{34, `ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS user_agent TEXT`},
	{35, `CREATE INDEX IF NOT EXISTS idx_audit_error_code ON audit_logs(error_code) WHERE error_code IS NOT NULL`},

	// P1-S1: RBAC fine-grained permissions
	{36, `CREATE TABLE IF NOT EXISTS roles (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID NOT NULL REFERENCES tenants(id),
		name TEXT NOT NULL,
		description TEXT,
		is_system BOOLEAN DEFAULT false,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE(tenant_id, name)
	)`},
	{37, `CREATE TABLE IF NOT EXISTS role_permissions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
		resource TEXT NOT NULL,
		action TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE(role_id, resource, action)
	)`},
}

const migrationLockID int64 = 0x48455231 // "HER1" — advisory lock for migration exclusion

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, migrationLockID).Scan(&locked); err != nil {
		return fmt.Errorf("advisory lock check: %w", err)
	}
	if !locked {
		slog.Info("PG migrations skipped — another instance holds the lock")
		return nil
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, migrationLockID) //nolint:errcheck

	_, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version INT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	if err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	var current int
	err = conn.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	applied := 0
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if _, err := conn.Exec(ctx, m.sql); err != nil {
			return fmt.Errorf("migration v%d failed: %w", m.version, err)
		}
		if _, err := conn.Exec(ctx, `INSERT INTO schema_version (version) VALUES ($1)`, m.version); err != nil {
			return fmt.Errorf("record migration v%d: %w", m.version, err)
		}
		applied++
	}

	slog.Info("PG migrations completed", "current", current, "applied", applied, "latest", len(migrations))
	return nil
}
