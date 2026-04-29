package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
)

// chatMessage represents a single message in a mock chat session.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// mockChatStore is a thread-safe in-memory store for mock chat sessions.
// Messages are scoped to (tenantID, sessionID).
type mockChatStore struct {
	mu       sync.RWMutex
	messages map[string][]chatMessage // key: "tenantID:sessionID"
}

func newMockChatStore() *mockChatStore {
	return &mockChatStore{messages: make(map[string][]chatMessage)}
}

func (s *mockChatStore) sessionKey(tenantID, sessionID string) string {
	return tenantID + ":" + sessionID
}

func (s *mockChatStore) GetMessages(tenantID, sessionID string) []chatMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.messages[s.sessionKey(tenantID, sessionID)]
}

func (s *mockChatStore) AppendMessage(tenantID, sessionID, role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.sessionKey(tenantID, sessionID)
	s.messages[key] = append(s.messages[key], chatMessage{Role: role, Content: content})
}

func (s *mockChatStore) ClearSession(tenantID, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messages, s.sessionKey(tenantID, sessionID))
}

// mockChatHandler handles mock chat requests for multi-tenant isolation testing.
// It stores messages per (tenant, session) and returns deterministic, tenant-aware
// mock responses so tests can verify isolation.
type mockChatHandler struct {
	store *mockChatStore
}

func newMockChatHandler() *mockChatHandler {
	return &mockChatHandler{store: newMockChatStore()}
}

// chatReq / chatResp match the OpenAI /v1/chat/completions format.
type chatReq struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
}

type chatResp struct {
	ID        string           `json:"id"`
	Object    string           `json:"object"`
	Created   int64            `json:"created"`
	Model     string           `json:"model"`
	Choices   []chatChoice    `json:"choices"`
	Usage     chatUsage        `json:"usage"`
}

type chatChoice struct {
	Index        int          `json:"index"`
	Message      chatMessage  `json:"message"`
	FinishReason string       `json:"finish_reason"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens     int `json:"total_tokens"`
}

// generateMockResponse creates a deterministic, tenant-aware mock response.
// The response text varies by tenant and conversation to make isolation visible.
func (h *mockChatHandler) generateMockResponse(tenantID, sessionID string, messages []chatMessage) string {
	// Count messages to vary response by conversation depth
	userCount := 0
	for _, m := range messages {
		if m.Role == "user" {
			userCount++
		}
	}

	// Build a hash of tenant+session for deterministic variation
	hashed := sha256.Sum256([]byte(tenantID + ":" + sessionID))
	hashStr := hex.EncodeToString(hashed[:8])

	// Truncate tenant ID for display
	tenantShort := tenantID
	if len(tenantID) > 8 {
		tenantShort = tenantID[:8]
	}

	responses := []string{
		fmt.Sprintf("[%s] Understood! I'm ready to help. (session: %s, depth: %d, hash: %s)",
			tenantShort, sessionID[:8], userCount, hashStr),
		fmt.Sprintf("[%s] Got it! Processing your request. (tenant: %s, messages so far: %d)",
			tenantShort, tenantID, userCount),
		fmt.Sprintf("[%s] Roger that! This is a mock response. (session: %s, turn: %d)",
			tenantShort, sessionID, userCount),
		fmt.Sprintf("[%s] Acknowledged. I'm a test bot for tenant %s, session %s, turn %d.",
			tenantShort, tenantID, sessionID[:8], userCount),
		fmt.Sprintf("[%s] Echo: received your message. (tenant=%s, session=%s, #%d)",
			tenantShort, tenantID, sessionID[:8], userCount),
	}

	// Deterministic selection based on message count
	idx := userCount % len(responses)
	return responses[idx]
}

// ServeHTTP handles POST /v1/chat/completions (mock).
func (h *mockChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract auth context (set by Auth middleware).
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tenantID := ac.TenantID

	// Session ID from header or generate one.
	sessionID := r.Header.Get("X-Hermes-Session-Id")
	if sessionID == "" {
		sessionID = fmt.Sprintf("mock_%d", time.Now().UnixNano())
	}

	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Append all messages to session store (for isolation verification).
	for _, msg := range req.Messages {
		if msg.Content == "" {
			continue
		}
		h.store.AppendMessage(tenantID, sessionID, msg.Role, msg.Content)
	}

	// Get conversation history for context.
	history := h.store.GetMessages(tenantID, sessionID)

	// Generate response.
	reply := h.generateMockResponse(tenantID, sessionID, history)

	// Append assistant response to session store.
	h.store.AppendMessage(tenantID, sessionID, "assistant", reply)

	slog.Info("mock_chat",
		"tenant", tenantID,
		"session", sessionID,
		"messages", len(history),
		"auth_method", ac.AuthMethod,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResp{
		ID:      sessionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []chatChoice{{
			Index:        0,
			Message:      chatMessage{Role: "assistant", Content: reply},
			FinishReason: "stop",
		}},
		Usage: chatUsage{
			PromptTokens:     42,
			CompletionTokens: 20,
			TotalTokens:     62,
		},
	})
}

// sessionsResp returns session info for GET /v1/mock-sessions.
type sessionsResp struct {
	TenantID string         `json:"tenant_id"`
	Sessions []sessionInfo  `json:"sessions"`
}

type sessionInfo struct {
	SessionID string `json:"session_id"`
	Messages  int    `json:"message_count"`
}

// handleMockSessionList handles GET /v1/mock-sessions.
func (h *mockChatHandler) handleSessionList(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	h.store.mu.RLock()
	defer h.store.mu.RUnlock()

	prefix := ac.TenantID + ":"
	var sessions []sessionInfo
	for key, msgs := range h.store.messages {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		sessionID := strings.TrimPrefix(key, prefix)
		sessions = append(sessions, sessionInfo{
			SessionID: sessionID,
			Messages:  len(msgs),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionsResp{
		TenantID: ac.TenantID,
		Sessions: sessions,
	})
}

// handleMockClearSession handles DELETE /v1/mock-sessions/:id.
func (h *mockChatHandler) handleClearSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/v1/mock-sessions/")
	h.store.ClearSession(ac.TenantID, sessionID)

	w.WriteHeader(http.StatusNoContent)
}

// MockChatStore returns the underlying store for inspection (used by tests).
func (h *mockChatHandler) MockChatStore() *mockChatStore {
	return h.store
}

// NewMockChatHandler creates a mock chat handler wired into the SaaS API server.
func NewMockChatHandler() *mockChatHandler {
	return newMockChatHandler()
}
