package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
)

func TestRBACMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		cfg        RBACConfig
		authCtx    *auth.AuthContext // nil means no AuthContext in request
		method     string            // defaults to GET
		path       string
		wantStatus int
	}{
		{
			name:       "no AuthContext returns 401",
			cfg:        RBACConfig{DefaultRole: "user"},
			authCtx:    nil,
			path:       "/v1/data",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "matching role returns 200",
			cfg:  RBACConfig{DefaultRole: "user"},
			authCtx: &auth.AuthContext{
				Identity: "u1",
				TenantID: "t1",
				Roles:    []string{"user"},
			},
			path:       "/v1/data",
			wantStatus: http.StatusOK,
		},
		{
			name: "admin bypasses any role requirement",
			cfg:  RBACConfig{DefaultRole: "operator"},
			authCtx: &auth.AuthContext{
				Identity: "u2",
				TenantID: "t1",
				Roles:    []string{"admin"},
			},
			path:       "/v1/data",
			wantStatus: http.StatusOK,
		},
		{
			name: "missing required role returns 403",
			cfg:  RBACConfig{DefaultRole: "operator"},
			authCtx: &auth.AuthContext{
				Identity: "u3",
				TenantID: "t1",
				Roles:    []string{"user"},
			},
			path:       "/v1/data",
			wantStatus: http.StatusForbidden,
		},
		{
			name: "path-based rule overrides default",
			cfg: RBACConfig{
				DefaultRole: "user",
				Rules:       map[string]string{"/v1/admin": "admin"},
			},
			authCtx: &auth.AuthContext{
				Identity: "u4",
				TenantID: "t1",
				Roles:    []string{"user"},
			},
			path:       "/v1/admin/settings",
			wantStatus: http.StatusForbidden,
		},
		{
			name: "no default role and no path match allows through",
			cfg: RBACConfig{
				DefaultRole: "",
				Rules:       map[string]string{"/v1/admin": "admin"},
			},
			authCtx: &auth.AuthContext{
				Identity: "u5",
				TenantID: "t1",
				Roles:    []string{"user"},
			},
			path:       "/v1/data",
			wantStatus: http.StatusOK,
		},
		{
			name: "method+path rule matches DELETE",
			cfg: RBACConfig{
				DefaultRole: "user",
				Rules:       map[string]string{"DELETE /v1/tenants": "admin"},
			},
			authCtx: &auth.AuthContext{
				Identity: "u6",
				TenantID: "t1",
				Roles:    []string{"user"},
			},
			method:     http.MethodDelete,
			path:       "/v1/tenants/t1",
			wantStatus: http.StatusForbidden,
		},
		{
			name: "method+path rule allows GET on same path",
			cfg: RBACConfig{
				DefaultRole: "user",
				Rules:       map[string]string{"DELETE /v1/tenants": "admin"},
			},
			authCtx: &auth.AuthContext{
				Identity: "u7",
				TenantID: "t1",
				Roles:    []string{"user"},
			},
			path:       "/v1/tenants",
			wantStatus: http.StatusOK,
		},
		{
			name: "method+path rule takes priority over path-only",
			cfg: RBACConfig{
				DefaultRole: "",
				Rules: map[string]string{
					"/v1/tenants":        "user",
					"DELETE /v1/tenants": "admin",
				},
			},
			authCtx: &auth.AuthContext{
				Identity: "u8",
				TenantID: "t1",
				Roles:    []string{"user"},
			},
			method:     http.MethodDelete,
			path:       "/v1/tenants/t1",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			mw := RBACMiddleware(tt.cfg)
			handler := mw(inner)

			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, tt.path, nil)
			if tt.authCtx != nil {
				ctx := auth.WithContext(req.Context(), tt.authCtx)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
