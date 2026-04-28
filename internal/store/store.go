package store

import "context"

// Store is the unified state persistence interface.
// Implementations: pg.PGStore (production), sqlite.SQLiteStore (local dev).
type Store interface {
	Sessions() SessionStore
	Messages() MessageStore
	Users() UserStore
	Tenants() TenantStore
	AuditLogs() AuditLogStore
	APIKeys() APIKeyStore
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

// TenantStore manages tenant CRUD operations.
type TenantStore interface {
	Create(ctx context.Context, t *Tenant) error
	Get(ctx context.Context, id string) (*Tenant, error)
	Update(ctx context.Context, t *Tenant) error
	Delete(ctx context.Context, id string) error // soft delete
	List(ctx context.Context, opts ListOptions) ([]*Tenant, int, error)
}

// AuditLogStore manages append-only audit trail.
type AuditLogStore interface {
	Append(ctx context.Context, log *AuditLog) error
	List(ctx context.Context, tenantID string, opts AuditListOptions) ([]*AuditLog, int, error)
}

// AuditListOptions controls pagination for audit log queries.
type AuditListOptions struct {
	Action string
	Limit  int
	Offset int
}

// APIKeyStore manages hashed API key lifecycle.
type APIKeyStore interface {
	Create(ctx context.Context, key *APIKey) error
	GetByHash(ctx context.Context, hash string) (*APIKey, error)
	GetByID(ctx context.Context, tenantID, id string) (*APIKey, error)
	List(ctx context.Context, tenantID string) ([]*APIKey, error)
	Revoke(ctx context.Context, tenantID, id string) error
}

// ListOptions controls pagination and filtering for list queries.
type ListOptions struct {
	Platform string
	UserID   string
	Limit    int
	Offset   int
}
