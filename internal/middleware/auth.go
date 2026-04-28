package middleware

import (
	"log/slog"
	"net/http"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
)

// AuthMiddleware extracts credentials via the extractor chain and populates AuthContext.
// If allowAnonymous is true, requests without credentials proceed with no AuthContext.
// If allowAnonymous is false, unauthenticated requests receive 401.
func AuthMiddleware(chain *auth.ExtractorChain, allowAnonymous bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac, err := chain.Extract(r)
			if err != nil {
				slog.Warn("auth_extraction_failed",
					"error", err,
					"remote", r.RemoteAddr,
					"request_id", RequestIDFromContext(r.Context()),
				)
				http.Error(w, "authentication failed", http.StatusUnauthorized)
				return
			}
			if ac == nil && !allowAnonymous {
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
