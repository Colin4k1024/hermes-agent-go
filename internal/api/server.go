package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/objstore"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// APIServerConfig holds all dependencies for the SaaS API server.
type APIServerConfig struct {
	Port           int
	Store          store.Store
	DB             DBPinger // optional; nil disables readiness DB check
	AuthChain      *auth.ExtractorChain
	RBAC           middleware.RBACConfig
	RateLimit      middleware.RateLimitConfig
	Pool           *pgxpool.Pool            // direct pool access for memory operations
	AllowedOrigins string                  // comma-separated list of allowed origins, or "*" for all
	StaticDir      string                  // directory to serve static files from (optional)
	SkillsClient   *objstore.MinIOClient  // optional; nil disables per-tenant skill loading
}

// APIServer is the multi-tenant SaaS API HTTP server.
type APIServer struct {
	cfg    APIServerConfig
	server *http.Server
	MockChat *mockChatHandler // accessible for testing/debugging
}

// spaFallback wraps the API mux: serves index.html for root "/" and admin.html,
// delegates all other paths to the inner mux. This avoids ServeMux conflicts
// between "/" and "/v1/" patterns.
func spaFallback(mux, spa http.Handler, staticDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			http.ServeFile(w, r, staticDir+"/index.html")
			return
		}
		if path == "/admin.html" || path == "/index.html" || path == "/isolation-test.html" || path == "/chat.html" {
			http.ServeFile(w, r, staticDir+path)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// corsMiddleware returns an HTTP handler that adds CORS headers.
func corsMiddleware(next http.Handler, origins string) http.Handler {
	allowAll := origins == "*"
	allowed := make(map[string]bool)
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed[o] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAll || allowed[origin]) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Hermes-Session-Id, X-Hermes-User-Id")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NewAPIServer creates and configures the API server with all routes and middleware.
func NewAPIServer(cfg APIServerConfig) *APIServer {
	stack := middleware.NewStack(middleware.StackConfig{
		Metrics:   middleware.MetricsMiddleware,
		RequestID: middleware.RequestIDMiddleware,
		Auth:      middleware.AuthMiddleware(cfg.AuthChain, false),
		Tenant:    middleware.TenantMiddleware,
		Audit:     middleware.AuditMiddleware(cfg.Store.AuditLogs()),
		RBAC:      middleware.RBACMiddleware(cfg.RBAC),
		RateLimit: middleware.RateLimitMiddleware(cfg.RateLimit),
	})

	mux := http.NewServeMux()

	// Public routes — no middleware stack.
	health := NewHealthHandler(cfg.DB)
	mux.HandleFunc("GET /health/live", health.LiveHandler())
	mux.HandleFunc("GET /health/ready", health.ReadyHandler())
	mux.Handle("GET /metrics", stack.Wrap(promhttp.Handler()))

	// Authenticated routes — through middleware stack + audit.
	api := http.NewServeMux()
	api.Handle("/v1/tenants", NewTenantHandler(cfg.Store.Tenants()))
	api.Handle("/v1/tenants/", NewTenantHandler(cfg.Store.Tenants()))
	api.Handle("/v1/api-keys", NewAPIKeyHandler(cfg.Store.APIKeys()))
	api.Handle("/v1/api-keys/", NewAPIKeyHandler(cfg.Store.APIKeys()))
	api.Handle("/v1/audit-logs", NewAuditHandler(cfg.Store.AuditLogs()))
	api.Handle("/v1/usage", NewUsageHandler(cfg.Store.Sessions(), cfg.Store.Messages()))
	api.HandleFunc("GET /v1/openapi", OpenAPISpec())

	me := NewMeHandler(cfg.Store)
	api.Handle("/v1/me", me)

	gdpr := NewGDPRHandler(cfg.Store.Sessions(), cfg.Store.Messages())
	api.HandleFunc("GET /v1/gdpr/export", gdpr.ExportHandler())
	api.HandleFunc("DELETE /v1/gdpr/data", gdpr.DeleteHandler())

	// Mock chat endpoints for multi-tenant isolation testing.
	mockChat := NewMockChatHandler(cfg.Store, cfg.Pool, cfg.SkillsClient)
	api.HandleFunc("POST /v1/chat/completions", mockChat.ServeHTTP)
	api.HandleFunc("GET /v1/mock-sessions", mockChat.handleSessionList)
	api.HandleFunc("DELETE /v1/mock-sessions/", mockChat.handleClearSession)

	// Memory management API (per-user long-term memory).
	api.HandleFunc("GET /v1/memories", mockChat.handleListMemories)
	api.HandleFunc("DELETE /v1/memories/", mockChat.handleDeleteMemory)

	// Session history API (per-user session and message history).
	api.HandleFunc("GET /v1/sessions", mockChat.handleListUserSessions)
	api.HandleFunc("GET /v1/sessions/", mockChat.handleGetSessionMessages)

	mux.Handle("/v1/", stack.Wrap(api))

	// Static file serving (optional).
	var spaHandler http.Handler
	if cfg.StaticDir != "" {
		if _, err := os.Stat(cfg.StaticDir); err == nil {
			spaHandler = http.FileServer(http.Dir(cfg.StaticDir))
			// Strip /static/ prefix.
			mux.Handle("/static/", http.StripPrefix("/static/", spaHandler))
			slog.Info("Serving static files", "dir", cfg.StaticDir)
		} else {
			slog.Warn("Static directory not found, skipping static file serving", "dir", cfg.StaticDir)
		}
	}

	var handler http.Handler = mux

	// Wrap mux with SPA fallback: serve index.html for root, else pass through to mux.
	// Done outside mux to avoid ServeMux path conflict between "/" and "/v1/".
	if spaHandler != nil {
		handler = spaFallback(handler, spaHandler, cfg.StaticDir)
	}

	// Apply CORS if configured.
	if cfg.AllowedOrigins != "" {
		handler = corsMiddleware(handler, cfg.AllowedOrigins)
		slog.Info("CORS enabled", "origins", cfg.AllowedOrigins)
	}

	s := &APIServer{
		cfg:      cfg,
		MockChat: mockChat,
		server: &http.Server{
			Addr:         fmt.Sprintf("0.0.0.0:%d", cfg.Port),
			Handler:      handler,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
	return s
}

// Start begins serving. Blocks until the server is shut down.
func (s *APIServer) Start() error {
	slog.Info("SaaS API server starting", "port", s.cfg.Port)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("api server: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *APIServer) Shutdown(ctx context.Context) error {
	slog.Info("SaaS API server shutting down")
	return s.server.Shutdown(ctx)
}
