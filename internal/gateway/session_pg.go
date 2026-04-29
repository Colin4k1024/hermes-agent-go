package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

// PGSessionStore manages per-chat session tracking backed by PostgreSQL.
type PGSessionStore struct {
	mu          sync.Mutex
	entries     map[string]*SessionEntry
	pool        *pgxpool.Pool
	tenantID    string
	idleTimeout time.Duration
	loaded      bool
}

func NewPGSessionStore(pool *pgxpool.Pool, tenantID string) *PGSessionStore {
	if tenantID == "" {
		tenantID = DefaultTenantID
	}
	return &PGSessionStore{
		entries:     make(map[string]*SessionEntry),
		pool:        pool,
		tenantID:    tenantID,
		idleTimeout: 30 * time.Minute,
	}
}

func (s *PGSessionStore) GetOrCreateSession(source *SessionSource, forceNew bool) *SessionEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureLoaded()

	sessionKey := BuildSessionKey(source, true, false)

	if !forceNew {
		if entry, ok := s.entries[sessionKey]; ok {
			if !s.isExpired(entry) {
				entry.UpdatedAt = time.Now()
				s.persistMetadata(entry)
				return entry
			}
		}
	}

	now := time.Now()
	sessionID := fmt.Sprintf("%s_%s", now.Format("20060102_150405"), uuid.New().String()[:8])

	entry := &SessionEntry{
		SessionKey:  sessionKey,
		SessionID:   sessionID,
		CreatedAt:   now,
		UpdatedAt:   now,
		Origin:      source,
		DisplayName: source.ChatName,
		Platform:    source.Platform,
		ChatType:    source.ChatType,
	}

	s.entries[sessionKey] = entry
	s.insertSession(entry, source.UserID)
	return entry
}

func (s *PGSessionStore) ResetSession(sessionKey string) *SessionEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureLoaded()

	oldEntry, ok := s.entries[sessionKey]
	if !ok {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET ended_at = $1, end_reason = $2 WHERE tenant_id = $3 AND id = $4`,
		time.Now(), "session_reset", s.tenantID, oldEntry.SessionID)
	if err != nil {
		slog.Warn("PG: failed to end old session", "error", err)
	}

	now := time.Now()
	sessionID := fmt.Sprintf("%s_%s", now.Format("20060102_150405"), uuid.New().String()[:8])

	newEntry := &SessionEntry{
		SessionKey:  sessionKey,
		SessionID:   sessionID,
		CreatedAt:   now,
		UpdatedAt:   now,
		Origin:      oldEntry.Origin,
		DisplayName: oldEntry.DisplayName,
		Platform:    oldEntry.Platform,
		ChatType:    oldEntry.ChatType,
	}

	s.entries[sessionKey] = newEntry

	userID := ""
	if oldEntry.Origin != nil {
		userID = oldEntry.Origin.UserID
	}
	s.insertSession(newEntry, userID)
	return newEntry
}

func (s *PGSessionStore) UpdateSession(sessionKey string, lastPromptTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureLoaded()

	entry, ok := s.entries[sessionKey]
	if !ok {
		return
	}
	entry.UpdatedAt = time.Now()
	if lastPromptTokens > 0 {
		entry.LastPromptTokens = lastPromptTokens
	}
	s.persistMetadata(entry)
}

func (s *PGSessionStore) SetMemoryFlushed(sessionKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.entries[sessionKey]; exists {
		entry.MemoryFlushed = true
		s.persistMetadata(entry)
	}
}

func (s *PGSessionStore) ListSessions(activeMinutes int) []*SessionEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureLoaded()

	var result []*SessionEntry
	cutoff := time.Now().Add(-time.Duration(activeMinutes) * time.Minute)

	for _, entry := range s.entries {
		if activeMinutes > 0 && entry.UpdatedAt.Before(cutoff) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func (s *PGSessionStore) Close() {}

// EnsureDefaultTenant creates the default tenant row if missing.
func (s *PGSessionStore) EnsureDefaultTenant(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tenants (id, name, plan)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING`,
		s.tenantID, "default", "free")
	return err
}

// --- internal helpers ---

func (s *PGSessionStore) isExpired(entry *SessionEntry) bool {
	if s.idleTimeout <= 0 {
		return false
	}
	return time.Since(entry.UpdatedAt) > s.idleTimeout
}

