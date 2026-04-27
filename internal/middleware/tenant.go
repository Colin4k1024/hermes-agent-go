package middleware

import (
	"context"
	"net/http"
	"regexp"
)

type contextKey string

const tenantKey contextKey = "tenant_id"

var tenantIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// TenantFromContext extracts the tenant ID from context.
func TenantFromContext(ctx context.Context) string {
	v, _ := ctx.Value(tenantKey).(string)
	return v
}

// WithTenant injects a tenant ID into the context.
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantKey, tenantID)
}

// TenantMiddleware extracts tenant_id from X-Tenant-ID header or defaults to "default".
func TenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			tenantID = "default"
		}
		if !tenantIDPattern.MatchString(tenantID) {
			http.Error(w, "invalid tenant ID", http.StatusBadRequest)
			return
		}
		ctx := WithTenant(r.Context(), tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
