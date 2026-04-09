// Package acp implements the Agent Communication Protocol server for
// editor integration (VS Code, Zed, JetBrains, etc.).
package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// AgentHandler is the interface that the ACP server uses to interact with
// the underlying AI agent. This avoids a direct dependency on the agent package.
type AgentHandler interface {
	Chat(message string) (string, error)
	Model() string
	SessionID() string
}

// ACPServer implements the Agent Communication Protocol HTTP server.
type ACPServer struct {
	agent  AgentHandler
	port   int
	server *http.Server
	mu     sync.Mutex
}

// ChatRequest is the request body for POST /v1/chat.
type ChatRequest struct {
	Message   string         `json:"message"`
	SessionID string         `json:"session_id,omitempty"`
	Model     string         `json:"model,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
}

// ChatResponse is the response body for POST /v1/chat.
type ChatResponse struct {
	Response  string `json:"response"`
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
}

// StatusResponse is the response body for GET /v1/status.
type StatusResponse struct {
	Status    string `json:"status"`
	Model     string `json:"model"`
	SessionID string `json:"session_id"`
	Version   string `json:"version"`
	Uptime    string `json:"uptime"`
}

// ToolRequest is the request body for POST /v1/tool.
type ToolRequest struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
	SessionID string         `json:"session_id,omitempty"`
}

// ToolResponse is the response body for POST /v1/tool.
type ToolResponse struct {
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

// NewACPServer creates a new ACP server on the given port.
// If port is 0, the default port 3000 is used.
func NewACPServer(port int) *ACPServer {
	if port == 0 {
		port = 3000
	}
	return &ACPServer{
		port: port,
	}
}

// SetAgent attaches an agent handler to the server.
func (s *ACPServer) SetAgent(agent AgentHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agent = agent
}

// Start begins serving HTTP requests. It blocks until Stop is called.
func (s *ACPServer) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/chat", s.handleChat)
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("POST /v1/tool", s.handleTool)
	mux.HandleFunc("GET /v1/health", s.handleHealth)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      withCORS(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("ACP server starting", "port", s.port)

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("acp server listen: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the server.
func (s *ACPServer) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	slog.Info("ACP server shutting down")
	return s.server.Shutdown(ctx)
}

// Port returns the configured port.
func (s *ACPServer) Port() int {
	return s.port
}

// handleChat processes POST /v1/chat requests.
func (s *ACPServer) handleChat(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	agent := s.agent
	s.mu.Unlock()

	if agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent not initialized")
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	response, err := agent.Chat(req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chat error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{
		Response:  response,
		SessionID: agent.SessionID(),
		Model:     agent.Model(),
	})
}

// handleStatus returns the server status.
func (s *ACPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	agent := s.agent
	s.mu.Unlock()

	status := StatusResponse{
		Status:  "running",
		Version: "dev",
	}

	if agent != nil {
		status.Model = agent.Model()
		status.SessionID = agent.SessionID()
	}

	writeJSON(w, http.StatusOK, status)
}

// handleTool processes POST /v1/tool requests.
func (s *ACPServer) handleTool(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	agent := s.agent
	s.mu.Unlock()

	if agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent not initialized")
		return
	}

	var req ToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Tool == "" {
		writeError(w, http.StatusBadRequest, "tool name is required")
		return
	}

	// Delegate tool execution to the agent via a chat message that invokes the tool.
	// In a full implementation this would call the tool registry directly.
	prompt := fmt.Sprintf("Please execute the %s tool with these arguments: %v", req.Tool, req.Arguments)
	result, err := agent.Chat(prompt)
	if err != nil {
		writeJSON(w, http.StatusOK, ToolResponse{
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, ToolResponse{
		Result: result,
	})
}

// handleHealth is a simple health check endpoint.
func (s *ACPServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// withCORS wraps a handler to add CORS headers for editor integration.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
