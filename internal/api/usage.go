package api

import (
	"encoding/json"
	"net/http"

	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// UsageHandler serves GET /v1/usage for billing/usage summary per tenant.
type UsageHandler struct {
	sessions store.SessionStore
	messages store.MessageStore
}

func NewUsageHandler(sessions store.SessionStore, messages store.MessageStore) *UsageHandler {
	return &UsageHandler{sessions: sessions, messages: messages}
}

func (h *UsageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == "" {
		http.Error(w, "tenant context required", http.StatusBadRequest)
		return
	}

	sessions, total, err := h.sessions.List(r.Context(), tenantID, store.ListOptions{Limit: 1})
	if err != nil {
		http.Error(w, "failed to fetch sessions", http.StatusInternalServerError)
		return
	}

	var totalInput, totalOutput int
	if len(sessions) > 0 {
		allSessions, _, _ := h.sessions.List(r.Context(), tenantID, store.ListOptions{Limit: 10000})
		for _, s := range allSessions {
			totalInput += s.InputTokens
			totalOutput += s.OutputTokens
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tenant_id":           tenantID,
		"total_sessions":      total,
		"total_input_tokens":  totalInput,
		"total_output_tokens": totalOutput,
	})
}
