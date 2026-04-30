package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// mockExtractor is a test double for auth.CredentialExtractor.
type mockExtractor struct {
	ac  *auth.AuthContext
	err error
}

func (m *mockExtractor) Extract(_ *http.Request) (*auth.AuthContext, error) {
	return m.ac, m.err
}

func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		extractor      *mockExtractor
		allowAnonymous bool
		wantStatus     int
		wantIdentity   string // expected identity in context; empty means no AuthContext
	}{
		{
			name: "valid auth populates context",
			extractor: &mockExtractor{
				ac: &auth.AuthContext{
					Identity: "user-1",
					TenantID: "t-1",
					Roles:    []string{"user"},
				},
			},
			allowAnonymous: false,
			wantStatus:     http.StatusOK,
			wantIdentity:   "user-1",
		},
		{
			name: "extractor returns error gives 401",
			extractor: &mockExtractor{
				err: errors.New("invalid token"),
			},
			allowAnonymous: false,
			wantStatus:     http.StatusUnauthorized,
		},
		{
			name:           "no credentials and allowAnonymous false gives 401",
			extractor:      &mockExtractor{},
			allowAnonymous: false,
			wantStatus:     http.StatusUnauthorized,
		},
		{
			name:           "no credentials and allowAnonymous true gives 200",
			extractor:      &mockExtractor{},
			allowAnonymous: true,
			wantStatus:     http.StatusOK,
			wantIdentity:   "", // no AuthContext set
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := auth.NewExtractorChain(tt.extractor)

			var capturedIdentity string
			var contextHasAuth bool
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ac, ok := auth.FromContext(r.Context())
				contextHasAuth = ok && ac != nil
				if contextHasAuth {
					capturedIdentity = ac.Identity
				}
				w.WriteHeader(http.StatusOK)
			})

			mw := AuthMiddleware(chain, tt.allowAnonymous)
			handler := mw(inner)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK && tt.wantIdentity != "" {
				if !contextHasAuth {
					t.Fatal("expected AuthContext in context, got none")
				}
				if capturedIdentity != tt.wantIdentity {
					t.Errorf("identity = %q, want %q", capturedIdentity, tt.wantIdentity)
				}
			}

			if tt.wantStatus == http.StatusOK && tt.wantIdentity == "" && tt.allowAnonymous {
				if contextHasAuth {
					t.Error("expected no AuthContext for anonymous request, but got one")
				}
			}
		})
	}
}

// authAuditStore captures audit logs written by AuthMiddleware on auth failures.
type authAuditStore struct {
	logs []*store.AuditLog
}

func (s *authAuditStore) Append(_ context.Context, log *store.AuditLog) error {
	s.logs = append(s.logs, log)
	return nil
}
func (s *authAuditStore) List(_ context.Context, _ string, _ store.AuditListOptions) ([]*store.AuditLog, int, error) {
	return nil, 0, nil
}
func (s *authAuditStore) DeleteByTenant(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func TestAuthMiddleware_AuditOnFailure(t *testing.T) {
	tests := []struct {
		name          string
		extractor     *mockExtractor
		wantAuditLogs int
		wantErrorCode string
	}{
		{
			name:          "invalid credentials logs AUTH_INVALID_CREDENTIALS",
			extractor:     &mockExtractor{err: errors.New("bad token")},
			wantAuditLogs: 1,
			wantErrorCode: "AUTH_INVALID_CREDENTIALS",
		},
		{
			name:          "missing credentials logs AUTH_MISSING_CREDENTIALS",
			extractor:     &mockExtractor{},
			wantAuditLogs: 1,
			wantErrorCode: "AUTH_MISSING_CREDENTIALS",
		},
		{
			name: "successful auth writes no audit log",
			extractor: &mockExtractor{ac: &auth.AuthContext{
				Identity: "u1", TenantID: "t1", Roles: []string{"user"},
			}},
			wantAuditLogs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := &authAuditStore{}
			chain := auth.NewExtractorChain(tt.extractor)
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			mw := AuthMiddleware(chain, false, as)
			handler := mw(inner)

			req := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
			req.Header.Set("User-Agent", "test-agent")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if len(as.logs) != tt.wantAuditLogs {
				t.Fatalf("audit logs = %d, want %d", len(as.logs), tt.wantAuditLogs)
			}
			if tt.wantAuditLogs > 0 {
				entry := as.logs[0]
				if entry.Action != "AUTH_FAILED" {
					t.Errorf("action = %q, want AUTH_FAILED", entry.Action)
				}
				if entry.ErrorCode != tt.wantErrorCode {
					t.Errorf("error_code = %q, want %q", entry.ErrorCode, tt.wantErrorCode)
				}
				if entry.StatusCode != http.StatusUnauthorized {
					t.Errorf("status_code = %d, want 401", entry.StatusCode)
				}
				if entry.UserAgent != "test-agent" {
					t.Errorf("user_agent = %q, want test-agent", entry.UserAgent)
				}
			}
		})
	}
}
