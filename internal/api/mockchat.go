package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/objstore"
	"github.com/hermes-agent/hermes-agent-go/internal/skills"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// chatMessage represents a single message in a session.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// mockChatHandler handles chat requests. It stores messages per (tenant, session)
// in PostgreSQL and calls a real LLM API to generate responses, enabling true
// multi-tenant isolation with real AI responses and persistent conversation history.
type mockChatHandler struct {
	store        store.Store
	llmURL       string
	llmAPIKey    string
	llmModel     string
	httpClient   *http.Client
	skillsClient *objstore.MinIOClient

	// skillsCache caches loaded skills per tenant with TTL to avoid
	// re-fetching from MinIO on every request.
	skillsCache   map[string]*skillsCacheEntry
	skillsCacheMu sync.RWMutex

	// soulCache caches loaded soul per tenant (long TTL since souls change rarely).
	soulCache   map[string]*soulCacheEntry
	soulCacheMu sync.RWMutex
}

type skillsCacheEntry struct {
	entries  []*skills.SkillEntry
	loadedAt time.Time
}

type soulCacheEntry struct {
	content  string
	loadedAt time.Time
}

const skillsCacheTTL = 5 * time.Minute
const soulCacheTTL = 30 * time.Minute

// defaultSoul is used when no per-tenant soul is found in MinIO.
const defaultSoul = `You are a helpful, knowledgeable, and friendly AI assistant.
Be concise, accurate, and helpful in all your responses.`

func newMockChatHandler(s store.Store, skillsClient *objstore.MinIOClient) *mockChatHandler {
	return &mockChatHandler{
		store:        s,
		llmURL:       getEnvOr("LLM_API_URL", "http://localhost:8000"),
		llmAPIKey:    getEnvOr("LLM_API_KEY", "123456"),
		llmModel:     getEnvOr("LLM_MODEL", "Qwen3-Coder-Next-4bit"),
		httpClient:   &http.Client{Timeout: 120 * time.Second},
		skillsClient: skillsClient,
		skillsCache:  make(map[string]*skillsCacheEntry),
		soulCache:    make(map[string]*soulCacheEntry),
	}
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

// chatReq / chatResp match the OpenAI /v1/chat/completions format.
type chatReq struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
}

type chatResp struct {
	ID      string           `json:"id"`
	Object string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []chatChoice    `json:"choices"`
	Usage   chatUsage        `json:"usage"`
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

func (h *mockChatHandler) callLLM(ctx context.Context, tenantID, sessionID string, messages []chatMessage) (string, error) {
	// Build system prompt with tenant context
	// Strip dashes from UUID so the first 8 chars are a compact, unique prefix.
	tenantCompact := strings.ReplaceAll(tenantID, "-", "")

	// Load per-tenant soul (persona) and skills from MinIO (both cached).
	soulPrompt := h.getSoulPrompt(ctx, tenantID)
	skillsPrompt := h.getSkillsPrompt(ctx, tenantID)
	slog.Info("LLM_request",
		"tenant", tenantID,
		"session", sessionID,
		"soul_bytes", len(soulPrompt),
		"skills_bytes", len(skillsPrompt),
		"soul_preview", fmt.Sprintf("%.80s", soulPrompt),
	)

	systemPrompt := fmt.Sprintf(
		"%s\n\n%s\n\nYour tenant ID is '%s' and session ID is '%s'. "+
			"Include BOTH in every reply in this exact format: [T:%s S:%s] — then your answer.",
		soulPrompt, skillsPrompt, tenantID, sessionID, tenantCompact[:8], sessionID,
	)

	// Prepend system prompt
	llmMessages := append([]chatMessage{{Role: "system", Content: systemPrompt}}, messages...)

	payload := map[string]any{
		"model":    h.llmModel,
		"messages": llmMessages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.llmURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.llmAPIKey)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call LLM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", fmt.Errorf("LLM returned HTTP %d: %s", resp.StatusCode, string(b))
	}

	var llmResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return "", fmt.Errorf("decode LLM response: %w", err)
	}
	if len(llmResp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}
	return llmResp.Choices[0].Message.Content, nil
}

