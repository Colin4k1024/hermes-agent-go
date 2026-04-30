package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// mockSessionStore is a minimal in-memory implementation of store.SessionStore.
type mockSessionStore struct {
	sessions map[string]*store.Session
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{sessions: make(map[string]*store.Session)}
}

func (m *mockSessionStore) Create(_ context.Context, tenantID string, s *store.Session) error {
	s.TenantID = tenantID
	m.sessions[tenantID+"/"+s.ID] = s
	return nil
}

func (m *mockSessionStore) Get(_ context.Context, tenantID, sessionID string) (*store.Session, error) {
	s, ok := m.sessions[tenantID+"/"+sessionID]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return s, nil
}

func (m *mockSessionStore) End(_ context.Context, _, _ string, _ string) error { return nil }

func (m *mockSessionStore) List(_ context.Context, tenantID string, _ store.ListOptions) ([]*store.Session, int, error) {
	var result []*store.Session
	for _, s := range m.sessions {
		if s.TenantID == tenantID {
			result = append(result, s)
		}
	}
	return result, len(result), nil
}

func (m *mockSessionStore) Delete(_ context.Context, tenantID, sessionID string) error {
	delete(m.sessions, tenantID+"/"+sessionID)
	return nil
}

func (m *mockSessionStore) UpdateTokens(_ context.Context, _, _ string, _ store.TokenDelta) error {
	return nil
}

func (m *mockSessionStore) SetTitle(_ context.Context, _, _, _ string) error { return nil }

// mockMessageStore is a minimal in-memory implementation of store.MessageStore.
type mockMessageStore struct {
	messages map[string][]*store.Message
}

func newMockMessageStore() *mockMessageStore {
	return &mockMessageStore{messages: make(map[string][]*store.Message)}
}

func (m *mockMessageStore) Append(_ context.Context, _, sessionID string, msg *store.Message) (int64, error) {
	m.messages[sessionID] = append(m.messages[sessionID], msg)
	return int64(len(m.messages[sessionID])), nil
}

func (m *mockMessageStore) List(_ context.Context, _, sessionID string, limit, _ int) ([]*store.Message, error) {
	msgs := m.messages[sessionID]
	if limit > 0 && limit < len(msgs) {
		msgs = msgs[:limit]
	}
	return msgs, nil
}

func (m *mockMessageStore) Search(_ context.Context, _, _ string, _ int) ([]*store.SearchResult, error) {
	return nil, nil
}

func (m *mockMessageStore) CountBySession(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}

// mockGDPRStore wraps session/message stores to satisfy store.Store for GDPR tests.
type mockGDPRStore struct {
	ss *mockSessionStore
	ms *mockMessageStore
	al *mockGDPRAuditStore
}

func (m *mockGDPRStore) Sessions() store.SessionStore    { return m.ss }
func (m *mockGDPRStore) Messages() store.MessageStore    { return m.ms }
func (m *mockGDPRStore) Users() store.UserStore          { return nil }
func (m *mockGDPRStore) Tenants() store.TenantStore      { return &mockGDPRTenantStore{} }
func (m *mockGDPRStore) AuditLogs() store.AuditLogStore  { return m.al }
func (m *mockGDPRStore) APIKeys() store.APIKeyStore           { return nil }
func (m *mockGDPRStore) Memories() store.MemoryStore           { return nil }
func (m *mockGDPRStore) UserProfiles() store.UserProfileStore  { return nil }
func (m *mockGDPRStore) CronJobs() store.CronJobStore          { return nil }
func (m *mockGDPRStore) Roles() store.RoleStore                { return nil }
func (m *mockGDPRStore) Close() error                          { return nil }
func (m *mockGDPRStore) Migrate(_ context.Context) error       { return nil }

type mockGDPRAuditStore struct{}

