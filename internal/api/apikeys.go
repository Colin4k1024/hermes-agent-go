package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// APIKeyHandler serves lifecycle endpoints for /v1/api-keys.
type APIKeyHandler struct {
	store store.APIKeyStore
}

func NewAPIKeyHandler(s store.APIKeyStore) *APIKeyHandler {
	return &APIKeyHandler{store: s}
}

func (h *APIKeyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/api-keys")
	path = strings.TrimPrefix(path, "/")

	switch {
	case r.Method == http.MethodPost && path == "":
		h.create(w, r)
	case r.Method == http.MethodGet && path == "":
		h.list(w, r)
	case r.Method == http.MethodDelete && path != "":
		h.revoke(w, r, path)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type createKeyRequest struct {
	TenantID string   `json:"tenant_id"`
	Name     string   `json:"name"`
	Roles    []string `json:"roles,omitempty"`
}

type createKeyResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
	RawKey string `json:"key"` // only returned on creation
}

func (h *APIKeyHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	// Admin callers pass tenant_id in body; otherwise use extracted tenant context.
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = middleware.TenantFromContext(r.Context())
	}
	if tenantID == "" {
		http.Error(w, "tenant context required", http.StatusBadRequest)
		return
	}

	rawKey := generateRawKey()
	prefix := rawKey[:8]
	roles := req.Roles
	if len(roles) == 0 {
		roles = []string{"user"}
	}

	key := &store.APIKey{
		TenantID: tenantID,
		Name:     req.Name,
		KeyHash:  auth.HashKey(rawKey),
		Prefix:   prefix,
		Roles:    roles,
	}

	if err := h.store.Create(r.Context(), key); err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createKeyResponse{
		ID:     key.ID,
		Name:   key.Name,
		Prefix: prefix,
		RawKey: rawKey,
	})
}

func (h *APIKeyHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	keys, err := h.store.List(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"api_keys": keys, "count": len(keys)})
}

func (h *APIKeyHandler) revoke(w http.ResponseWriter, r *http.Request, id string) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == "" {
		http.Error(w, "tenant context required", http.StatusBadRequest)
		return
	}
	if _, err := h.store.GetByID(r.Context(), tenantID, id); err != nil {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}
	if err := h.store.Revoke(r.Context(), tenantID, id); err != nil {
		http.Error(w, "revoke failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func generateRawKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "hk_" + hex.EncodeToString(b)
}
