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

// mockAuditStore is a test double for store.AuditLogStore.
type mockAuditStore struct {
	logs []*store.AuditLog
	err  error
}

func (m *mockAuditStore) Append(_ context.Context, log *store.AuditLog) error {
	if m.err != nil {
		return m.err
	}
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockAuditStore) List(_ context.Context, _ string, _ store.AuditListOptions) ([]*store.AuditLog, int, error) {
	return m.logs, len(m.logs), nil
}

func TestAuditMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		authCtx    *auth.AuthContext
		storeErr   error
		method     string
		path       string
		query      string
		wantLogs   int
		wantAction string
		wantTenant string
		wantUser   string
		wantDetail string
		wantStatus int
	}{
		{
			name: "with AuthContext writes audit log",
			authCtx: &auth.AuthContext{
				Identity: "00000000-0000-0000-0000-000000000001",
				TenantID: "t-1",
				Roles:    []string{"user"},
			},
			method:     http.MethodPost,
			path:       "/v1/sessions",
			query:      "model=gpt-4",
			wantLogs:   1,
			wantAction: "POST /v1/sessions",
			wantTenant: "t-1",
			wantUser:   "00000000-0000-0000-0000-000000000001",
			wantDetail: "model=gpt-4",
			wantStatus: http.StatusOK,
		},
		{
			name:       "no AuthContext skips audit log",
			authCtx:    nil,
			method:     http.MethodGet,
			path:       "/v1/health",
			wantLogs:   0,
			wantStatus: http.StatusOK,
		},
		{
			name: "store error still returns 200",
			authCtx: &auth.AuthContext{
				Identity: "user-2",
				TenantID: "t-2",
				Roles:    []string{"admin"},
			},
			storeErr:   errors.New("db write failed"),
			method:     http.MethodDelete,
			path:       "/v1/sessions/abc",
			wantLogs:   0,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := &mockAuditStore{err: tt.storeErr}

			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			mw := AuditMiddleware(as)
			handler := mw(inner)

			target := tt.path
			if tt.query != "" {
				target += "?" + tt.query
			}
			req := httptest.NewRequest(tt.method, target, nil)
			if tt.authCtx != nil {
				ctx := auth.WithContext(req.Context(), tt.authCtx)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if len(as.logs) != tt.wantLogs {
				t.Fatalf("audit log count = %d, want %d", len(as.logs), tt.wantLogs)
			}

			if tt.wantLogs > 0 {
				entry := as.logs[0]
				if entry.Action != tt.wantAction {
					t.Errorf("action = %q, want %q", entry.Action, tt.wantAction)
				}
				if entry.TenantID != tt.wantTenant {
					t.Errorf("tenant = %q, want %q", entry.TenantID, tt.wantTenant)
				}
				if entry.UserID != tt.wantUser {
					t.Errorf("user = %q, want %q", entry.UserID, tt.wantUser)
				}
				if entry.Detail != tt.wantDetail {
					t.Errorf("detail = %q, want %q", entry.Detail, tt.wantDetail)
				}
			}
		})
	}
}
