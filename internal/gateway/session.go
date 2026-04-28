package gateway

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"github.com/hermes-agent/hermes-agent-go/internal/state"
)

// SessionEntry represents a session in the store.
type SessionEntry struct {
	SessionKey       string         `json:"session_key"`
	SessionID        string         `json:"session_id"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	Origin           *SessionSource `json:"origin,omitempty"`
	DisplayName      string         `json:"display_name,omitempty"`
	Platform         Platform       `json:"platform,omitempty"`
	ChatType         string         `json:"chat_type"`
	InputTokens      int            `json:"input_tokens"`
	OutputTokens     int            `json:"output_tokens"`
	CacheReadTokens  int            `json:"cache_read_tokens"`
	CacheWriteTokens int            `json:"cache_write_tokens"`
	TotalTokens      int            `json:"total_tokens"`
	LastPromptTokens int            `json:"last_prompt_tokens"`
	EstimatedCostUSD float64        `json:"estimated_cost_usd"`
	CostStatus       string         `json:"cost_status"`
	MemoryFlushed    bool           `json:"memory_flushed"`
	WasAutoReset     bool           `json:"was_auto_reset"`
	AutoResetReason  string         `json:"auto_reset_reason,omitempty"`
	ResetHadActivity bool           `json:"reset_had_activity"`
}

// SessionStore manages per-chat session tracking and persistence.
type SessionStore struct {
	mu          sync.Mutex
	entries     map[string]*SessionEntry
	sessionsDir string
	loaded      bool
	db          *state.SessionDB
	gatewayCfg  *GatewayConfig
	idleTimeout time.Duration
}

// NewSessionStore creates a new session store.
func NewSessionStore(gatewayCfg *GatewayConfig) *SessionStore {
	sessionsDir := filepath.Join(config.HermesHome(), "sessions")
	os.MkdirAll(sessionsDir, 0755)

	var db *state.SessionDB
	sdb, err := state.NewSessionDB("")
	if err != nil {
		slog.Warn("SQLite session store unavailable", "error", err)
	} else {
		db = sdb
	}

	return &SessionStore{
		entries:     make(map[string]*SessionEntry),
		sessionsDir: sessionsDir,
		db:          db,
		gatewayCfg:  gatewayCfg,
		idleTimeout: 30 * time.Minute,
	}
}

// GetOrCreateSession returns an existing session or creates a new one.
func (s *SessionStore) GetOrCreateSession(source *SessionSource, forceNew bool) *SessionEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureLoaded()

	sessionKey := BuildSessionKey(source, true, false)

	if !forceNew {
		if entry, ok := s.entries[sessionKey]; ok {
			if !s.isExpired(entry) {
				entry.UpdatedAt = time.Now()
				s.save()
				return entry
			}
			// Session expired, will create new.
		}
	}

	// Create new session.
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
	s.save()

	// Create in SQLite.
	if s.db != nil {
		s.db.CreateSession(sessionID, string(source.Platform), source.UserID, "")
	}

	return entry
}

// ResetSession forces a session reset.
func (s *SessionStore) ResetSession(sessionKey string) *SessionEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureLoaded()

	oldEntry, ok := s.entries[sessionKey]
	if !ok {
		return nil
	}

	// End old session in DB.
	if s.db != nil {
		s.db.EndSession(oldEntry.SessionID, "session_reset")
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
	s.save()

	if s.db != nil {
		userID := ""
		if oldEntry.Origin != nil {
			userID = oldEntry.Origin.UserID
		}
		s.db.CreateSession(sessionID, string(oldEntry.Platform), userID, "")
	}

	return newEntry
}

// UpdateSession updates lightweight session metadata.
func (s *SessionStore) UpdateSession(sessionKey string, lastPromptTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureLoaded()

	if entry, ok := s.entries[sessionKey]; ok {
		entry.UpdatedAt = time.Now()
		if lastPromptTokens > 0 {
			entry.LastPromptTokens = lastPromptTokens
		}
		s.save()
	}
}

// ListSessions returns all sessions, optionally filtered by active window.
func (s *SessionStore) ListSessions(activeMinutes int) []*SessionEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureLoaded()

	var entries []*SessionEntry
	cutoff := time.Now().Add(-time.Duration(activeMinutes) * time.Minute)

	for _, entry := range s.entries {
		if activeMinutes > 0 && entry.UpdatedAt.Before(cutoff) {
			continue
		}
		entries = append(entries, entry)
	}

	return entries
}

// SetMemoryFlushed marks a session's memory as flushed.
func (s *SessionStore) SetMemoryFlushed(sessionKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, exists := s.entries[sessionKey]; exists {
		entry.MemoryFlushed = true
	}
}

// Close closes the session store and underlying database.
func (s *SessionStore) Close() {
	if s.db != nil {
		s.db.Close()
	}
}

// --- Internal ---

func (s *SessionStore) isExpired(entry *SessionEntry) bool {
	if s.idleTimeout <= 0 {
		return false
	}
	return time.Since(entry.UpdatedAt) > s.idleTimeout
}

func (s *SessionStore) ensureLoaded() {
	if s.loaded {
		return
	}

	sessionsFile := filepath.Join(s.sessionsDir, "sessions.json")
	data, err := os.ReadFile(sessionsFile)
	if err == nil {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err == nil {
			for key, entryData := range raw {
				var entry SessionEntry
				if err := json.Unmarshal(entryData, &entry); err == nil {
					s.entries[key] = &entry
				}
			}
		}
	}

	s.loaded = true
}

func (s *SessionStore) save() {
	data := make(map[string]any, len(s.entries))
	for key, entry := range s.entries {
		data[key] = entry
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		slog.Warn("Failed to marshal sessions", "error", err)
		return
	}

	sessionsFile := filepath.Join(s.sessionsDir, "sessions.json")
	if err := os.WriteFile(sessionsFile, b, 0644); err != nil {
		slog.Warn("Failed to save sessions", "error", err)
	}
}

// BuildSessionKey builds a deterministic session key from a message source.
// The key includes tenant_id to guarantee cross-tenant isolation.
func BuildSessionKey(source *SessionSource, groupSessionsPerUser, threadSessionsPerUser bool) string {
	platform := string(source.Platform)
	tenant := source.TenantID
	if tenant == "" {
		tenant = "default"
	}

	if source.ChatType == "dm" {
		if source.ChatID != "" {
			if source.ThreadID != "" {
				return fmt.Sprintf("%s:agent:main:%s:dm:%s:%s", tenant, platform, source.ChatID, source.ThreadID)
			}
			return fmt.Sprintf("%s:agent:main:%s:dm:%s", tenant, platform, source.ChatID)
		}
		if source.ThreadID != "" {
			return fmt.Sprintf("%s:agent:main:%s:dm:%s", tenant, platform, source.ThreadID)
		}
		return fmt.Sprintf("%s:agent:main:%s:dm", tenant, platform)
	}

	participantID := source.UserIDAlt
	if participantID == "" {
		participantID = source.UserID
	}

	parts := []string{tenant, "agent:main", platform, source.ChatType}

	if source.ChatID != "" {
		parts = append(parts, source.ChatID)
	}
	if source.ThreadID != "" {
		parts = append(parts, source.ThreadID)
	}

	isolateUser := groupSessionsPerUser
	if source.ThreadID != "" && !threadSessionsPerUser {
		isolateUser = false
	}

	if isolateUser && participantID != "" {
		parts = append(parts, participantID)
	}

	return strings.Join(parts, ":")
}

// HashID returns a deterministic 12-char hex hash of an identifier.
func HashID(value string) string {
	h := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", h[:6])
}

// HashSenderID hashes a sender ID to "user_<12hex>".
func HashSenderID(value string) string {
	return "user_" + HashID(value)
}
