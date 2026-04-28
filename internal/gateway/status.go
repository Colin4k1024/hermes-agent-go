package gateway

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// PlatformState represents the connection state of a platform.
type PlatformState struct {
	Platform     string `json:"platform"`
	State        string `json:"state"` // "connected", "disconnected", "error", "connecting"
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	ConnectedAt  string `json:"connected_at,omitempty"`
	LastActivity string `json:"last_activity,omitempty"`
	MessageCount int64  `json:"message_count"`
}

// RuntimeStatus holds the runtime state of the gateway.
type RuntimeStatus struct {
	mu sync.RWMutex

	StartedAt      string                   `json:"started_at"`
	Uptime         string                   `json:"uptime"`
	Platforms      map[string]PlatformState `json:"platforms"`
	TotalMessages  int64                    `json:"total_messages"`
	ActiveSessions int                      `json:"active_sessions"`
	LastUpdated    string                   `json:"last_updated"`

	// Internal tracking (not serialized).
	startTime time.Time `json:"-"`
}

// NewRuntimeStatus creates a new runtime status tracker.
func NewRuntimeStatus() *RuntimeStatus {
	now := time.Now()
	return &RuntimeStatus{
		StartedAt: now.Format(time.RFC3339),
		Platforms: make(map[string]PlatformState),
		startTime: now,
	}
}

// WriteRuntimeStatus updates the status for a specific platform and persists it.
func (rs *RuntimeStatus) WriteRuntimeStatus(platform, state, errorCode, errorMessage string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	now := time.Now()

	ps, exists := rs.Platforms[platform]
	if !exists {
		ps = PlatformState{
			Platform: platform,
		}
	}

	ps.State = state
	ps.ErrorCode = errorCode
	ps.ErrorMessage = errorMessage
	ps.LastActivity = now.Format(time.RFC3339)

	if state == "connected" && ps.ConnectedAt == "" {
		ps.ConnectedAt = now.Format(time.RFC3339)
	}
	if state == "disconnected" || state == "error" {
		ps.ConnectedAt = ""
	}

	rs.Platforms[platform] = ps
	rs.LastUpdated = now.Format(time.RFC3339)
	rs.Uptime = time.Since(rs.startTime).Round(time.Second).String()

	rs.persistLocked()
}

// IncrementMessageCount increments the message count for a platform.
func (rs *RuntimeStatus) IncrementMessageCount(platform string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	ps := rs.Platforms[platform]
	ps.Platform = platform
	ps.MessageCount++
	ps.LastActivity = time.Now().Format(time.RFC3339)
	rs.Platforms[platform] = ps

	rs.TotalMessages++
	rs.LastUpdated = time.Now().Format(time.RFC3339)
	rs.Uptime = time.Since(rs.startTime).Round(time.Second).String()

	rs.persistLocked()
}

// SetActiveSessions updates the active session count.
func (rs *RuntimeStatus) SetActiveSessions(count int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.ActiveSessions = count
	rs.LastUpdated = time.Now().Format(time.RFC3339)
}

// ReadRuntimeStatus loads the runtime status from disk.
func ReadRuntimeStatus() *RuntimeStatus {
	statusPath := filepath.Join(config.HermesHome(), "gateway_status.json")

	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil
	}

	var status RuntimeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		slog.Debug("Failed to parse gateway status", "error", err)
		return nil
	}

	return &status
}

// Snapshot returns a copy of the current status (thread-safe).
func (rs *RuntimeStatus) Snapshot() *RuntimeStatus {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	uptime := time.Since(rs.startTime).Round(time.Second).String()

	snapshot := &RuntimeStatus{
		StartedAt:      rs.StartedAt,
		Uptime:         uptime,
		TotalMessages:  rs.TotalMessages,
		ActiveSessions: rs.ActiveSessions,
		LastUpdated:    rs.LastUpdated,
		Platforms:      make(map[string]PlatformState, len(rs.Platforms)),
	}

	for k, v := range rs.Platforms {
		snapshot.Platforms[k] = v
	}

	return snapshot
}

// persistLocked writes the status to disk. Must be called with mu held.
func (rs *RuntimeStatus) persistLocked() {
	statusPath := filepath.Join(config.HermesHome(), "gateway_status.json")

	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		slog.Debug("Failed to marshal gateway status", "error", err)
		return
	}

	if err := os.WriteFile(statusPath, data, 0644); err != nil {
		slog.Debug("Failed to write gateway status", "error", err)
	}
}
