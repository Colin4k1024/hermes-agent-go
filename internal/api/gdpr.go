package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/observability"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

const gdprExportMaxSessions = 1000

var gdprAllowedTables = map[string]struct{}{
	"messages":      {},
	"sessions":      {},
	"memories":      {},
	"user_profiles": {},
	"api_keys":      {},
	"cron_jobs":     {},
	"users":         {},
	"audit_logs":    {},
}

// GDPRHandler serves data export and deletion endpoints.
// Accepts the full Store + direct pool for tables not exposed via store interface.
type GDPRHandler struct {
	store store.Store
	pool  *pgxpool.Pool
}

func NewGDPRHandler(s store.Store, pool *pgxpool.Pool) *GDPRHandler {
	return &GDPRHandler{store: s, pool: pool}
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

		ctx := r.Context()
		log := observability.ContextLogger(ctx)

		sessions, _, err := h.store.Sessions().List(ctx, tenantID, store.ListOptions{Limit: gdprExportMaxSessions})
		if err != nil {
			http.Error(w, "export failed", http.StatusInternalServerError)
			return
		}

		type sessionExport struct {
			Session  *store.Session   `json:"session"`
			Messages []*store.Message `json:"messages"`
		}

		// Collect memories via direct SQL.
		type memoryEntry struct {
			Key     string `json:"key"`
			Content string `json:"content"`
		}
		var memories []memoryEntry
		if h.pool != nil {
			rows, err := h.pool.Query(ctx, `SELECT key, content FROM memories WHERE tenant_id = $1`, tenantID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var m memoryEntry
					if err := rows.Scan(&m.Key, &m.Content); err == nil {
						memories = append(memories, m)
					}
				}
			} else {
				log.Warn("gdpr export: failed to list memories", "error", err)
			}
		}

		// Collect user profiles via direct SQL.
		type profileEntry struct {
			UserID  string `json:"user_id"`
			Content string `json:"content"`
		}
		var profiles []profileEntry
		if h.pool != nil {
			rows, err := h.pool.Query(ctx, `SELECT user_id, content FROM user_profiles WHERE tenant_id = $1`, tenantID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var p profileEntry
					if err := rows.Scan(&p.UserID, &p.Content); err == nil {
						profiles = append(profiles, p)
					}
				}
			} else {
				log.Warn("gdpr export: failed to list profiles", "error", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=export.json")

		enc := json.NewEncoder(w)
		fmt.Fprint(w, `{"tenant_id":"`+tenantID+`","sessions":[`)
		for i, sess := range sessions {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			msgs, err := h.store.Messages().List(ctx, tenantID, sess.ID, 1000, 0)
			if err != nil {
				log.Warn("gdpr export: failed to list messages", "session_id", sess.ID, "error", err)
				msgs = nil
			}
			enc.Encode(sessionExport{Session: sess, Messages: msgs})
		}
		fmt.Fprint(w, `],"memories":`)
		enc.Encode(memories)
		fmt.Fprint(w, `,"profiles":`)
		enc.Encode(profiles)
		fmt.Fprint(w, `}`)
	}
}

// DeleteHandler returns DELETE /v1/gdpr/data — deletes all data for a tenant.
// All deletions run in a single transaction to prevent partial data loss.
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

		ctx := r.Context()
		log := observability.ContextLogger(ctx)

		if h.pool != nil {
			if err := h.deleteViaTx(ctx, tenantID); err != nil {
				log.Error("gdpr delete failed", "error", err)
				http.Error(w, "deletion failed", http.StatusInternalServerError)
				return
			}
		} else {
			if err := h.deleteViaStore(ctx, tenantID, log); err != nil {
				http.Error(w, "deletion failed", http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// deleteViaTx performs all deletions in a single PG transaction (production path).
func (h *GDPRHandler) deleteViaTx(ctx context.Context, tenantID string) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	cascadeTables := []string{
		"messages", "sessions", "memories", "user_profiles",
		"api_keys", "cron_jobs", "users", "audit_logs",
	}
	for _, table := range cascadeTables {
		if _, ok := gdprAllowedTables[table]; !ok {
			return fmt.Errorf("table %s not in allowlist", table)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE tenant_id = $1`, table), tenantID); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID); err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	return tx.Commit(ctx)
}

// deleteViaStore performs deletions through the store interface (fallback when pool is nil).
func (h *GDPRHandler) deleteViaStore(ctx context.Context, tenantID string, log *slog.Logger) error {
	sessions, _, _ := h.store.Sessions().List(ctx, tenantID, store.ListOptions{Limit: gdprExportMaxSessions})
	for _, sess := range sessions {
		if err := h.store.Sessions().Delete(ctx, tenantID, sess.ID); err != nil {
			log.Error("gdpr delete: session failed", "session_id", sess.ID, "error", err)
			return err
		}
	}
	if _, err := h.store.AuditLogs().DeleteByTenant(ctx, tenantID); err != nil {
		log.Error("gdpr delete: audit_logs failed", "error", err)
		return err
	}
	return nil
}
