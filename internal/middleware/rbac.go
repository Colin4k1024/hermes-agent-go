package middleware

import (
	"net/http"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
)

// RBACConfig maps route patterns to required roles.
// Rules keys support two formats:
//   - "METHOD /path" — matches only the given HTTP method and path prefix (e.g. "DELETE /v1/tenants")
//   - "/path"        — matches any method with the given path prefix (backward compatible)
type RBACConfig struct {
	DefaultRole string
	Rules       map[string]string
}

// RBACMiddleware checks that the authenticated user has the required role.
func RBACMiddleware(cfg RBACConfig) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac, ok := auth.FromContext(r.Context())
			if !ok || ac == nil {
				http.Error(w, "authorization required", http.StatusUnauthorized)
				return
			}

			required := cfg.DefaultRole
			bestLen := 0
			for pattern, role := range cfg.Rules {
				method, prefix := parseRBACPattern(pattern)
				if method != "" && method != r.Method {
					continue
				}
				if len(r.URL.Path) >= len(prefix) && r.URL.Path[:len(prefix)] == prefix {
					score := len(prefix)
					if method != "" {
						score += 1000 // method-specific rules take priority
					}
					if score > bestLen {
						bestLen = score
						required = role
					}
				}
			}

			if required != "" && !ac.HasRole(required) && !ac.HasRole("admin") {
				http.Error(w, "insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func parseRBACPattern(pattern string) (method, prefix string) {
	if i := strings.Index(pattern, " "); i > 0 && i < len(pattern)-1 {
		m := pattern[:i]
		switch m {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
			return m, pattern[i+1:]
		}
	}
	return "", pattern
}
