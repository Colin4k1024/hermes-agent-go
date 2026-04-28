package middleware

import (
	"log/slog"
	"net/http"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/observability"
)

// LoggingMiddleware creates an enriched slog.Logger with request_id and
// tenant_id, then injects it into the context for downstream handlers.
// Must run after RequestID, Auth, and Tenant middleware.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attrs := []any{
			"request_id", RequestIDFromContext(r.Context()),
		}
		if ac, ok := auth.FromContext(r.Context()); ok && ac != nil {
			attrs = append(attrs, "tenant_id", ac.TenantID)
			if ac.Identity != "" {
				attrs = append(attrs, "user_id", ac.Identity)
			}
		} else {
			tid := TenantFromContext(r.Context())
			if tid != "" {
				attrs = append(attrs, "tenant_id", tid)
			}
		}
		logger := slog.With(attrs...)
		ctx := observability.WithLogger(r.Context(), logger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
