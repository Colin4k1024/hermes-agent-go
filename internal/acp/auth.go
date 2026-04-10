package acp

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// authToken returns the configured ACP bearer token, or "" if disabled.
func authToken() string {
	return os.Getenv("HERMES_ACP_TOKEN")
}

// withAuth wraps a handler to require bearer token authentication.
// If HERMES_ACP_TOKEN is not set, all requests are allowed (dev mode).
func withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := authToken()
		if token == "" {
			next(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, "authorization header required")
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "bearer token required")
			return
		}

		provided := strings.TrimPrefix(auth, "Bearer ")
		if provided != token {
			slog.Warn("ACP auth failed", "remote", r.RemoteAddr)
			writeError(w, http.StatusForbidden, "invalid token")
			return
		}

		next(w, r)
	}
}
