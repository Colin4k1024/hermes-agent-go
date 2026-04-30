package middleware

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/observability"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// AuditMiddleware logs key actions to the audit store.
// Captures method, path, status code, latency, and request_id.
func AuditMiddleware(auditStore store.AuditLogStore) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &auditStatusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			ac, ok := auth.FromContext(r.Context())
			if !ok || ac == nil {
				return
			}

			// Only set UserID if Identity is a valid UUID (API keys use UUID; static tokens use "static-user" string).
			entry := &store.AuditLog{
				TenantID:   ac.TenantID,
				Action:     r.Method + " " + r.URL.Path,
				Detail:     sanitizeQuery(r.URL.RawQuery),
				RequestID:  RequestIDFromContext(r.Context()),
				StatusCode: sw.status,
				LatencyMs:  int(time.Since(start).Milliseconds()),
				SourceIP:   r.RemoteAddr,
				UserAgent:  r.UserAgent(),
			}
			if _, err := uuid.Parse(ac.Identity); err == nil {
				entry.UserID = ac.Identity
			}
			if err := auditStore.Append(r.Context(), entry); err != nil {
				observability.ContextLogger(r.Context()).Warn("audit log write failed", "error", err)
			}
		})
	}
}

type auditStatusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *auditStatusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *auditStatusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *auditStatusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

var sensitiveParams = map[string]bool{
	"token": true, "key": true, "secret": true, "password": true,
	"api_key": true, "apikey": true, "access_token": true, "auth": true,
}

func sanitizeQuery(raw string) string {
	if raw == "" {
		return ""
	}
	vals, err := url.ParseQuery(raw)
	if err != nil {
		return "[redacted:parse_error]"
	}
	for k := range vals {
		if sensitiveParams[strings.ToLower(k)] {
			vals.Set(k, "[REDACTED]")
		}
	}
	return vals.Encode()
}
