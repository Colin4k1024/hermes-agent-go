package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/observability"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

const gdprExportMaxSessions = 1000

// GDPRHandler serves data export and deletion endpoints.
type GDPRHandler struct {
	sessions store.SessionStore
	messages store.MessageStore
}

func NewGDPRHandler(sessions store.SessionStore, messages store.MessageStore) *GDPRHandler {
	return &GDPRHandler{sessions: sessions, messages: messages}
}

// ExportHandler returns GET /v1/gdpr/export — exports all user data for a tenant.
func (h *GDPRHandler) ExportHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tenantID := middleware.TenantFromContext(r.Context())
		if tenantID == "" {
			http.Error(w, "tenant context required", http.StatusBadRequest)
			return
		}

		sessions, _, err := h.sessions.List(r.Context(), tenantID, store.ListOptions{Limit: gdprExportMaxSessions})
		if err != nil {
			http.Error(w, "export failed", http.StatusInternalServerError)
			return
		}

		type sessionExport struct {
			Session  *store.Session   `json:"session"`
			Messages []*store.Message `json:"messages"`
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=export.json")

		enc := json.NewEncoder(w)
		fmt.Fprint(w, `{"tenant_id":"`+tenantID+`","sessions":[`)
		for i, sess := range sessions {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			msgs, err := h.messages.List(r.Context(), tenantID, sess.ID, 1000, 0)
			if err != nil {
				observability.ContextLogger(r.Context()).Warn("gdpr export: failed to list messages", "session_id", sess.ID, "error", err)
				msgs = nil
			}
			enc.Encode(sessionExport{Session: sess, Messages: msgs})
		}
		fmt.Fprint(w, `]}`)
	}
}

// DeleteHandler returns DELETE /v1/gdpr/delete — deletes all data for a tenant.
func (h *GDPRHandler) DeleteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tenantID := middleware.TenantFromContext(r.Context())
		if tenantID == "" {
			http.Error(w, "tenant context required", http.StatusBadRequest)
			return
		}

		sessions, _, err := h.sessions.List(r.Context(), tenantID, store.ListOptions{Limit: gdprExportMaxSessions})
		if err != nil {
			http.Error(w, "delete failed", http.StatusInternalServerError)
			return
		}

		var errs []error
		for _, sess := range sessions {
			if err := h.sessions.Delete(r.Context(), tenantID, sess.ID); err != nil {
				observability.ContextLogger(r.Context()).Error("gdpr delete: failed to delete session", "session_id", sess.ID, "error", err)
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			http.Error(w, fmt.Sprintf("partial deletion: %d of %d sessions failed", len(errs), len(sessions)), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
