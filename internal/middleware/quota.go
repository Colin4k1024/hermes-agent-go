package middleware

import (
	"net/http"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// QuotaConfig holds quota enforcement settings.
type QuotaConfig struct {
	Sessions      store.SessionStore
	MaxSessionsFn func(tenantID string) int // returns max sessions for tenant
}

// QuotaMiddleware enforces per-tenant session limits.
// Applied to session-creating endpoints only.
func QuotaMiddleware(cfg QuotaConfig) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.MaxSessionsFn == nil {
				next.ServeHTTP(w, r)
				return
			}

			ac, ok := auth.FromContext(r.Context())
			if !ok || ac == nil {
				next.ServeHTTP(w, r)
				return
			}

			maxSessions := cfg.MaxSessionsFn(ac.TenantID)
			if maxSessions <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			_, count, err := cfg.Sessions.List(r.Context(), ac.TenantID, store.ListOptions{Limit: 1})
			if err != nil {
				http.Error(w, "quota check failed", http.StatusInternalServerError)
				return
			}

			if count >= maxSessions {
				http.Error(w, "session quota exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