func (s *PGSessionStore) ensureLoaded() {
	if s.loaded {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, `
		SELECT id, session_key, platform, user_id, started_at,
		       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		       estimated_cost_usd, metadata
		FROM sessions
		WHERE tenant_id = $1 AND ended_at IS NULL AND session_key IS NOT NULL
		ORDER BY started_at DESC`, s.tenantID)
	if err != nil {
		slog.Warn("PG: failed to load sessions", "error", err)
		s.loaded = true
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id, sessionKey, platform, userID string
			startedAt                        time.Time
			inputTokens, outputTokens        int
			cacheRead, cacheWrite            int
			costUSD                          *float64
			metaJSON                         []byte
		)

		if err := rows.Scan(&id, &sessionKey, &platform, &userID, &startedAt,
			&inputTokens, &outputTokens, &cacheRead, &cacheWrite, &costUSD, &metaJSON); err != nil {
			slog.Warn("PG: failed to scan session", "error", err)
			continue
		}

		entry := &SessionEntry{
			SessionKey:       sessionKey,
			SessionID:        id,
			CreatedAt:        startedAt,
			UpdatedAt:        startedAt,
			Platform:         Platform(platform),
			InputTokens:      inputTokens,
			OutputTokens:     outputTokens,
			CacheReadTokens:  cacheRead,
			CacheWriteTokens: cacheWrite,
		}
		if costUSD != nil {
			entry.EstimatedCostUSD = *costUSD
		}

		if len(metaJSON) > 2 {
			var meta map[string]any
			if err := json.Unmarshal(metaJSON, &meta); err == nil {
				restoreGatewayMetadata(entry, meta)
			}
		}

		s.entries[sessionKey] = entry
	}
	if err := rows.Err(); err != nil {
		slog.Warn("PG: error iterating sessions", "error", err)
	}

	slog.Info("PG: loaded gateway sessions", "count", len(s.entries), "tenant", s.tenantID)
	s.loaded = true
}

func (s *PGSessionStore) insertSession(entry *SessionEntry, userID string) {
	if userID == "" {
		userID = "unknown"
	}

	meta := buildGatewayMetadata(entry)
	metaJSON, _ := json.Marshal(meta)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, tenant_id, platform, user_id, session_key, started_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		entry.SessionID, s.tenantID, string(entry.Platform), userID, entry.SessionKey,
		entry.CreatedAt, metaJSON)
	if err != nil {
		slog.Warn("PG: failed to insert session", "error", err, "session_key", entry.SessionKey)
	}
}

func (s *PGSessionStore) persistMetadata(entry *SessionEntry) {
	meta := buildGatewayMetadata(entry)
	metaJSON, _ := json.Marshal(meta)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx, `
		UPDATE sessions SET metadata = $1
		WHERE tenant_id = $2 AND id = $3`,
		metaJSON, s.tenantID, entry.SessionID)
	if err != nil {
		slog.Warn("PG: failed to persist session metadata", "error", err, "session_id", entry.SessionID)
	}
}

func buildGatewayMetadata(entry *SessionEntry) map[string]any {
	meta := map[string]any{
		"updated_at":         entry.UpdatedAt,
		"chat_type":          entry.ChatType,
		"display_name":       entry.DisplayName,
		"total_tokens":       entry.TotalTokens,
		"last_prompt_tokens": entry.LastPromptTokens,
		"cost_status":        entry.CostStatus,
		"memory_flushed":     entry.MemoryFlushed,
		"was_auto_reset":     entry.WasAutoReset,
		"auto_reset_reason":  entry.AutoResetReason,
		"reset_had_activity": entry.ResetHadActivity,
	}
	if entry.Origin != nil {
		originJSON, err := json.Marshal(entry.Origin)
		if err == nil {
			meta["origin"] = json.RawMessage(originJSON)
		}
	}
	return meta
}

func restoreGatewayMetadata(entry *SessionEntry, meta map[string]any) {
	if v, ok := meta["updated_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			entry.UpdatedAt = t
		}
	}
	if v, ok := meta["chat_type"].(string); ok {
		entry.ChatType = v
	}
	if v, ok := meta["display_name"].(string); ok {
		entry.DisplayName = v
	}
	if v, ok := meta["total_tokens"].(float64); ok {
		entry.TotalTokens = int(v)
	}
	if v, ok := meta["last_prompt_tokens"].(float64); ok {
		entry.LastPromptTokens = int(v)
	}
	if v, ok := meta["cost_status"].(string); ok {
		entry.CostStatus = v
	}
	if v, ok := meta["memory_flushed"].(bool); ok {
		entry.MemoryFlushed = v
	}
	if v, ok := meta["was_auto_reset"].(bool); ok {
		entry.WasAutoReset = v
	}
	if v, ok := meta["auto_reset_reason"].(string); ok {
		entry.AutoResetReason = v
	}
	if v, ok := meta["reset_had_activity"].(bool); ok {
		entry.ResetHadActivity = v
	}
	if v, ok := meta["origin"]; ok {
		originBytes, err := json.Marshal(v)
		if err == nil {
			var origin SessionSource
			if err := json.Unmarshal(originBytes, &origin); err == nil {
				entry.Origin = &origin
			}
		}
	}
}
