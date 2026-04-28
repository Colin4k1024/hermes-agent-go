package observability

import (
	"context"
	"log/slog"
)

type ctxKey int

const loggerKey ctxKey = iota

// WithLogger stores an enriched slog.Logger in the context.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// ContextLogger returns a logger enriched with request_id, tenant_id,
// session_id, and trace_id extracted from the context. Falls back to the
// default slog logger when no enriched logger is stored.
func ContextLogger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
