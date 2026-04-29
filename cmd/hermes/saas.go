package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/hermes-agent/hermes-agent-go/internal/acp"
	"github.com/hermes-agent/hermes-agent-go/internal/api"
	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/gateway/platforms"
	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/objstore"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/hermes-agent/hermes-agent-go/internal/store/pg"
	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
)

var saasAPICmd = &cobra.Command{
	Use:   "saas-api",
	Short: "Start the SaaS multi-tenant API server",
	Long: `Starts the Hermes SaaS API server with multi-tenant auth, RBAC, rate limiting, and static file serving.

Required environment variables:
  DATABASE_URL      PostgreSQL connection URL
  HERMES_ACP_TOKEN  Static bearer token for ACP endpoints

Optional environment variables:
  SAAS_API_PORT          API server port (default: 8080)
  SAAS_ALLOWED_ORIGINS    Comma-separated CORS origins, or "*" for all
  SAAS_STATIC_DIR         Directory for static files (e.g. ./internal/dashboard/static)
  HERMES_API_PORT         OpenAI-compatible adapter port (default: 8081)
  HERMES_API_KEY          Bearer token for OpenAI-compatible adapter
  HERMES_ACP_PORT         ACP server port

Examples:
  hermes saas-api                                           # Basic
  SAAS_API_PORT=8080 SAAS_STATIC_DIR=./internal/dashboard/static hermes saas-api
`,
	RunE: runSaaSAPI,
}

func init() {
	rootCmd.AddCommand(saasAPICmd)
}