// getSkillsPrompt returns the skills section of the system prompt for a tenant.
// It caches loaded skills per tenant to avoid repeated MinIO fetches.
// When MinIO is unavailable, it returns an empty string gracefully.
func (h *mockChatHandler) getSkillsPrompt(ctx context.Context, tenantID string) string {
	if h.skillsClient == nil {
		return ""
	}

	// Check cache first.
	h.skillsCacheMu.RLock()
	entry, ok := h.skillsCache[tenantID]
	cacheValid := ok && time.Since(entry.loadedAt) < skillsCacheTTL
	if cacheValid {
		h.skillsCacheMu.RUnlock()
		return skills.BuildSkillsPrompt(entry.entries)
	}
	h.skillsCacheMu.RUnlock()

	// Load from MinIO.
	loader := skills.NewMinIOSkillLoader(h.skillsClient, tenantID)
	entries, err := loader.LoadAll(ctx)
	if err != nil || len(entries) == 0 {
		// Cache negative result briefly to avoid hammering MinIO.
		if err != nil {
			slog.Info("skills_load_failed", "tenant", tenantID, "error", err)
		}
		h.skillsCacheMu.Lock()
		h.skillsCache[tenantID] = &skillsCacheEntry{
			entries:  nil,
			loadedAt: time.Now(),
		}
		h.skillsCacheMu.Unlock()
		return ""
	}

	// Update cache.
	h.skillsCacheMu.Lock()
	h.skillsCache[tenantID] = &skillsCacheEntry{
		entries:  entries,
		loadedAt: time.Now(),
	}
	h.skillsCacheMu.Unlock()

	slog.Info("skills_loaded", "tenant", tenantID, "count", len(entries))
	return skills.BuildSkillsPrompt(entries)
}

// getSoulPrompt returns the persona/soul content for a tenant.
// It loads {tenantID}/SOUL.md from MinIO, caches it, and falls back to defaultSoul.
func (h *mockChatHandler) getSoulPrompt(ctx context.Context, tenantID string) string {
	if h.skillsClient == nil {
		return defaultSoul
	}

	// Check cache first.
	h.soulCacheMu.RLock()
	entry, ok := h.soulCache[tenantID]
	cacheValid := ok && time.Since(entry.loadedAt) < soulCacheTTL
	if cacheValid {
		h.soulCacheMu.RUnlock()
		return entry.content
	}
	h.soulCacheMu.RUnlock()

	// Load soul from MinIO: {tenantID}/SOUL.md
	key := fmt.Sprintf("%s/SOUL.md", tenantID)
	data, err := h.skillsClient.GetObject(ctx, key)
	if err != nil || len(data) == 0 {
		slog.Info("soul_not_found", "tenant", tenantID, "key", key, "error", err)
		h.soulCacheMu.Lock()
		h.soulCache[tenantID] = &soulCacheEntry{
			content:  defaultSoul,
			loadedAt: time.Now(),
		}
		h.soulCacheMu.Unlock()
		return defaultSoul
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		content = defaultSoul
	}

	h.soulCacheMu.Lock()
	h.soulCache[tenantID] = &soulCacheEntry{
		content:  content,
		loadedAt: time.Now(),
	}
	h.soulCacheMu.Unlock()

	slog.Info("soul_loaded", "tenant", tenantID, "bytes", len(data), "preview", fmt.Sprintf("%.50s", content))
	return content
}

// sendMsg is like Messages.Append but logs the result for debugging.
func (h *mockChatHandler) sendMsg(ctx context.Context, tenantID, sessionID string, role, content string) int64 {
	id, err := h.store.Messages().Append(ctx, tenantID, sessionID, &store.Message{
		TenantID:  tenantID,
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	})
	if err != nil {
		slog.Error("sendMsg_FAILED", "tenant", tenantID, "session", sessionID, "role", role, "error", err)
		return 0
	}
	slog.Info("sendMsg_OK", "tenant", tenantID, "session", sessionID, "role", role, "msg_id", id)
	return id
}

