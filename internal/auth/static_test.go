package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStaticTokenExtractor(t *testing.T) {
	const (
		validToken = "my-secret-token"
		tenantID   = "tenant-abc"
	)

	tests := []struct {
		name       string
		authHeader string
		wantNil    bool
		wantErr    bool
		wantID     string
		wantTenant string
		wantRole   string
		wantMethod string
	}{
		{
			name:       "correct token",
			authHeader: "Bearer " + validToken,
			wantNil:    false,
			wantErr:    false,
			wantID:     "static-user",
			wantTenant: tenantID,
			wantRole:   "admin",
			wantMethod: "static_token",
		},
		{
			name:       "wrong token",
			authHeader: "Bearer wrong-token",
			wantNil:    true,
			wantErr:    false,
		},
		{
			name:       "missing authorization header",
			authHeader: "",
			wantNil:    true,
			wantErr:    false,
		},
		{
			name:       "non-bearer scheme",
			authHeader: "Basic dXNlcjpwYXNz",
			wantNil:    true,
			wantErr:    false,
		},
		{
			name:       "empty token in bearer",
			authHeader: "Bearer ",
			wantNil:    true,
			wantErr:    false,
		},
	}

	extractor := NewStaticTokenExtractor(validToken, tenantID)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			ac, err := extractor.Extract(req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if ac != nil {
					t.Fatalf("expected nil AuthContext, got %+v", ac)
				}
				return
			}

			if ac == nil {
				t.Fatal("expected non-nil AuthContext, got nil")
			}
			if ac.Identity != tt.wantID {
				t.Errorf("Identity = %q, want %q", ac.Identity, tt.wantID)
			}
			if ac.TenantID != tt.wantTenant {
				t.Errorf("TenantID = %q, want %q", ac.TenantID, tt.wantTenant)
			}
			if !ac.HasRole(tt.wantRole) {
				t.Errorf("Roles = %v, want to contain %q", ac.Roles, tt.wantRole)
			}
			if ac.AuthMethod != tt.wantMethod {
				t.Errorf("AuthMethod = %q, want %q", ac.AuthMethod, tt.wantMethod)
			}
		})
	}
}
