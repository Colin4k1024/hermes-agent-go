package api

import (
	crypto_rand "crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/agent"
	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/llm"
	"github.com/hermes-agent/hermes-agent-go/internal/skills"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	"github.com/hermes-agent/hermes-agent-go/internal/tools"
)

// ServeAgentHTTP handles POST /v1/agent/chat using the full AIAgent (tool loop, soul, skills, memory).
func (h *chatHandler) ServeAgentHTTP(w http.ResponseWriter, r *http.Request) {
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
	userID := ac.Identity

	sessionID := r.Header.Get("X-Hermes-Session-Id")
	if sessionID == "" {
		b := make([]byte, 16)
		_, _ = crypto_rand.Read(b)
		sessionID = fmt.Sprintf("sess_%x", b)
	}

	// Ensure session exists in PG.
	sess, err := h.store.Sessions().Get(ctx, tenantID, sessionID)
	if err != nil || sess == nil {
		sess = &store.Session{
			ID:        sessionID,
			TenantID:  tenantID,
			Platform:  "api",
			UserID:    userID,
			Model:     h.llmModel,
			StartedAt: time.Now(),
		}
		if createErr := h.store.Sessions().Create(ctx, tenantID, sess); createErr != nil {
			http.Error(w, "session creation failed", http.StatusInternalServerError)
			return
		}
	}

	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Extract the latest user message from the request.
	userMessage := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			userMessage = req.Messages[i].Content
			break
		}
	}
	if userMessage == "" {
		http.Error(w, "no user message found", http.StatusBadRequest)
		return
	}

	// Persist the user message to PG.
	h.sendMsg(ctx, tenantID, sessionID, "user", userMessage)

	// Load full conversation history from PG (includes the just-persisted message).
	historyMsgs, err := h.store.Messages().List(ctx, tenantID, sessionID, 200, 0)
	if err != nil {
		historyMsgs = nil
	}

	// Build []llm.Message history for the agent — everything except the last user message
	// (RunConversation takes userMessage separately and appends it internally).
	history := make([]llm.Message, 0, len(historyMsgs))
	for _, m := range historyMsgs {
		if m.Role == "system" {
			continue
		}
		history = append(history, llm.Message{Role: m.Role, Content: m.Content})
	}
	// Drop the last entry — it's the user message we just added (agent appends it itself).
	if len(history) > 0 && history[len(history)-1].Role == "user" {
		history = history[:len(history)-1]
	}

	// Load per-tenant soul from MinIO.
	soulContent := h.getSoulPrompt(ctx, tenantID)

	// Build skill loader for this tenant.
	var skillLoader skills.SkillLoader
	if h.skillsClient != nil {
		skillLoader = skills.NewMinIOSkillLoader(h.skillsClient, tenantID)
	}

	// Build PG memory provider.
	var memProvider tools.MemoryProvider
	if h.pool != nil {
		memProvider = agent.NewPGMemoryProvider(h.pool, tenantID, userID)
	}

	// Build the agent with all SaaS-mode options.
	a, err := agent.New(
		agent.WithBaseURL(h.llmURL),
		agent.WithAPIKey(h.llmAPIKey),
		agent.WithModel(h.llmModel),
		agent.WithTenantID(tenantID),
		agent.WithUserID(userID),
		agent.WithSessionID(sessionID),
		agent.WithPlatform("api"),
		agent.WithSkipContextFiles(true), // no local filesystem; soul comes from MinIO
		agent.WithPersistSession(false),  // we persist to PG ourselves
		agent.WithSoulContent(soulContent),
		agent.WithSkillLoader(skillLoader),
		agent.WithMemoryProvider(memProvider),
	)
	if err != nil {
		slog.Error("agent_create_failed", "tenant", tenantID, "error", err)
		http.Error(w, fmt.Sprintf("agent creation failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer a.Close()

	// Run the agent with the full conversation history.
	result, err := a.RunConversation(userMessage, history)
	if err != nil {
		slog.Error("agent_run_failed", "tenant", tenantID, "session", sessionID, "error", err)
		http.Error(w, fmt.Sprintf("agent error: %v", err), http.StatusBadGateway)
		return
	}

	reply := result.FinalResponse

	// Persist assistant reply to PG.
	msgID := h.sendMsg(ctx, tenantID, sessionID, "assistant", reply)

	// Run rule-based memory extractor on the user message.
	if h.pool != nil {
		extractor := &memoryExtractor{pool: h.pool}
		if memories := extractor.extract(userMessage); len(memories) > 0 {
			extractor.persist(tenantID, userID, memories)
		}
	}

	// Update session token counters.
	h.store.Sessions().UpdateTokens(ctx, tenantID, sessionID, store.TokenDelta{
		Input:  result.InputTokens,
		Output: result.OutputTokens,
	})

	slog.Info("agent_chat_completion",
		"tenant", tenantID,
		"session", sessionID,
		"api_calls", result.APICalls,
		"input_tokens", result.InputTokens,
		"output_tokens", result.OutputTokens,
		"msg_id", msgID,
	)

	if req.Stream {
		h.serveSSE(w, r, sessionID, reply, result)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResp{
		ID:      sessionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   h.llmModel,
		Choices: []chatChoice{{
			Index:        0,
			Message:      chatMessage{Role: "assistant", Content: reply},
			FinishReason: "stop",
		}},
		Usage: chatUsage{
			PromptTokens:     result.InputTokens,
			CompletionTokens: result.OutputTokens,
			TotalTokens:      result.TotalTokens,
		},
	})
}

// SSE chunk types matching OpenAI chat.completion.chunk format.
type sseChunkResp struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []sseChunkDelta `json:"choices"`
}

type sseChunkDelta struct {
	Index        int             `json:"index"`
	Delta        sseChunkContent `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
}

type sseChunkContent struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

func (h *chatHandler) serveSSE(w http.ResponseWriter, r *http.Request, sessionID, reply string, result *agent.ConversationResult) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	created := time.Now().Unix()
	chunkID := "chatcmpl-" + sessionID

	writeSSE := func(data []byte) {
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// First chunk: role announcement.
	roleChunk, _ := json.Marshal(sseChunkResp{
		ID: chunkID, Object: "chat.completion.chunk", Created: created, Model: h.llmModel,
		Choices: []sseChunkDelta{{Index: 0, Delta: sseChunkContent{Role: "assistant"}}},
	})
	writeSSE(roleChunk)

	// Stream content word-by-word with small delay for natural feel.
	// A 15-second heartbeat keeps the connection alive during long pauses.
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	words := strings.Fields(reply)
	for i, word := range words {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		default:
		}

		content := word
		if i < len(words)-1 {
			content += " "
		}

		chunk, _ := json.Marshal(sseChunkResp{
			ID: chunkID, Object: "chat.completion.chunk", Created: created, Model: h.llmModel,
			Choices: []sseChunkDelta{{Index: 0, Delta: sseChunkContent{Content: content}}},
		})
		writeSSE(chunk)
		time.Sleep(30 * time.Millisecond)
	}

	// Final chunk: finish_reason=stop.
	stop := "stop"
	finalChunk, _ := json.Marshal(sseChunkResp{
		ID: chunkID, Object: "chat.completion.chunk", Created: created, Model: h.llmModel,
		Choices: []sseChunkDelta{{Index: 0, Delta: sseChunkContent{}, FinishReason: &stop}},
	})
	writeSSE(finalChunk)

	// Terminator.
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}