func (m *mockGDPRAuditStore) Append(_ context.Context, _ *store.AuditLog) error { return nil }
func (m *mockGDPRAuditStore) List(_ context.Context, _ string, _ store.AuditListOptions) ([]*store.AuditLog, int, error) {
	return nil, 0, nil
}
func (m *mockGDPRAuditStore) DeleteByTenant(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

type mockGDPRTenantStore struct{}

func (m *mockGDPRTenantStore) Create(_ context.Context, _ *store.Tenant) error { return nil }
func (m *mockGDPRTenantStore) Get(_ context.Context, _ string) (*store.Tenant, error) {
	return nil, nil
}
func (m *mockGDPRTenantStore) Update(_ context.Context, _ *store.Tenant) error { return nil }
func (m *mockGDPRTenantStore) Delete(_ context.Context, _ string) error        { return nil }
func (m *mockGDPRTenantStore) List(_ context.Context, _ store.ListOptions) ([]*store.Tenant, int, error) {
	return nil, 0, nil
}
func (m *mockGDPRTenantStore) ListDeleted(_ context.Context, _ time.Time) ([]*store.Tenant, error) {
	return nil, nil
}
func (m *mockGDPRTenantStore) HardDelete(_ context.Context, _ string) error { return nil }
func (m *mockGDPRTenantStore) Restore(_ context.Context, _ string) error    { return nil }

func gdprReq(method, path string, tenantID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	ctx := req.Context()
	if tenantID != "" {
		ctx = middleware.WithTenant(ctx, tenantID)
	}
	return req.WithContext(ctx)
}

func TestGDPRExportHandler(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		tenantID    string
		seedData    func(*mockSessionStore, *mockMessageStore)
		wantStatus  int
		checkHeader func(t *testing.T, h http.Header)
	}{
		{
			name:     "GET export with tenant returns 200",
			method:   http.MethodGet,
			tenantID: "tenant-1",
			seedData: func(ss *mockSessionStore, ms *mockMessageStore) {
				ss.sessions["tenant-1/s1"] = &store.Session{ID: "s1", TenantID: "tenant-1"}
				ms.messages["s1"] = []*store.Message{
					{ID: 1, SessionID: "s1", Content: "hello"},
				}
			},
			wantStatus: http.StatusOK,
			checkHeader: func(t *testing.T, h http.Header) {
				cd := h.Get("Content-Disposition")
				if cd == "" {
					t.Error("expected Content-Disposition header")
				}
			},
		},
		{
			name:       "GET export without tenant returns 400",
			method:     http.MethodGet,
			tenantID:   "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "POST export returns 405",
			method:     http.MethodPost,
			tenantID:   "tenant-1",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := newMockSessionStore()
			ms := newMockMessageStore()
			if tt.seedData != nil {
				tt.seedData(ss, ms)
			}

			s := &mockGDPRStore{ss: ss, ms: ms, al: &mockGDPRAuditStore{}}
			handler := NewGDPRHandler(s, nil).ExportHandler()
			rec := httptest.NewRecorder()
			req := gdprReq(tt.method, "/v1/gdpr/export", tt.tenantID)

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if tt.checkHeader != nil {
				tt.checkHeader(t, rec.Header())
			}
		})
	}
}

func TestGDPRDeleteHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		tenantID   string
		seedData   func(*mockSessionStore)
		wantStatus int
	}{
		{
			name:     "DELETE with tenant and sessions returns 204",
			method:   http.MethodDelete,
			tenantID: "tenant-1",
			seedData: func(ss *mockSessionStore) {
				ss.sessions["tenant-1/s1"] = &store.Session{ID: "s1", TenantID: "tenant-1"}
				ss.sessions["tenant-1/s2"] = &store.Session{ID: "s2", TenantID: "tenant-1"}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "DELETE with no sessions returns 204",
			method:     http.MethodDelete,
			tenantID:   "tenant-1",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "DELETE without tenant returns 400",
			method:     http.MethodDelete,
			tenantID:   "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET on delete endpoint returns 405",
			method:     http.MethodGet,
			tenantID:   "tenant-1",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := newMockSessionStore()
			if tt.seedData != nil {
				tt.seedData(ss)
			}

			s := &mockGDPRStore{ss: ss, ms: newMockMessageStore(), al: &mockGDPRAuditStore{}}
			handler := NewGDPRHandler(s, nil).DeleteHandler()
			rec := httptest.NewRecorder()
			req := gdprReq(tt.method, "/v1/gdpr/data", tt.tenantID)

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
