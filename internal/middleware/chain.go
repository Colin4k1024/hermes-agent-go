package middleware

import "net/http"

// Middleware wraps an http.Handler with cross-cutting behavior.
type Middleware func(http.Handler) http.Handler

// StackConfig configures which middleware layers are active.
type StackConfig struct {
	Auth      Middleware
	Tenant    Middleware
	RBAC      Middleware
	RateLimit Middleware
	RequestID Middleware
	Metrics   Middleware
	Logging   Middleware
	Audit     Middleware
	Tracing   Middleware
}

// MiddlewareStack enforces a fixed middleware ordering across all HTTP servers.
// Execution order (outermost first): Metrics → RequestID → Auth → Tenant → RBAC → RateLimit → Handler.
type MiddlewareStack struct {
	cfg StackConfig
}

// NewStack creates a MiddlewareStack from the given config.
// Nil middleware slots are treated as passthrough (no-op).
func NewStack(cfg StackConfig) *MiddlewareStack {
	return &MiddlewareStack{cfg: cfg}
}

// Wrap applies the full middleware chain to the given handler.
// Execution order (outermost first):
//
//	Tracing → Metrics → RequestID → Auth → Tenant → Logging → Audit → RBAC → RateLimit → Handler
//
// Logging runs after Auth+Tenant so it can enrich the logger with tenant_id.
// Auth errors use ContextLogger fallback (slog.Default with request_id from RequestID middleware).
func (s *MiddlewareStack) Wrap(handler http.Handler) http.Handler {
	h := handler
	h = apply(s.cfg.RateLimit, h)
	h = apply(s.cfg.RBAC, h)
	h = apply(s.cfg.Audit, h)
	h = apply(s.cfg.Logging, h) // after Auth+Tenant so tenant_id is available
	h = apply(s.cfg.Tenant, h)
	h = apply(s.cfg.Auth, h)
	h = apply(s.cfg.RequestID, h)
	h = apply(s.cfg.Metrics, h)
	h = apply(s.cfg.Tracing, h)
	return h
}

func apply(mw Middleware, h http.Handler) http.Handler {
	if mw == nil {
		return h
	}
	return mw(h)
}
