package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/hermes-agent/hermes-agent-go/internal/objstore"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatHandler holds shared dependencies for agent chat, session, and memory endpoints.
type chatHandler struct {
	store        store.Store
	pool         *pgxpool.Pool
	llmURL       string
	llmAPIKey    string
	llmModel     string
	httpClient   *http.Client
	skillsClient *objstore.MinIOClient

	soulCache *lru.LRU[string, string]
}

const soulCacheTTL = 30 * time.Minute
const soulCacheMaxEntries = 500

const defaultSoul = `You are a helpful, knowledgeable, and friendly AI assistant.
Be concise, accurate, and helpful in all your responses.`

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

// chatReq / chatResp match the OpenAI /v1/chat/completions format.
type chatReq struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResp struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
}

type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (h *chatHandler) getSoulPrompt(ctx context.Context, tenantID string) string {
	if content, ok := h.soulCache.Get(tenantID); ok {
		return content
	}

	if h.skillsClient == nil {
		return defaultSoul
	}

	raw, err := h.skillsClient.GetObject(ctx, tenantID+"/SOUL.md")
	if err != nil {
		slog.Debug("soul_load_fallback", "tenant", tenantID, "error", err)
		return defaultSoul
	}
	content := string(raw)

	h.soulCache.Add(tenantID, content)
	return content
}

func (h *chatHandler) sendMsg(ctx context.Context, tenantID, sessionID string, role, content string) int64 {
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

// NewChatHandler creates the chat handler wired into the SaaS API server.
func NewChatHandler(s store.Store, pool *pgxpool.Pool, skillsClient *objstore.MinIOClient) *chatHandler {
	return &chatHandler{
		store:        s,
		pool:         pool,
		llmURL:       getEnvOr("LLM_API_URL", "http://localhost:8000"),
		llmAPIKey:    getEnvOr("LLM_API_KEY", ""),
		llmModel:     getEnvOr("LLM_MODEL", "Qwen3-Coder-Next-4bit"),
		httpClient:   &http.Client{Timeout: 120 * time.Second},
		skillsClient: skillsClient,
		soulCache:    lru.NewLRU[string, string](soulCacheMaxEntries, nil, soulCacheTTL),
	}
}
