package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

//go:embed static
var staticFS embed.FS

// Dashboard serves the web management interface.
type Dashboard struct {
	port           int
	DashboardToken string // Bearer token for write endpoints; empty = dev mode (no auth)
	server         *http.Server

	// Handlers for data endpoints
	GetSessions func() (any, error)
	GetConfig   func() (any, error)
	GetSkills   func() (any, error)
	GetGateways func() (any, error)
	SaveConfig  func(data map[string]any) error
}

// New creates a dashboard on the given port.
func New(port int) *Dashboard {
	return &Dashboard{port: port}
}

// Start launches the dashboard HTTP server.
func (d *Dashboard) Start() error {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/sessions", d.handleSessions)
	mux.HandleFunc("/api/config", d.handleConfig)
	mux.HandleFunc("/api/skills", d.handleSkills)
	mux.HandleFunc("/api/gateways", d.handleGateways)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Static files (embedded SPA)
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("embed fs: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticContent)))

	d.server = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", d.port),
		Handler:      withDashboardCORS(d.port, mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	slog.Info("Dashboard starting", "port", d.port, "url", fmt.Sprintf("http://localhost:%d", d.port))

	go func() {
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Dashboard error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the dashboard.
func (d *Dashboard) Stop() error {
	if d.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.server.Shutdown(ctx)
}

func (d *Dashboard) handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if d.GetSessions == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	data, err := d.GetSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (d *Dashboard) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost && d.SaveConfig != nil {
		if d.DashboardToken != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+d.DashboardToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		var data map[string]any
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := d.SaveConfig(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
		return
	}

	if d.GetConfig == nil {
		json.NewEncoder(w).Encode(map[string]any{})
		return
	}
	data, err := d.GetConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (d *Dashboard) handleSkills(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if d.GetSkills == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	data, err := d.GetSkills()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (d *Dashboard) handleGateways(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if d.GetGateways == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	data, err := d.GetGateways()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func withDashboardCORS(port int, next http.Handler) http.Handler {
	allowed := fmt.Sprintf("http://localhost:%d", port)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == allowed {
			w.Header().Set("Access-Control-Allow-Origin", allowed)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
