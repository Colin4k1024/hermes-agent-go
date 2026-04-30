package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// mockMeStore satisfies the minimal store.Store needed by MeHandler.
type mockMeStore struct {
	tenant *store.Tenant
}

func (m *mockMeStore) Tenants() store.TenantStore {
	return &mockMeTenantStore{tenant: m.tenant}
}

func (m *mockMeStore) Sessions() store.SessionStore    { return nil }
func (m *mockMeStore) Messages() store.MessageStore    { return nil }
func (m *mockMeStore) Users() store.UserStore          { return nil }
func (m *mockMeStore) AuditLogs() store.AuditLogStore  { return nil }
func (m *mockMeStore) APIKeys() store.APIKeyStore           { return nil }
func (m *mockMeStore) Memories() store.MemoryStore           { return nil }
func (m *mockMeStore) UserProfiles() store.UserProfileStore  { return nil }
func (m *mockMeStore) CronJobs() store.CronJobStore          { return nil }
func (m *mockMeStore) Roles() store.RoleStore                { return nil }
func (m *mockMeStore) Close() error                          { return nil }
func (m *mockMeStore) Migrate(_ context.Context) error       { return nil }

type mockMeTenantStore struct{ tenant *store.Tenant }

func (m *mockMeTenantStore) Get(_ context.Context, _ string) (*store.Tenant, error) {
	return m.tenant, nil
}
func (m *mockMeTenantStore) Create(_ context.Context, t *store.Tenant) error { return nil }
func (m *mockMeTenantStore) Update(_ context.Context, t *store.Tenant) error { return nil }
func (m *mockMeTenantStore) Delete(_ context.Context, _ string) error        { return nil }
func (m *mockMeTenantStore) List(_ context.Context, _ store.ListOptions) ([]*store.Tenant, int, error) {
	return nil, 0, nil
}
func (m *mockMeTenantStore) ListDeleted(_ context.Context, _ time.Time) ([]*store.Tenant, error) {
	return nil, nil
}
func (m *mockMeTenantStore) HardDelete(_ context.Context, _ string) error { return nil }
func (m *mockMeTenantStore) Restore(_ context.Context, _ string) error    { return nil }

// authContextReq creates a request with an AuthContext in the context.
func authContextReq(method, path string, ac *auth.AuthContext) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	return req.WithContext(auth.WithContext(req.Context(), ac))
}

func TestMeHandler_Unauthorized(t *testing.T) {
	h := NewMeHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMeHandler_OK_WithoutStore(t *testing.T) {
	h := NewMeHandler(nil)
	ac := &auth.AuthContext{
		TenantID:   "tenant-1",
		Identity:   "user-1",
		Roles:      []string{"user"},
		AuthMethod: "jwt",
	}
	req := authContextReq(http.MethodGet, "/v1/me", ac)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp meResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.TenantID != "tenant-1" {
		t.Errorf("TenantID: want tenant-1, got %s", resp.TenantID)
	}
	if resp.Identity != "user-1" {
		t.Errorf("Identity: want user-1, got %s", resp.Identity)
	}
	if len(resp.Roles) != 1 || resp.Roles[0] != "user" {
		t.Errorf("Roles: got %v", resp.Roles)
	}
	if resp.AuthMethod != "jwt" {
		t.Errorf("AuthMethod: want jwt, got %s", resp.AuthMethod)
	}
	// Without store, tenant enrichment fields stay zero.
	if resp.Plan != "" || resp.RateLimitRPM != 0 || resp.MaxSessions != 0 {
		t.Errorf("tenant fields should be zero without store: plan=%s rpm=%d max=%d",
			resp.Plan, resp.RateLimitRPM, resp.MaxSessions)
	}
}

func TestMeHandler_OK_WithStore(t *testing.T) {
	s := &mockMeStore{tenant: &store.Tenant{
		ID:           "tenant-abc",
		Name:         "Test Tenant",
		Plan:         "enterprise",
		RateLimitRPM: 500,
		MaxSessions:  50,
	}}
	h := NewMeHandler(s)
	ac := &auth.AuthContext{
		TenantID:   "tenant-abc",
		Identity:   "admin-1",
		Roles:      []string{"admin", "user"},
		AuthMethod: "api_key",
	}
	req := authContextReq(http.MethodGet, "/v1/me", ac)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp meResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.TenantID != "tenant-abc" {
		t.Errorf("TenantID: want tenant-abc, got %s", resp.TenantID)
	}
	if resp.Plan != "enterprise" {
		t.Errorf("Plan: want enterprise, got %s", resp.Plan)
	}
	if resp.RateLimitRPM != 500 {
		t.Errorf("RateLimitRPM: want 500, got %d", resp.RateLimitRPM)
	}
	if resp.MaxSessions != 50 {
		t.Errorf("MaxSessions: want 50, got %d", resp.MaxSessions)
	}
}

func TestMeHandler_MethodNotAllowed(t *testing.T) {
	h := NewMeHandler(nil)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/v1/me", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, w.Code)
		}
	}
}

func TestMeHandler_StoreTenantNotFound(t *testing.T) {
	// Store returns not-found for any tenant lookup — should not panic.
	s := &mockMeStore{tenant: nil}
	h := NewMeHandler(s)
	ac := &auth.AuthContext{
		TenantID:   "unknown",
		Identity:   "user-1",
		Roles:      []string{"user"},
		AuthMethod: "api_key",
	}
	req := authContextReq(http.MethodGet, "/v1/me", ac)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (graceful degradation), got %d", w.Code)
	}
}
