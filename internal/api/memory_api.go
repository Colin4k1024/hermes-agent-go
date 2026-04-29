package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/agent"
	"github.com/hermes-agent/hermes-agent-go/internal/auth"
)

type memoryEntry struct {
	Key     string `json:"key"`
	Content string `json:"content"`
}

func (h *mockChatHandler) handleListMemories(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID := r.Header.Get("X-Hermes-User-Id")
	if userID == "" {
		userID = ac.Identity
	}

	if h.pool == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"memories": []memoryEntry{}, "count": 0})
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT key, content FROM memories
		 WHERE tenant_id = $1 AND user_id = $2
		 ORDER BY updated_at DESC`,
		ac.TenantID, userID)
	if err != nil {
		http.Error(w, "failed to query memories", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries []memoryEntry
	for rows.Next() {
		var e memoryEntry
		if err := rows.Scan(&e.Key, &e.Content); err == nil {
			entries = append(entries, e)
		}
	}
	if entries == nil {
		entries = []memoryEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tenant_id": ac.TenantID,
		"user_id":   userID,
		"memories":  entries,
		"count":     len(entries),
	})
}

func (h *mockChatHandler) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID := r.Header.Get("X-Hermes-User-Id")
	if userID == "" {
		userID = ac.Identity
	}

	key := strings.TrimPrefix(r.URL.Path, "/v1/memories/")
	if key == "" {
		http.Error(w, "memory key required", http.StatusBadRequest)
		return
	}

	if h.pool == nil {
		http.Error(w, "memory store not available", http.StatusServiceUnavailable)
		return
	}

	provider := agent.NewPGMemoryProvider(h.pool, ac.TenantID, userID)
	if err := provider.DeleteMemory(key); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "failed to delete memory", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *mockChatHandler) handleListUserSessions(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID := r.Header.Get("X-Hermes-User-Id")
	if userID == "" {
		userID = ac.Identity
	}

	if h.pool == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"sessions": []any{}})
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT s.id, s.started_at, s.ended_at,
		        (SELECT COUNT(*) FROM messages m WHERE m.tenant_id = s.tenant_id AND m.session_id = s.id) as msg_count
		 FROM sessions s
		 WHERE s.tenant_id = $1 AND s.user_id = $2
		 ORDER BY s.started_at DESC
		 LIMIT 50`, ac.TenantID, userID)
	if err != nil {
		http.Error(w, "failed to query sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type sessionEntry struct {
		ID           string  `json:"id"`
		StartedAt    string  `json:"started_at"`
		EndedAt      *string `json:"ended_at,omitempty"`
		MessageCount int     `json:"message_count"`
	}

	var sessions []sessionEntry
	for rows.Next() {
		var id string
		var startedAt time.Time
		var endedAt *time.Time
		var msgCount int
		if err := rows.Scan(&id, &startedAt, &endedAt, &msgCount); err == nil {
			s := sessionEntry{
				ID:           id,
				StartedAt:    startedAt.Format(time.RFC3339),
				MessageCount: msgCount,
			}
			if endedAt != nil {
				ea := endedAt.Format(time.RFC3339)
				s.EndedAt = &ea
			}
			sessions = append(sessions, s)
		}
	}
	if sessions == nil {
		sessions = []sessionEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tenant_id": ac.TenantID,
		"user_id":   userID,
		"sessions":  sessions,
		"count":     len(sessions),
	})
}

func (h *mockChatHandler) handleGetSessionMessages(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	sessionID := parts[0]

	if h.pool == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"messages": []any{}})
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT role, content, timestamp FROM messages
		 WHERE tenant_id = $1 AND session_id = $2
		 ORDER BY timestamp ASC
		 LIMIT 200`, ac.TenantID, sessionID)
	if err != nil {
		http.Error(w, "failed to query messages", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type msgEntry struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		Timestamp string `json:"timestamp"`
	}

	var messages []msgEntry
	for rows.Next() {
		var role, content string
		var ts time.Time
		if err := rows.Scan(&role, &content, &ts); err == nil {
			messages = append(messages, msgEntry{
				Role:      role,
				Content:   content,
				Timestamp: ts.Format(time.RFC3339),
			})
		}
	}
	if messages == nil {
		messages = []msgEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tenant_id":  ac.TenantID,
		"session_id": sessionID,
		"messages":   messages,
		"count":      len(messages),
	})
}
