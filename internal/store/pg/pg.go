package pg

import (
	"context"
	"fmt"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore implements store.Store backed by PostgreSQL.
type PGStore struct {
	pool     *pgxpool.Pool
	sessions *pgSessionStore
	messages *pgMessageStore
	users    *pgUserStore
}

// New creates a PGStore with a connection pool.
func New(ctx context.Context, databaseURL string) (*PGStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
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
	return s, nil
}

func (s *PGStore) Sessions() store.SessionStore { return s.sessions }
func (s *PGStore) Messages() store.MessageStore { return s.messages }
func (s *PGStore) Users() store.UserStore       { return s.users }

func (s *PGStore) Close() error {
	s.pool.Close()
	return nil
}

func (s *PGStore) Migrate(ctx context.Context) error {
	return runMigrations(ctx, s.pool)
}

var _ store.Store = (*PGStore)(nil)
