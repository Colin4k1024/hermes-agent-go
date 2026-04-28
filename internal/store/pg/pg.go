package pg

import (
	"context"
	"fmt"

	"github.com/hermes-agent/hermes-agent-go/internal/observability"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func init() {
	store.RegisterDriver("postgres", func(ctx context.Context, cfg store.StoreConfig) (store.Store, error) {
		return New(ctx, cfg.URL)
	})
}

// PGStore implements store.Store backed by PostgreSQL.
type PGStore struct {
	pool      *pgxpool.Pool
	sessions  *pgSessionStore
	messages  *pgMessageStore
	users     *pgUserStore
	tenants   *pgTenantStore
	auditLogs *pgAuditLogStore
	apiKeys   *pgAPIKeyStore
}

// New creates a PGStore with a connection pool and query tracing.
func New(ctx context.Context, databaseURL string) (*PGStore, error) {
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pg parse config: %w", err)
	}
	poolCfg.ConnConfig.Tracer = &observability.PGXTracer{}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("pg connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pg ping: %w", err)
	}

	s := &PGStore{pool: pool}
	s.sessions = &pgSessionStore{pool: pool}
	s.messages = &pgMessageStore{pool: pool}
	s.users = &pgUserStore{pool: pool}
	s.tenants = &pgTenantStore{pool: pool}
	s.auditLogs = &pgAuditLogStore{pool: pool}
	s.apiKeys = &pgAPIKeyStore{pool: pool}
	return s, nil
}

func (s *PGStore) Sessions() store.SessionStore   { return s.sessions }
func (s *PGStore) Messages() store.MessageStore   { return s.messages }
func (s *PGStore) Users() store.UserStore         { return s.users }
func (s *PGStore) Tenants() store.TenantStore     { return s.tenants }
func (s *PGStore) AuditLogs() store.AuditLogStore { return s.auditLogs }
func (s *PGStore) APIKeys() store.APIKeyStore     { return s.apiKeys }

func (s *PGStore) Close() error {
	s.pool.Close()
	return nil
}

func (s *PGStore) Migrate(ctx context.Context) error {
	return runMigrations(ctx, s.pool)
}

// Pool returns the underlying connection pool for direct use by gateway components.
func (s *PGStore) Pool() *pgxpool.Pool {
	return s.pool
}

// Ping checks database connectivity (used by health probes).
func (s *PGStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

var _ store.Store = (*PGStore)(nil)
