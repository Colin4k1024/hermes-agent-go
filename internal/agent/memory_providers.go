package agent

import (
	"os"
	"sync"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"github.com/hermes-agent/hermes-agent-go/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Package-level PG pool for the postgres memory provider.
// Set via SetPGMemoryPool before any memory operations.
var (
	pgMemoryPool     *pgxpool.Pool
	pgMemoryTenantID string
	pgMemoryMu       sync.RWMutex
)

// SetPGMemoryPool configures the PostgreSQL connection pool and tenant ID
// used by the "postgres" memory provider. Call from main.go at startup.
func SetPGMemoryPool(pool *pgxpool.Pool, tenantID string) {
	pgMemoryMu.Lock()
	defer pgMemoryMu.Unlock()
	pgMemoryPool = pool
	pgMemoryTenantID = tenantID
}

// builtinMemoryAdapter wraps agent.BuiltinMemoryProvider to satisfy
// tools.MemoryProvider (avoids import cycle).
type builtinMemoryAdapter struct {
	inner *BuiltinMemoryProvider
}

func (a *builtinMemoryAdapter) ReadMemory() (string, error)      { return a.inner.ReadMemory() }
func (a *builtinMemoryAdapter) SaveMemory(k, c string) error     { return a.inner.SaveMemory(k, c) }
func (a *builtinMemoryAdapter) DeleteMemory(k string) error      { return a.inner.DeleteMemory(k) }
func (a *builtinMemoryAdapter) ReadUserProfile() (string, error) { return a.inner.ReadUserProfile() }
func (a *builtinMemoryAdapter) SaveUserProfile(c string) error   { return a.inner.SaveUserProfile(c) }

// pgMemoryAdapter wraps agent.PGMemoryProvider to satisfy tools.MemoryProvider.
var _ SystemPromptProvider = (*pgMemoryAdapter)(nil)

type pgMemoryAdapter struct {
	inner *PGMemoryProvider
}

func (a *pgMemoryAdapter) ReadMemory() (string, error)      { return a.inner.ReadMemory() }
func (a *pgMemoryAdapter) SaveMemory(k, c string) error     { return a.inner.SaveMemory(k, c) }
func (a *pgMemoryAdapter) DeleteMemory(k string) error      { return a.inner.DeleteMemory(k) }
func (a *pgMemoryAdapter) ReadUserProfile() (string, error) { return a.inner.ReadUserProfile() }
func (a *pgMemoryAdapter) SaveUserProfile(c string) error   { return a.inner.SaveUserProfile(c) }
func (a *pgMemoryAdapter) SystemPromptBlock() string        { return a.inner.SystemPromptBlock() }

// honchoMemoryAdapter wraps agent.HonchoProvider to satisfy tools.MemoryProvider.
type honchoMemoryAdapter struct {
	inner *HonchoProvider
}

func (a *honchoMemoryAdapter) ReadMemory() (string, error)      { return a.inner.ReadMemory() }
func (a *honchoMemoryAdapter) SaveMemory(k, c string) error     { return a.inner.SaveMemory(k, c) }
func (a *honchoMemoryAdapter) DeleteMemory(k string) error      { return a.inner.DeleteMemory(k) }
func (a *honchoMemoryAdapter) ReadUserProfile() (string, error) { return a.inner.ReadUserProfile() }
func (a *honchoMemoryAdapter) SaveUserProfile(c string) error   { return a.inner.SaveUserProfile(c) }

// NewPGMemoryProviderAsToolsProvider creates a tools.MemoryProvider backed by
// PostgreSQL, scoped to a specific tenant and user. Used by the gateway runner
// to inject per-agent memory isolation.
func NewPGMemoryProviderAsToolsProvider(pool *pgxpool.Pool, tenantID, userID string) tools.MemoryProvider {
	return &pgMemoryAdapter{
		inner: NewPGMemoryProvider(pool, tenantID, userID),
	}
}

func init() {
	// Register builtin provider.
	tools.RegisterMemoryProvider("builtin", func() tools.MemoryProvider {
		return &builtinMemoryAdapter{
			inner: NewBuiltinMemoryProvider(config.HermesHome()),
		}
	})

	// Register honcho provider.
	tools.RegisterMemoryProvider("honcho", func() tools.MemoryProvider {
		appID := os.Getenv("HONCHO_APP_ID")
		userID := os.Getenv("HONCHO_USER_ID")
		if userID == "" {
			userID = "default"
		}
		if appID == "" || os.Getenv("HONCHO_API_KEY") == "" {
			// Fall back to builtin when Honcho is not configured.
			return &builtinMemoryAdapter{
				inner: NewBuiltinMemoryProvider(config.HermesHome()),
			}
		}
		return &honchoMemoryAdapter{
			inner: NewHonchoProvider(appID, userID),
		}
	})

	// Register postgres provider.
	tools.RegisterMemoryProvider("postgres", func() tools.MemoryProvider {
		pgMemoryMu.RLock()
		pool := pgMemoryPool
		tid := pgMemoryTenantID
		pgMemoryMu.RUnlock()

		if pool == nil {
			// Fall back to builtin when pool is not configured.
			return &builtinMemoryAdapter{
				inner: NewBuiltinMemoryProvider(config.HermesHome()),
			}
		}
		userID := "default"
		return &pgMemoryAdapter{
			inner: NewPGMemoryProvider(pool, tid, userID),
		}
	})

	// Wire config reader for provider name.
	// If DATABASE_URL is set, default to "postgres" provider.
	tools.SetMemoryProviderNameFunc(func() string {
		if os.Getenv("DATABASE_URL") != "" {
			return "postgres"
		}
		cfg := config.Load()
		return cfg.Memory.Provider
	})
}