func runSaaSAPI(cmd *cobra.Command, args []string) error {
	setupLogging()

	// ── 1. Parse environment ──────────────────────────────────
	port := 8080
	if v := os.Getenv("SAAS_API_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			port = p
		}
	}
	allowedOrigins := os.Getenv("SAAS_ALLOWED_ORIGINS")
	staticDir := os.Getenv("SAAS_STATIC_DIR")
	acpToken := os.Getenv("HERMES_ACP_TOKEN")
	apiKey := os.Getenv("HERMES_API_KEY")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	// ── 2. Initialize store ──────────────────────────────────
	cfg := store.StoreConfig{Driver: "postgres", URL: dbURL}
	dataStore, err := store.NewStore(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("init postgres store: %w", err)
	}
	defer dataStore.Close()

	pgStore, ok := dataStore.(*pg.PGStore)
	if !ok {
		return fmt.Errorf("expected PGStore, got %T", dataStore)
	}

	// ── 3. Seed default tenant (for static token auth) ────────
	if acpToken != "" {
		if err := seedDefaultTenant(context.Background(), pgStore); err != nil {
			slog.Warn("failed to seed default tenant", "error", err)
		}
	}

	// ── 4. Build auth chain ──────────────────────────────────
	authChain := auth.NewExtractorChain()

	// Static token extractor (for backward compatibility and dev mode).
	if acpToken != "" {
		authChain.Add(auth.NewStaticTokenExtractor(acpToken, "00000000-0000-0000-0000-000000000001"))
	}

	// API key extractor.
	authChain.Add(auth.NewAPIKeyExtractor(pgStore.APIKeys()))

	// JWT extractor — add JWT config here in production.
	// authChain.Extractors = append(authChain.Extractors, auth.NewJWTExtractor(...))

	// ── 4. RBAC config ──────────────────────────────────────
	rbacCfg := middleware.RBACConfig{
		DefaultRole: "user",
		Rules: map[string]string{
			"/v1/tenants":    "admin",
			"/v1/tenants/":   "admin",
			"/v1/api-keys":   "admin",
			"/v1/api-keys/":  "admin",
			"/v1/audit-logs": "admin",
			"/v1/gdpr/":      "admin",
		},
	}

	// ── 5. Rate limit config ─────────────────────────────────
	rateLimitCfg := middleware.RateLimitConfig{
		DefaultRPM: 60,
		TenantLimitFn: func(tenantID string) int {
			t, err := pgStore.Tenants().Get(context.Background(), tenantID)
			if err != nil || t == nil {
				return 0 // fall back to DefaultRPM
			}
			return t.RateLimitRPM
		},
	}

	// ── 6. Initialize MinIO client for per-tenant skills and soul storage ──
	var skillsClient *objstore.MinIOClient
	if minioEndpoint := os.Getenv("MINIO_ENDPOINT"); minioEndpoint != "" {
		minioAccessKey := os.Getenv("MINIO_ACCESS_KEY")
		minioSecretKey := os.Getenv("MINIO_SECRET_KEY")
		minioBucket := os.Getenv("MINIO_BUCKET")
		if minioAccessKey != "" && minioSecretKey != "" && minioBucket != "" {
			skillsClient, err = objstore.NewMinIOClient(minioEndpoint, minioAccessKey, minioSecretKey, minioBucket, false)
			if err != nil {
				slog.Warn("minio_client_init_failed", "endpoint", minioEndpoint, "error", err)
				skillsClient = nil
			} else {
				if ensureErr := skillsClient.EnsureBucket(context.Background()); ensureErr != nil {
					slog.Warn("minio_bucket_init_failed", "bucket", minioBucket, "error", ensureErr)
				} else {
					slog.Info("MinIO client initialized", "endpoint", minioEndpoint, "bucket", minioBucket)
				}
			}
		}
	}

	// ── 7. Build API server config ───────────────────────────
	serverCfg := api.APIServerConfig{
		Port:           port,
		Store:          dataStore,
		DB:             pgStore,
		Pool:           pgStore.Pool(),
		AuthChain:      authChain,
		RBAC:           rbacCfg,
		RateLimit:      rateLimitCfg,
		AllowedOrigins: allowedOrigins,
		StaticDir:      staticDir,
		SkillsClient:   skillsClient,
	}

	saasServer := api.NewAPIServer(serverCfg)

	// ── 8. Optionally start ACP server ────────────────────────
	var acpServer *acp.ACPServer
	if acpPortStr := os.Getenv("HERMES_ACP_PORT"); acpPortStr != "" {
		if acpPort, err := strconv.Atoi(acpPortStr); err == nil && acpPort > 0 {
			acpServer = acp.NewACPServer(acpPort)
		}
	}

	// ── 9. OpenAI-compatible adapter ─────────────────────────
	if apiKey != "" {
		adapterPortStr := os.Getenv("HERMES_API_PORT")
		if adapterPortStr == "" {
			adapterPortStr = "8081"
		}
		if adapterPort, err := strconv.Atoi(adapterPortStr); err == nil && adapterPort > 0 {
			adapter := platforms.NewAPIServerAdapter(adapterPort, apiKey)
			go func() {
				slog.Info("OpenAI API adapter starting", "port", adapterPort)
				if err := adapter.Connect(context.Background()); err != nil {
					slog.Warn("API adapter error", "error", err)
				}
			}()
			_ = adapter
		}
	}

	// ── 10. Signal handling ──────────────────────────────────
	done := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("Shutting down...")
		ctx := context.Background()
		if acpServer != nil {
			_ = acpServer.Stop()
		}
		_ = saasServer.Shutdown(ctx)
		_ = dataStore.Close()
		close(done)
	}()

	// ── 10. Start ACP server ────────────────────────────────
	if acpServer != nil {
		go func() {
			slog.Info("ACP server starting", "port", os.Getenv("HERMES_ACP_PORT"))
			if err := acpServer.Start(); err != nil {
				slog.Warn("ACP server error", "error", err)
			}
		}()
	}

	slog.Info("SaaS API server running",
		"port", port,
		"openapi", fmt.Sprintf("http://localhost:%d/v1/openapi", port),
		"admin", fmt.Sprintf("http://localhost:%d/admin.html", port),
		"health_live", fmt.Sprintf("http://localhost:%d/health/live", port),
		"health_ready", fmt.Sprintf("http://localhost:%d/health/ready", port),
	)

	err = saasServer.Start()
	<-done
	return err
}

// seedDefaultTenant creates the default SaaS tenant if it does not already exist.
// It is idempotent — calling it multiple times is safe.
func seedDefaultTenant(ctx context.Context, pgStore *pg.PGStore) error {
	const defaultTenantID = "00000000-0000-0000-0000-000000000001"

	_, err := pgStore.Tenants().Get(ctx, defaultTenantID)
	if err == nil {
		// Tenant already exists.
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("seedDefaultTenant: get tenant: %w", err)
	}

	// Create the default tenant.
	defaultTenant := &store.Tenant{
		ID:           defaultTenantID,
		Name:         "Default Tenant",
		Plan:         "pro",
		RateLimitRPM: 120,
		MaxSessions:  10,
	}
	if err := pgStore.Tenants().Create(ctx, defaultTenant); err != nil {
		return fmt.Errorf("seedDefaultTenant: create tenant: %w", err)
	}

	slog.Info("seeded default tenant", "id", defaultTenantID)
	return nil
}
