package store

import "time"

// Session represents a conversation session.
type Session struct {
	ID               string         `json:"id" db:"id"`
	TenantID         string         `json:"tenant_id" db:"tenant_id"`
	Platform         string         `json:"platform" db:"platform"`
	UserID           string         `json:"user_id" db:"user_id"`
	Model            string         `json:"model" db:"model"`
	SystemPrompt     string         `json:"system_prompt,omitempty" db:"system_prompt"`
	ParentSessionID  string         `json:"parent_session_id,omitempty" db:"parent_session_id"`
	Title            string         `json:"title,omitempty" db:"title"`
	StartedAt        time.Time      `json:"started_at" db:"started_at"`
	EndedAt          *time.Time     `json:"ended_at,omitempty" db:"ended_at"`
	EndReason        string         `json:"end_reason,omitempty" db:"end_reason"`
	MessageCount     int            `json:"message_count" db:"message_count"`
	ToolCallCount    int            `json:"tool_call_count" db:"tool_call_count"`
	InputTokens      int            `json:"input_tokens" db:"input_tokens"`
	OutputTokens     int            `json:"output_tokens" db:"output_tokens"`
	CacheReadTokens  int            `json:"cache_read_tokens" db:"cache_read_tokens"`
	CacheWriteTokens int            `json:"cache_write_tokens" db:"cache_write_tokens"`
	EstimatedCostUSD float64        `json:"estimated_cost_usd" db:"estimated_cost_usd"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// Message represents a conversation message.
type Message struct {
	ID           int64     `json:"id" db:"id"`
	TenantID     string    `json:"tenant_id" db:"tenant_id"`
	SessionID    string    `json:"session_id" db:"session_id"`
	Role         string    `json:"role" db:"role"`
	Content      string    `json:"content,omitempty" db:"content"`
	ToolCallID   string    `json:"tool_call_id,omitempty" db:"tool_call_id"`
	ToolCalls    string    `json:"tool_calls,omitempty" db:"tool_calls"` // JSON
	ToolName     string    `json:"tool_name,omitempty" db:"tool_name"`
	Reasoning    string    `json:"reasoning,omitempty" db:"reasoning"`
	Timestamp    time.Time `json:"timestamp" db:"timestamp"`
	TokenCount   int       `json:"token_count,omitempty" db:"token_count"`
	FinishReason string    `json:"finish_reason,omitempty" db:"finish_reason"`
}

// User represents a platform user.
type User struct {
	ID          string         `json:"id" db:"id"`
	TenantID    string         `json:"tenant_id" db:"tenant_id"`
	ExternalID  string         `json:"external_id" db:"external_id"` // platform:user_id
	Username    string         `json:"username,omitempty" db:"username"`
	DisplayName string         `json:"display_name,omitempty" db:"display_name"`
	Role        string         `json:"role" db:"role"` // user / admin
	ApprovedAt  *time.Time     `json:"approved_at,omitempty" db:"approved_at"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// TokenDelta represents incremental token count updates.
type TokenDelta struct {
	Input      int
	Output     int
	CacheRead  int
	CacheWrite int
	Reasoning  int
}

// SearchResult represents a full-text search match.
type SearchResult struct {
	SessionID string  `json:"session_id"`
	MessageID int64   `json:"message_id"`
	Content   string  `json:"content"`
	Snippet   string  `json:"snippet"`
	Rank      float64 `json:"rank"`
}

// Tenant represents a SaaS tenant.
type Tenant struct {
	ID           string     `json:"id" db:"id"`
	Name         string     `json:"name" db:"name"`
	Plan         string     `json:"plan" db:"plan"` // free / pro / enterprise
	RateLimitRPM int        `json:"rate_limit_rpm" db:"rate_limit_rpm"`
	MaxSessions  int        `json:"max_sessions" db:"max_sessions"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// AuditLog represents an immutable audit trail entry.
type AuditLog struct {
	ID         int64     `json:"id" db:"id"`
	TenantID   string    `json:"tenant_id" db:"tenant_id"`
	UserID     string    `json:"user_id,omitempty" db:"user_id"`
	SessionID  string    `json:"session_id,omitempty" db:"session_id"`
	Action     string    `json:"action" db:"action"`
	Detail     string    `json:"detail,omitempty" db:"detail"`
	RequestID  string    `json:"request_id,omitempty" db:"request_id"`
	StatusCode int       `json:"status_code,omitempty" db:"status_code"`
	LatencyMs  int       `json:"latency_ms,omitempty" db:"latency_ms"`
	SourceIP   string    `json:"source_ip,omitempty" db:"source_ip"`
	ErrorCode  string    `json:"error_code,omitempty" db:"error_code"`
	UserAgent  string    `json:"user_agent,omitempty" db:"user_agent"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// APIKey represents a hashed API key bound to a tenant.
type APIKey struct {
	ID        string     `json:"id" db:"id"`
	TenantID  string     `json:"tenant_id" db:"tenant_id"`
	Name      string     `json:"name" db:"name"`
	KeyHash   string     `json:"-" db:"key_hash"`    // SHA-256 hash, never exposed
	Prefix    string     `json:"prefix" db:"prefix"` // first 8 chars for identification
	Roles     []string   `json:"roles" db:"roles"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}
