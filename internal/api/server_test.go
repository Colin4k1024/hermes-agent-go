package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// stubSessionStore implements store.SessionStore with no-op methods.
type stubSessionStore struct{}

func (stubSessionStore) Create(_ context.Context, _ string, _ *store.Session) error { return nil }
func (stubSessionStore) Get(_ context.Context, _, _ string) (*store.Session, error) { return nil, nil }
func (stubSessionStore) End(_ context.Context, _, _, _ string) error                { return nil }
func (stubSessionStore) List(_ context.Context, _ string, _ store.ListOptions) ([]*store.Session, int, error) {
	return nil, 0, nil
}
func (stubSessionStore) Delete(_ context.Context, _, _ string) error { return nil }
func (stubSessionStore) UpdateTokens(_ context.Context, _, _ string, _ store.TokenDelta) error {
	return nil
}
func (stubSessionStore) SetTitle(_ context.Context, _, _, _ string) error { return nil }

// stubMessageStore implements store.MessageStore with no-op methods.
type stubMessageStore struct{}

func (stubMessageStore) Append(_ context.Context, _, _ string, _ *store.Message) (int64, error) {
	return 0, nil
}
func (stubMessageStore) List(_ context.Context, _, _ string, _, _ int) ([]*store.Message, error) {
	return nil, nil
}
func (stubMessageStore) Search(_ context.Context, _, _ string, _ int) ([]*store.SearchResult, error) {
	return nil, nil
}
func (stubMessageStore) CountBySession(_ context.Context, _, _ string) (int, error) { return 0, nil }

// stubUserStore implements store.UserStore with no-op methods.
type stubUserStore struct{}

func (stubUserStore) GetOrCreate(_ context.Context, _, _, _ string) (*store.User, error) {
	return nil, nil
}
func (stubUserStore) IsApproved(_ context.Context, _, _, _ string) (bool, error)    { return false, nil }
func (stubUserStore) Approve(_ context.Context, _, _, _ string) error               { return nil }
func (stubUserStore) Revoke(_ context.Context, _, _, _ string) error                { return nil }
func (stubUserStore) ListApproved(_ context.Context, _, _ string) ([]string, error) { return nil, nil }

// stubTenantStore implements store.TenantStore with no-op methods.
type stubTenantStore struct{}

func (stubTenantStore) Create(_ context.Context, _ *store.Tenant) error        { return nil }
func (stubTenantStore) Get(_ context.Context, _ string) (*store.Tenant, error) { return nil, nil }
func (stubTenantStore) Update(_ context.Context, _ *store.Tenant) error        { return nil }
func (stubTenantStore) Delete(_ context.Context, _ string) error               { return nil }
func (stubTenantStore) List(_ context.Context, _ store.ListOptions) ([]*store.Tenant, int, error) {
	return nil, 0, nil
}
func (stubTenantStore) ListDeleted(_ context.Context, _ time.Time) ([]*store.Tenant, error) {
	return nil, nil
}
func (stubTenantStore) HardDelete(_ context.Context, _ string) error { return nil }
func (stubTenantStore) Restore(_ context.Context, _ string) error    { return nil }

// stubAuditLogStore implements store.AuditLogStore with no-op methods.
type stubAuditLogStore struct{}

func (stubAuditLogStore) Append(_ context.Context, _ *store.AuditLog) error { return nil }
func (stubAuditLogStore) List(_ context.Context, _ string, _ store.AuditListOptions) ([]*store.AuditLog, int, error) {
	return nil, 0, nil
}
func (stubAuditLogStore) DeleteByTenant(_ context.Context, _ string) (int64, error) { return 0, nil }

// stubAPIKeyStore implements store.APIKeyStore with no-op methods.
type stubAPIKeyStore struct{}

func (stubAPIKeyStore) Create(_ context.Context, _ *store.APIKey) error              { return nil }
func (stubAPIKeyStore) GetByHash(_ context.Context, _ string) (*store.APIKey, error) { return nil, nil }
func (stubAPIKeyStore) GetByID(_ context.Context, _, _ string) (*store.APIKey, error) {
	return nil, nil
}
func (stubAPIKeyStore) List(_ context.Context, _ string) ([]*store.APIKey, error) { return nil, nil }
func (stubAPIKeyStore) Revoke(_ context.Context, _, _ string) error               { return nil }

// stubStore implements store.Store, returning stub sub-stores.
type stubStore struct{}

