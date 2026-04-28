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

	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// mockAPIKeyStore is an in-memory implementation of store.APIKeyStore.
type mockAPIKeyStore struct {
	keys   map[string]*store.APIKey
	nextID int
}

func newMockAPIKeyStore() *mockAPIKeyStore {
	return &mockAPIKeyStore{keys: make(map[string]*store.APIKey)}
}

func (m *mockAPIKeyStore) Create(_ context.Context, key *store.APIKey) error {
	if key.ID == "" {
		m.nextID++
		key.ID = fmt.Sprintf("key-%d", m.nextID)
	}
	m.keys[key.ID] = key
	return nil
}

func (m *mockAPIKeyStore) GetByHash(_ context.Context, hash string) (*store.APIKey, error) {
	for _, k := range m.keys {
		if k.KeyHash == hash {
			return k, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockAPIKeyStore) GetByID(_ context.Context, tenantID, id string) (*store.APIKey, error) {
	k, ok := m.keys[id]
	if !ok || k.TenantID != tenantID {
		return nil, fmt.Errorf("not found")
	}
	return k, nil
}

func (m *mockAPIKeyStore) List(_ context.Context, tenantID string) ([]*store.APIKey, error) {
	var result []*store.APIKey
	for _, k := range m.keys {
		if k.TenantID == tenantID {
			result = append(result, k)
		}
	}
	return result, nil
}

func (m *mockAPIKeyStore) Revoke(_ context.Context, tenantID, id string) error {
	k, ok := m.keys[id]
	if !ok || k.TenantID != tenantID {
		return fmt.Errorf("not found")
	}
	now := time.Now()
	k.RevokedAt = &now
	return nil
}

func apiKeyReq(method, path string, body any, tenantID string) *http.Request {
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
	return req.WithContext(ctx)
}

func TestAPIKeyHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		tenantID   string
		seedData   func(*mockAPIKeyStore)
		wantStatus int
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name:       "create with name returns 201",
			method:     http.MethodPost,
			path:       "/v1/api-keys",
			body:       map[string]string{"name": "my-key"},
			tenantID:   "tenant-1",
			wantStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var resp createKeyResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal: %v", err)
				}
				if resp.RawKey == "" {
					t.Error("expected raw key in response")
				}
				if resp.Name != "my-key" {
					t.Errorf("name = %q, want %q", resp.Name, "my-key")
				}
				if resp.ID == "" {
					t.Error("expected non-empty ID")
				}
			},
		},
		{
			name:       "create without name returns 400",
			method:     http.MethodPost,
			path:       "/v1/api-keys",
			body:       map[string]string{"name": ""},
			tenantID:   "tenant-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create without tenant context returns 400",
			method:     http.MethodPost,
			path:       "/v1/api-keys",
			body:       map[string]string{"name": "my-key"},
			tenantID:   "", // no tenant
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "list returns 200",
			method:   http.MethodGet,
			path:     "/v1/api-keys",
			tenantID: "tenant-1",
			seedData: func(ms *mockAPIKeyStore) {
				ms.keys["k1"] = &store.APIKey{ID: "k1", TenantID: "tenant-1", Name: "key-1"}
				ms.keys["k2"] = &store.APIKey{ID: "k2", TenantID: "tenant-1", Name: "key-2"}
			},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal: %v", err)
				}
				keys, ok := resp["api_keys"].([]any)
				if !ok {
					t.Fatal("expected api_keys array in response")
				}
				if len(keys) != 2 {
					t.Errorf("api_keys count = %d, want 2", len(keys))
				}
			},
		},
		{
			name:     "revoke with correct tenant returns 204",
			method:   http.MethodDelete,
			path:     "/v1/api-keys/k1",
			tenantID: "tenant-1",
			seedData: func(ms *mockAPIKeyStore) {
				ms.keys["k1"] = &store.APIKey{ID: "k1", TenantID: "tenant-1", Name: "key-1"}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:     "revoke with wrong tenant returns 403",
			method:   http.MethodDelete,
			path:     "/v1/api-keys/k1",
			tenantID: "tenant-2",
			seedData: func(ms *mockAPIKeyStore) {
				ms.keys["k1"] = &store.APIKey{ID: "k1", TenantID: "tenant-1", Name: "key-1"}
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := newMockAPIKeyStore()
			if tt.seedData != nil {
				tt.seedData(ms)
			}

			handler := NewAPIKeyHandler(ms)
			rec := httptest.NewRecorder()
			req := apiKeyReq(tt.method, tt.path, tt.body, tt.tenantID)

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