// ServeHTTP handles POST /v1/chat/completions.
// All messages are persisted to PostgreSQL and loaded from there on each request.
func (h *mockChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	tenantID := ac.TenantID

	sessionID := r.Header.Get("X-Hermes-Session-Id")
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess_%d", time.Now().UnixNano())
	}

	// Ensure session exists in PostgreSQL.
	sess, err := h.store.Sessions().Get(ctx, tenantID, sessionID)
	if err != nil || sess == nil {
		sess = &store.Session{
			ID:        sessionID,
			TenantID:  tenantID,
			Platform:  "api",
			UserID:    ac.Identity,
			Model:     h.llmModel,
			StartedAt: time.Now(),
		}
		if createErr := h.store.Sessions().Create(ctx, tenantID, sess); createErr != nil {
			slog.Warn("session_create_failed", "tenant", tenantID, "session", sessionID, "error", createErr)
			http.Error(w, "session creation failed", http.StatusInternalServerError)
			return
		}
	}

	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Persist incoming messages (skip empty or system messages).
	for _, msg := range req.Messages {
		if msg.Content == "" || msg.Role == "system" {
			continue
		}
		h.sendMsg(ctx, tenantID, sessionID, msg.Role, msg.Content)
	}

	// Load full conversation history from PostgreSQL.
	historyMsgs, err := h.store.Messages().List(ctx, tenantID, sessionID, 200, 0)
	if err != nil {
		slog.Warn("messages_list_failed", "tenant", tenantID, "session", sessionID, "error", err)
		historyMsgs = nil
	}
	history := make([]chatMessage, 0, len(historyMsgs))
	for _, m := range historyMsgs {
		history = append(history, chatMessage{Role: m.Role, Content: m.Content})
	}

	// Call LLM.
	reply, err := h.callLLM(ctx, tenantID, sessionID, history)
	if err != nil {
		slog.Warn("LLM call failed", "error", err, "tenant", tenantID)
		http.Error(w, fmt.Sprintf("LLM error: %v", err), http.StatusBadGateway)
		return
	}

	// Persist assistant response.
	msgID := h.sendMsg(ctx, tenantID, sessionID, "assistant", reply)

	// Estimate tokens and update session counters.
	approxTokens := (len(req.Messages)*50 + len(reply)) / 4
	h.store.Sessions().UpdateTokens(ctx, tenantID, sessionID, store.TokenDelta{
		Input:  approxTokens / 2,
		Output: approxTokens / 2,
	})

	slog.Info("chat_completion",
		"tenant", tenantID,
		"session", sessionID,
		"messages", len(history),
		"msg_id", msgID,
		"soul_loaded", h.soulCache != nil,
		"auth_method", ac.AuthMethod,
		"model", h.llmModel,
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
			CompletionTokens: len(reply) / 4,
			TotalTokens:     42 + len(reply)/4,
		},
	})
}

// sessionsResp returns session info for GET /v1/mock-sessions.
type sessionsResp struct {
	TenantID string        `json:"tenant_id"`
	Sessions []sessionInfo `json:"sessions"`
}

type sessionInfo struct {
	SessionID  string `json:"session_id"`
	MessageCount int  `json:"message_count"`
}

// handleMockSessionList handles GET /v1/mock-sessions.
func (h *mockChatHandler) handleSessionList(w http.ResponseWriter, r *http.Request) {
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	tenantID := ac.TenantID
	sessions, _, err := h.store.Sessions().List(ctx, tenantID, store.ListOptions{Limit: 100, Offset: 0})
	if err != nil {
		sessions = nil
	}

	result := make([]sessionInfo, 0, len(sessions))
	for _, s := range sessions {
		count, _ := h.store.Messages().CountBySession(ctx, tenantID, s.ID)
		result = append(result, sessionInfo{
			SessionID:   s.ID,
			MessageCount: count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sessionsResp{
		TenantID: tenantID,
		Sessions: result,
	})
	_ = err // suppress unused variable warning when sessions == nil
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
	_ = h.store.Sessions().Delete(r.Context(), ac.TenantID, sessionID)

	w.WriteHeader(http.StatusNoContent)
}

// NewMockChatHandler creates a mock chat handler wired into the SaaS API server.
func NewMockChatHandler(s store.Store, skillsClient *objstore.MinIOClient) *mockChatHandler {
	return newMockChatHandler(s, skillsClient)
}
