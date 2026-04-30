package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// mockAuditLogStore is an in-memory implementation of store.AuditLogStore.
type mockAuditLogStore struct {
	logs []*store.AuditLog
}

func (m *mockAuditLogStore) Append(_ context.Context, log *store.AuditLog) error {
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockAuditLogStore) DeleteByTenant(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *mockAuditLogStore) List(_ context.Context, tenantID string, opts store.AuditListOptions) ([]*store.AuditLog, int, error) {
	var filtered []*store.AuditLog
	for _, l := range m.logs {
		if l.TenantID == tenantID {
			if opts.Action != "" && l.Action != opts.Action {
				continue
			}
			filtered = append(filtered, l)
		}
	}

	total := len(filtered)

	// Apply offset and limit.
	if opts.Offset > 0 && opts.Offset < len(filtered) {
		filtered = filtered[opts.Offset:]
	} else if opts.Offset >= len(filtered) {
		filtered = nil
	}
	if opts.Limit > 0 && opts.Limit < len(filtered) {
		filtered = filtered[:opts.Limit]
	}

	return filtered, total, nil
}

func auditReq(method, path string, tenantID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	ctx := req.Context()
	if tenantID != "" {
		ctx = middleware.WithTenant(ctx, tenantID)
	}
	return req.WithContext(ctx)
}

func TestAuditHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		tenantID   string
		seedData   func(*mockAuditLogStore)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:     "GET with tenant context returns 200",
			method:   http.MethodGet,
			path:     "/v1/audit-logs",
			tenantID: "tenant-1",
			seedData: func(ms *mockAuditLogStore) {
				ms.logs = append(ms.logs,
					&store.AuditLog{ID: 1, TenantID: "tenant-1", Action: "login"},
					&store.AuditLog{ID: 2, TenantID: "tenant-1", Action: "create"},
					&store.AuditLog{ID: 3, TenantID: "tenant-2", Action: "login"},
				)
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal: %v", err)
				}
				total, ok := resp["total"].(float64)
				if !ok {
					t.Fatal("expected total field in response")
				}
				if int(total) != 2 {
					t.Errorf("total = %d, want 2", int(total))
				}
			},
		},
		{
			name:       "POST returns 405",
			method:     http.MethodPost,
			path:       "/v1/audit-logs",
			tenantID:   "tenant-1",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "GET without tenant returns 400",
			method:     http.MethodGet,
			path:       "/v1/audit-logs",
			tenantID:   "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "pagination params are respected",
			method:   http.MethodGet,
			path:     "/v1/audit-logs?limit=1&offset=1",
			tenantID: "tenant-1",
			seedData: func(ms *mockAuditLogStore) {
				ms.logs = append(ms.logs,
					&store.AuditLog{ID: 1, TenantID: "tenant-1", Action: "a"},
					&store.AuditLog{ID: 2, TenantID: "tenant-1", Action: "b"},
					&store.AuditLog{ID: 3, TenantID: "tenant-1", Action: "c"},
				)
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal: %v", err)
				}
				logs, ok := resp["audit_logs"].([]any)
				if !ok {
					t.Fatal("expected audit_logs array")
				}
				if len(logs) != 1 {
					t.Errorf("audit_logs count = %d, want 1", len(logs))
				}
				offset, _ := resp["offset"].(float64)
				if int(offset) != 1 {
					t.Errorf("offset = %d, want 1", int(offset))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockAuditLogStore{}
			if tt.seedData != nil {
				tt.seedData(ms)
			}

			handler := NewAuditHandler(ms)
			rec := httptest.NewRecorder()
			req := auditReq(tt.method, tt.path, tt.tenantID)

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.checkBody != nil {
				tt.checkBody(t, rec.Body.Bytes())
			}
		})
	}
}
