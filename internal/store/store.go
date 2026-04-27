package store

import "context"

// Store is the unified state persistence interface.
// Implementations: pg.PGStore (production), sqlite.SQLiteStore (local dev).
type Store interface {
	Sessions() SessionStore
	Messages() MessageStore
	Users() UserStore
	Close() error
	Migrate(ctx context.Context) error
}

// SessionStore manages conversation sessions.
type SessionStore interface {
	Create(ctx context.Context, tenantID string, s *Session) error
	Get(ctx context.Context, tenantID, sessionID string) (*Session, error)
	End(ctx context.Context, tenantID, sessionID, reason string) error
	List(ctx context.Context, tenantID string, opts ListOptions) ([]*Session, int, error)
	Delete(ctx context.Context, tenantID, sessionID string) error
	UpdateTokens(ctx context.Context, tenantID, sessionID string, delta TokenDelta) error
	SetTitle(ctx context.Context, tenantID, sessionID, title string) error
}

// MessageStore manages conversation messages.
type MessageStore interface {
	Append(ctx context.Context, tenantID, sessionID string, msg *Message) (int64, error)
	List(ctx context.Context, tenantID, sessionID string, limit, offset int) ([]*Message, error)
	Search(ctx context.Context, tenantID, query string, limit int) ([]*SearchResult, error)
	CountBySession(ctx context.Context, tenantID, sessionID string) (int, error)
}

// UserStore manages user accounts and permissions.
type UserStore interface {
	GetOrCreate(ctx context.Context, tenantID, externalID, username string) (*User, error)
	IsApproved(ctx context.Context, tenantID, platform, userID string) (bool, error)
	Approve(ctx context.Context, tenantID, platform, userID string) error
	Revoke(ctx context.Context, tenantID, platform, userID string) error
	ListApproved(ctx context.Context, tenantID, platform string) ([]string, error)
}

// ListOptions controls pagination and filtering for list queries.
type ListOptions struct {
	Platform string
	UserID   string
	Limit    int
	Offset   int
}
