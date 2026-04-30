package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// mockTenantStore is an in-memory implementation of store.TenantStore.
type mockTenantStore struct {
	tenants map[string]*store.Tenant
	nextID  int
}

func newMockTenantStore() *mockTenantStore {
	return &mockTenantStore{tenants: make(map[string]*store.Tenant)}
}

func (m *mockTenantStore) Create(_ context.Context, t *store.Tenant) error {
	if t.ID == "" {
		m.nextID++
		t.ID = fmt.Sprintf("t%d", m.nextID)
	}
	m.tenants[t.ID] = t
	return nil
}

func (m *mockTenantStore) Get(_ context.Context, id string) (*store.Tenant, error) {
	t, ok := m.tenants[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return t, nil
}

func (m *mockTenantStore) Update(_ context.Context, t *store.Tenant) error {
	m.tenants[t.ID] = t
	return nil
}

func (m *mockTenantStore) Delete(_ context.Context, id string) error {
	delete(m.tenants, id)
	return nil
}

func (m *mockTenantStore) List(_ context.Context, _ store.ListOptions) ([]*store.Tenant, int, error) {
	var all []*store.Tenant
	for _, t := range m.tenants {
		all = append(all, t)
	}
	return all, len(all), nil
}

func (m *mockTenantStore) ListDeleted(_ context.Context, _ time.Time) ([]*store.Tenant, error) {
	return nil, nil
}
func (m *mockTenantStore) HardDelete(_ context.Context, _ string) error { return nil }
func (m *mockTenantStore) Restore(_ context.Context, _ string) error    { return nil }

func tenantReq(method, path string, body any, tenantID string, roles []string) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")

	ctx := req.Context()
	if tenantID != "" {
		ctx = middleware.WithTenant(ctx, tenantID)
	}
	if roles != nil {
		ac := &auth.AuthContext{
			Identity:   "test-user",
			TenantID:   tenantID,
			Roles:      roles,
			AuthMethod: "test",
		}
		ctx = auth.WithContext(ctx, ac)
	}
	return req.WithContext(ctx)
}

func TestTenantHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		tenantID   string
		roles      []string
		seedData   func(*mockTenantStore)
		wantStatus int
	}{
		{
			name:       "create as admin returns 201",
			method:     http.MethodPost,
			path:       "/v1/tenants",
			body:       map[string]string{"name": "Acme Corp"},
			tenantID:   "admin-tenant",
			roles:      []string{"admin"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "create as non-admin returns 403",
			method:     http.MethodPost,
			path:       "/v1/tenants",
			body:       map[string]string{"name": "Acme Corp"},
			tenantID:   "user-tenant",
			roles:      []string{"user"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "create with empty name returns 400",
			method:     http.MethodPost,
			path:       "/v1/tenants",
			body:       map[string]string{"name": ""},
			tenantID:   "admin-tenant",
			roles:      []string{"admin"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "get as admin returns 200",
			method:   http.MethodGet,
			path:     "/v1/tenants/t1",
			tenantID: "admin-tenant",
			roles:    []string{"admin"},
			seedData: func(ms *mockTenantStore) {
				ms.tenants["t1"] = &store.Tenant{ID: "t1", Name: "Tenant One"}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "get as owner tenant returns 200",
			method:   http.MethodGet,
			path:     "/v1/tenants/t1",
			tenantID: "t1",
			roles:    []string{"user"},
			seedData: func(ms *mockTenantStore) {
				ms.tenants["t1"] = &store.Tenant{ID: "t1", Name: "Tenant One"}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "get as different tenant returns 403",
			method:   http.MethodGet,
			path:     "/v1/tenants/t1",
			tenantID: "t2",
			roles:    []string{"user"},
			seedData: func(ms *mockTenantStore) {
				ms.tenants["t1"] = &store.Tenant{ID: "t1", Name: "Tenant One"}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:     "list as admin returns 200",
			method:   http.MethodGet,
			path:     "/v1/tenants",
			tenantID: "admin-tenant",
			roles:    []string{"admin"},
			seedData: func(ms *mockTenantStore) {
				ms.tenants["t1"] = &store.Tenant{ID: "t1", Name: "One"}
				ms.tenants["t2"] = &store.Tenant{ID: "t2", Name: "Two"}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "list as non-admin returns 403",
			method:     http.MethodGet,
			path:       "/v1/tenants",
			tenantID:   "user-tenant",
			roles:      []string{"user"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:     "delete as admin returns 202",
			method:   http.MethodDelete,
			path:     "/v1/tenants/t1",
			tenantID: "admin-tenant",
			roles:    []string{"admin"},
			seedData: func(ms *mockTenantStore) {
				ms.tenants["t1"] = &store.Tenant{ID: "t1", Name: "Tenant One"}
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:     "delete as non-admin returns 403",
			method:   http.MethodDelete,
			path:     "/v1/tenants/t1",
			tenantID: "user-tenant",
			roles:    []string{"user"},
			seedData: func(ms *mockTenantStore) {
				ms.tenants["t1"] = &store.Tenant{ID: "t1", Name: "Tenant One"}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:     "update as owner returns 200",
			method:   http.MethodPut,
			path:     "/v1/tenants/t1",
			body:     map[string]string{"name": "Updated"},
			tenantID: "t1",
			roles:    []string{"user"},
			seedData: func(ms *mockTenantStore) {
				ms.tenants["t1"] = &store.Tenant{ID: "t1", Name: "Original"}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "update as different tenant returns 403",
			method:   http.MethodPut,
			path:     "/v1/tenants/t1",
			body:     map[string]string{"name": "Hacked"},
			tenantID: "t2",
			roles:    []string{"user"},
			seedData: func(ms *mockTenantStore) {
				ms.tenants["t1"] = &store.Tenant{ID: "t1", Name: "Original"}
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := newMockTenantStore()
			if tt.seedData != nil {
				tt.seedData(ms)
			}

			handler := NewTenantHandler(ms)
			rec := httptest.NewRecorder()
			req := tenantReq(tt.method, tt.path, tt.body, tt.tenantID, tt.roles)

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
