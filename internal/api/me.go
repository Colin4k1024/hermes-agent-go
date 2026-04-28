package api

import (
	"encoding/json"
	"net/http"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// MeHandler returns the current authenticated identity and tenant info.
type MeHandler struct {
	store store.Store
}

func NewMeHandler(s store.Store) *MeHandler {
	return &MeHandler{store: s}
}

// meResponse mirrors the fields needed by the SaaS admin SPA.
type meResponse struct {
	TenantID     string   `json:"tenant_id"`
	Identity     string   `json:"identity"`
	Roles        []string `json:"roles"`
	AuthMethod   string   `json:"auth_method"`
	Plan         string   `json:"plan,omitempty"`
	RateLimitRPM int      `json:"rate_limit_rpm,omitempty"`
	MaxSessions  int      `json:"max_sessions,omitempty"`
}

func (h *MeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resp := meResponse{
		TenantID:   ac.TenantID,
		Identity:   ac.Identity,
		Roles:      ac.Roles,
		AuthMethod: ac.AuthMethod,
	}

	// Enrich with tenant config if available.
	if h.store != nil && ac.TenantID != "" {
		t, err := h.store.Tenants().Get(r.Context(), ac.TenantID)
		if err == nil && t != nil {
			resp.Plan = t.Plan
			resp.RateLimitRPM = t.RateLimitRPM
			resp.MaxSessions = t.MaxSessions
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
