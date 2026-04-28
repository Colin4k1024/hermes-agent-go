package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// mockAPIKeyStore implements store.APIKeyStore for testing.
type mockAPIKeyStore struct {
	keys map[string]*store.APIKey // keyed by hash
}

func (m *mockAPIKeyStore) Create(_ context.Context, _ *store.APIKey) error {
	return errors.New("not implemented")
}

func (m *mockAPIKeyStore) GetByHash(_ context.Context, hash string) (*store.APIKey, error) {
	key, ok := m.keys[hash]
	if !ok {
		return nil, errors.New("not found")
	}
	return key, nil
}

func (m *mockAPIKeyStore) GetByID(_ context.Context, _, _ string) (*store.APIKey, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAPIKeyStore) List(_ context.Context, _ string) ([]*store.APIKey, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAPIKeyStore) Revoke(_ context.Context, _, _ string) error {
	return errors.New("not implemented")
}

func hashTestKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func TestAPIKeyExtractor(t *testing.T) {
	now := time.Now()
	pastTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	const rawKey = "hk_test_abc123"

	validKey := &store.APIKey{
		ID:        "key-1",
		TenantID:  "tenant-1",
		Name:      "test-key",
		KeyHash:   hashTestKey(rawKey),
		Roles:     []string{"user", "operator"},
		ExpiresAt: &futureTime,
	}

	revokedKey := &store.APIKey{
		ID:        "key-2",
		TenantID:  "tenant-1",
		Name:      "revoked-key",
		KeyHash:   hashTestKey("revoked-key-raw"),
		Roles:     []string{"user"},
		RevokedAt: &now,
	}

	expiredKey := &store.APIKey{
		ID:        "key-3",
		TenantID:  "tenant-1",
		Name:      "expired-key",
		KeyHash:   hashTestKey("expired-key-raw"),
		Roles:     []string{"user"},
		ExpiresAt: &pastTime,
	}

	mockStore := &mockAPIKeyStore{
		keys: map[string]*store.APIKey{
			hashTestKey(rawKey):            validKey,
			hashTestKey("revoked-key-raw"): revokedKey,
			hashTestKey("expired-key-raw"): expiredKey,
		},
	}

	extractor := NewAPIKeyExtractor(mockStore)

	tests := []struct {
		name       string
		authHeader string
		wantNil    bool
		wantErr    bool
		wantErrMsg string
		wantID     string
		wantTenant string
		wantRoles  []string
		wantMethod string
	}{
		{
			name:       "valid key",
			authHeader: "Bearer " + rawKey,
			wantNil:    false,
			wantErr:    false,
			wantID:     "key-1",
			wantTenant: "tenant-1",
			wantRoles:  []string{"user", "operator"},
			wantMethod: "api_key",
		},
		{
			name:       "key not found",
			authHeader: "Bearer unknown-key",
			wantNil:    true,
			wantErr:    false,
		},
		{
			name:       "revoked key",
			authHeader: "Bearer revoked-key-raw",
			wantNil:    false,
			wantErr:    true,
			wantErrMsg: "api key revoked",
		},
		{
			name:       "expired key",
			authHeader: "Bearer expired-key-raw",
			wantNil:    false,
			wantErr:    true,
			wantErrMsg: "api key expired",
		},
		{
			name:       "no authorization header",
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
	}

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
				if tt.wantErrMsg != "" && err.Error() != tt.wantErrMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.wantErrMsg)
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
			if ac.AuthMethod != tt.wantMethod {
				t.Errorf("AuthMethod = %q, want %q", ac.AuthMethod, tt.wantMethod)
			}
			if len(ac.Roles) != len(tt.wantRoles) {
				t.Errorf("Roles = %v, want %v", ac.Roles, tt.wantRoles)
			} else {
				for i, r := range ac.Roles {
					if r != tt.wantRoles[i] {
						t.Errorf("Roles[%d] = %q, want %q", i, r, tt.wantRoles[i])
					}
				}
			}
		})
	}
}

func TestHashKey(t *testing.T) {
	input := "test-api-key"
	expected := sha256.Sum256([]byte(input))
	expectedHex := hex.EncodeToString(expected[:])

	got := HashKey(input)
	if got != expectedHex {
		t.Errorf("HashKey(%q) = %q, want %q", input, got, expectedHex)
	}

	// Verify determinism.
	got2 := HashKey(input)
	if got != got2 {
		t.Errorf("HashKey is not deterministic: %q != %q", got, got2)
	}

	// Different inputs produce different hashes.
	other := HashKey("different-key")
	if got == other {
		t.Errorf("HashKey produced same hash for different inputs")
	}
}
