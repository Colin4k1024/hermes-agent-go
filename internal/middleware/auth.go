package middleware

import (
	"log/slog"
	"net/http"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// AuthMiddleware extracts credentials via the extractor chain and populates AuthContext.
// If allowAnonymous is true, requests without credentials proceed with no AuthContext.
// If allowAnonymous is false, unauthenticated requests receive 401.
// An optional auditStore logs failed authentication attempts.
func AuthMiddleware(chain *auth.ExtractorChain, allowAnonymous bool, auditStore ...store.AuditLogStore) Middleware {
	var audit store.AuditLogStore
	if len(auditStore) > 0 {
		audit = auditStore[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac, err := chain.Extract(r)
			if err != nil {
				slog.Warn("auth_extraction_failed",
					"error", err,
					"remote", r.RemoteAddr,
					"request_id", RequestIDFromContext(r.Context()),
				)
				logAuthFailure(audit, r, "AUTH_INVALID_CREDENTIALS", err.Error())
				http.Error(w, "authentication failed", http.StatusUnauthorized)
				return
			}
			if ac == nil && !allowAnonymous {
				logAuthFailure(audit, r, "AUTH_MISSING_CREDENTIALS", "no credentials provided")
				http.Error(w, "authorization required", http.StatusUnauthorized)
				return
			}
			if ac != nil {
				ctx := auth.WithContext(r.Context(), ac)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func logAuthFailure(audit store.AuditLogStore, r *http.Request, errorCode, detail string) {
	if audit == nil {
		return
	}
	entry := &store.AuditLog{
		Action:     "AUTH_FAILED",
		Detail:     detail,
		RequestID:  RequestIDFromContext(r.Context()),
		StatusCode: http.StatusUnauthorized,
		SourceIP:   r.RemoteAddr,
		ErrorCode:  errorCode,
		UserAgent:  r.UserAgent(),
	}
	if err := audit.Append(r.Context(), entry); err != nil {
		slog.Warn("auth_failure_audit_failed", "error", err)
	}
}