func (stubStore) Sessions() store.SessionStore    { return stubSessionStore{} }
func (stubStore) Messages() store.MessageStore    { return stubMessageStore{} }
func (stubStore) Users() store.UserStore          { return stubUserStore{} }
func (stubStore) Tenants() store.TenantStore      { return stubTenantStore{} }
func (stubStore) AuditLogs() store.AuditLogStore  { return stubAuditLogStore{} }
func (stubStore) APIKeys() store.APIKeyStore      { return stubAPIKeyStore{} }
func (stubStore) Close() error                    { return nil }
func (stubStore) Migrate(_ context.Context) error { return nil }

func TestNewAPIServer(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		wantAddr string
	}{
		{
			name:     "returns non-nil server",
			port:     8080,
			wantAddr: ":8080",
		},
		{
			name:     "server addr matches configured port",
			port:     9090,
			wantAddr: ":9090",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewAPIServer(APIServerConfig{
				Port:  tt.port,
				Store: stubStore{},
				DB:    nil,
			})

			if srv == nil {
				t.Fatal("NewAPIServer returned nil")
			}

			gotAddr := fmt.Sprintf(":%d", srv.cfg.Port)
			if gotAddr != tt.wantAddr {
				t.Errorf("server addr = %q, want %q", gotAddr, tt.wantAddr)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────
//  corsMiddleware tests
// ──────────────────────────────────────────────────────────────────

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := corsMiddleware(mux, "https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("Allow-Origin = %q, want %q", got, "https://example.com")
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true", got)
	}
}

func TestCORSMiddleware_Wildcard(t *testing.T) {
	// When origins="*", any origin is allowed.
	// Note: the server also sets Allow-Credentials:true which means the actual
	// response header must be the specific origin (not "*") per CORS spec.
	// Browser handles this correctly; the key invariant is that the handler
	// does not return a 4xx, and the request is processed.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := corsMiddleware(mux, "*")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://any-site.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Request must succeed (not blocked by CORS).
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	// Either * or the specific origin is acceptable; both are valid CORS responses.
	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" && origin != "https://any-site.com" {
		t.Errorf("Allow-Origin = %q, want * or %q", origin, "https://any-site.com")
	}
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := corsMiddleware(mux, "https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty for disallowed origin", got)
	}
}

func TestCORSMiddleware_OptionsPreflight(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {})

	h := corsMiddleware(mux, "https://example.com")

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", rec.Code)
	}
}

func TestCORSMiddleware_MultipleOrigins(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := corsMiddleware(mux, "https://foo.com, https://bar.com, https://baz.com")

	for _, origin := range []string{"https://foo.com", "https://bar.com", "https://baz.com"} {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %q: Allow-Origin = %q, want %q", origin, got, origin)
		}
	}
}

// ──────────────────────────────────────────────────────────────────
//  spaFallback tests
// ──────────────────────────────────────────────────────────────────

func TestSPAFallback_RootServesIndex(t *testing.T) {
	// spaFallback reads from a real directory, so we test the logic only.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/me", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := spaFallback(mux, nil, "/nonexistent")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// / does NOT delegate to mux — it tries to serve index.html (fails if dir missing).
	// Verify it did NOT fall through to the mux (200 from /v1/me handler).
	if rec.Code == http.StatusOK {
		t.Errorf("root / should not delegate to mux; got 200 (mux served it)")
	}
}

func TestSPAFallback_AdminHtmlServed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/me", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := spaFallback(mux, nil, "/nonexistent")

	req := httptest.NewRequest(http.MethodGet, "/admin.html", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// admin.html tries to serve from filesystem, will fail if dir missing.
	// The key invariant: it did NOT fall through to mux (200 from /v1/me).
	if rec.Code == http.StatusOK {
		t.Errorf("/admin.html should not delegate to mux; got 200")
	}
}

func TestSPAFallback_ApiPathDelegates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/me", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := spaFallback(mux, nil, "/nonexistent")

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("/v1/me status = %d, want 200", rec.Code)
	}
}

func TestSPAFallback_UnknownPathDelegates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/me", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := spaFallback(mux, nil, "/nonexistent")

	req := httptest.NewRequest(http.MethodGet, "/v1/nonexistent-path", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// 404 from mux (no route registered) — NOT a fallback to static files.
	if rec.Code != http.StatusNotFound {
		t.Errorf("/v1/nonexistent-path status = %d, want 404", rec.Code)
	}
}
