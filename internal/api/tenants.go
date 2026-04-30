package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// TenantHandlerOption configures a TenantHandler.
type TenantHandlerOption func(*TenantHandler)

// WithOnTenantCreated registers a callback invoked asynchronously after tenant creation.
func WithOnTenantCreated(fn func(ctx context.Context, tenantID string)) TenantHandlerOption {
	return func(h *TenantHandler) { h.onCreated = fn }
}

// TenantHandler serves CRUD endpoints for /v1/tenants.
type TenantHandler struct {
	store     store.TenantStore
	onCreated func(ctx context.Context, tenantID string)
}

func NewTenantHandler(s store.TenantStore, opts ...TenantHandlerOption) *TenantHandler {
	h := &TenantHandler{store: s}
	for _, o := range opts {
		o(h)
	}
	return h
}

func (h *TenantHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/tenants")
	path = strings.TrimPrefix(path, "/")

	switch {
	case r.Method == http.MethodPost && path == "":
		h.create(w, r)
	case r.Method == http.MethodGet && path == "":
		h.list(w, r)
	case r.Method == http.MethodGet && path != "":
		h.get(w, r, path)
	case r.Method == http.MethodPut && path != "":
		h.update(w, r, path)
	case r.Method == http.MethodDelete && path != "":
		h.delete(w, r, path)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func isAdmin(r *http.Request) bool {
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		return false
	}
	for _, role := range ac.Roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

func (h *TenantHandler) requireTenantAccess(r *http.Request, targetTenantID string) bool {
	if isAdmin(r) {
		return true
	}
	callerTenant := middleware.TenantFromContext(r.Context())
	return callerTenant == targetTenantID
}

func (h *TenantHandler) create(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}
	var t store.Tenant
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if t.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if err := h.store.Create(r.Context(), &t); err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	if h.onCreated != nil {
		go func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.onCreated(ctx, id)
		}(t.ID)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(t)
}

func (h *TenantHandler) get(w http.ResponseWriter, r *http.Request, id string) {
	if !h.requireTenantAccess(r, id) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	t, err := h.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

func (h *TenantHandler) list(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tenants, total, err := h.store.List(r.Context(), store.ListOptions{Limit: limit, Offset: offset})
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tenants": tenants,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

func (h *TenantHandler) update(w http.ResponseWriter, r *http.Request, id string) {
	if !h.requireTenantAccess(r, id) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var t store.Tenant
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	t.ID = id
	if err := h.store.Update(r.Context(), &t); err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

func (h *TenantHandler) delete(w http.ResponseWriter, r *http.Request, id string) {
	if !isAdmin(r) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}
	if err := h.store.Delete(r.Context(), id); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "scheduled",
		"message": "Tenant soft-deleted. Data will be purged after retention period.",
	})
}
