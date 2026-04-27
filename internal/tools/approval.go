package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ApprovalScope defines how broadly an approval applies.
type ApprovalScope string

const (
	ApproveOnce      ApprovalScope = "once"
	ApproveSession   ApprovalScope = "session"
	ApprovePermanent ApprovalScope = "permanent"
	ApproveDeny      ApprovalScope = "deny"
)

// DefaultApprovalTimeout is the default time to wait for user approval.
const DefaultApprovalTimeout = 60 * time.Second

// ApprovalRequest contains details about a command needing approval.
type ApprovalRequest struct {
	Command    string   `json:"command"`
	Reason     string   `json:"reason"`
	PatternKey string   `json:"pattern_key"`
	AllReasons []string `json:"all_reasons,omitempty"`
	SessionKey string   `json:"session_key"`
}

// ApprovalResult contains the user's decision.
type ApprovalResult struct {
	Approved bool          `json:"approved"`
	Scope    ApprovalScope `json:"scope"`
}

// ApprovalHandler is the interface for requesting user approval.
// Implementations handle CLI prompts, gateway messages, etc.
type ApprovalHandler interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResult, error)
}

// Compile-time interface verification (规范五).
var _ ApprovalHandler = (*GatewayApprovalHandler)(nil)

// DangerousPatterns contains regex patterns for dangerous commands.
var DangerousPatterns = []struct {
	Pattern *regexp.Regexp
	Reason  string
}{
	{regexp.MustCompile(`\brm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+|--force\s+)?(-[a-zA-Z]*r[a-zA-Z]*|--recursive)\s`), "recursive file deletion (rm -rf)"},
	{regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*\s+|--recursive\s+)?(-[a-zA-Z]*f[a-zA-Z]*|--force)\s`), "forced file deletion (rm -f)"},
	{regexp.MustCompile(`(?i)\bDROP\s+(TABLE|DATABASE|SCHEMA|INDEX)\b`), "SQL DROP statement"},
	{regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+\S+\s*(;|$)`), "SQL DELETE without WHERE clause"},
	{regexp.MustCompile(`(?i)\bTRUNCATE\s+(TABLE\s+)?\S+`), "SQL TRUNCATE statement"},
	{regexp.MustCompile(`\bgit\s+push\s+.*--force\b`), "git force push"},
	{regexp.MustCompile(`\bgit\s+push\s+-f\b`), "git force push (-f)"},
	{regexp.MustCompile(`\bgit\s+reset\s+--hard\b`), "git hard reset"},
	{regexp.MustCompile(`\bgit\s+clean\s+.*-f`), "git clean with force"},
	{regexp.MustCompile(`\bchmod\s+(-[a-zA-Z]*R[a-zA-Z]*\s+)?777\b`), "setting world-writable permissions"},
	{regexp.MustCompile(`\b(mkfs|fdisk|dd\s+if=)\b`), "disk formatting or low-level write"},
	{regexp.MustCompile(`>\s*/dev/sd[a-z]`), "writing directly to disk device"},
	{regexp.MustCompile(`(?i)\b(shutdown|reboot|halt|poweroff)\b`), "system shutdown/reboot"},
	{regexp.MustCompile(`\bkubectl\s+delete\s+(namespace|ns|deployment|pod)\b`), "kubernetes resource deletion"},
	{regexp.MustCompile(`\bcurl\s+.*\|\s*(bash|sh|zsh)\b`), "piping remote script to shell"},
	{regexp.MustCompile(`\bwget\s+.*-O\s*-\s*\|\s*(bash|sh|zsh)\b`), "piping remote script to shell"},
	{regexp.MustCompile(`\b:\(\)\s*\{\s*:\|\:&\s*\}\s*;`), "fork bomb"},
	{regexp.MustCompile(`>\s*/etc/(passwd|shadow|sudoers|hosts)\b`), "overwriting critical system file"},
	{regexp.MustCompile(`\bsudo\s+rm\b`), "deleting files with sudo"},
	{regexp.MustCompile(`\brm\s+.*(/|~|\$HOME)\s*$`), "deleting root or home directory"},
}

// IsDangerousCommand checks if a command matches any dangerous patterns.
func IsDangerousCommand(cmd string) (bool, string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false, ""
	}
	for _, dp := range DangerousPatterns {
		if dp.Pattern.MatchString(cmd) {
			return true, dp.Reason
		}
	}
	return false, ""
}

// GetAllDangerousReasons returns all matching danger reasons for a command.
func GetAllDangerousReasons(cmd string) []string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}
	var reasons []string
	for _, dp := range DangerousPatterns {
		if dp.Pattern.MatchString(cmd) {
			reasons = append(reasons, dp.Reason)
		}
	}
	return reasons
}

// --- Approval Store (per-session + permanent memory) ---

// ApprovalStore manages approved command patterns. Thread-safe.
type ApprovalStore struct {
	mu                sync.RWMutex
	sessionApproved   map[string]map[string]bool // sessionKey → approved patterns
	permanentApproved map[string]bool
}

// NewApprovalStore creates a new ApprovalStore.
func NewApprovalStore() *ApprovalStore {
	return &ApprovalStore{
		sessionApproved:   make(map[string]map[string]bool),
		permanentApproved: make(map[string]bool),
	}
}

// IsApproved checks if a pattern is approved for the session or permanently.
func (s *ApprovalStore) IsApproved(sessionKey, patternKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.permanentApproved[patternKey] {
		return true
	}
	if sess, ok := s.sessionApproved[sessionKey]; ok {
		return sess[patternKey]
	}
	return false
}

// ApproveForSession marks a pattern as approved for the given session.
func (s *ApprovalStore) ApproveForSession(sessionKey, patternKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessionApproved[sessionKey] == nil {
		s.sessionApproved[sessionKey] = make(map[string]bool)
	}
	s.sessionApproved[sessionKey][patternKey] = true
}

// ApprovePermanently marks a pattern as permanently approved.
func (s *ApprovalStore) ApprovePermanently(patternKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permanentApproved[patternKey] = true
}

// ClearSession removes all session approvals for a session.
func (s *ApprovalStore) ClearSession(sessionKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessionApproved, sessionKey)
}

// LoadPermanent loads permanently approved patterns.
func (s *ApprovalStore) LoadPermanent(patterns []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range patterns {
		s.permanentApproved[p] = true
	}
}

// PermanentPatterns returns a copy of all permanently approved patterns.
func (s *ApprovalStore) PermanentPatterns() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 规范四：边界处拷贝 slice 防止外部篡改
	result := make([]string, 0, len(s.permanentApproved))
	for p := range s.permanentApproved {
		result = append(result, p)
	}
	return result
}

var globalApprovalStore = NewApprovalStore()

// GlobalApprovalStore returns the global approval store.
func GlobalApprovalStore() *ApprovalStore { return globalApprovalStore }

// --- Gateway Approval Queue (channel-based blocking) ---

type pendingApproval struct {
	Request  ApprovalRequest
	ResultCh chan ApprovalResult
}

// GatewayApprovalQueue manages pending approvals for gateway sessions.
type GatewayApprovalQueue struct {
	mu     sync.Mutex
	queues map[string][]*pendingApproval
}

// NewGatewayApprovalQueue creates a new queue.
func NewGatewayApprovalQueue() *GatewayApprovalQueue {
	return &GatewayApprovalQueue{
		queues: make(map[string][]*pendingApproval),
	}
}

// Submit adds a pending approval and returns a channel for the result.
func (q *GatewayApprovalQueue) Submit(sessionKey string, req ApprovalRequest) <-chan ApprovalResult {
	ch := make(chan ApprovalResult, 1) // allocate outside lock

	q.mu.Lock()
	pa := &pendingApproval{
		Request:  req,
		ResultCh: ch,
	}
	q.queues[sessionKey] = append(q.queues[sessionKey], pa)
	q.mu.Unlock()

	return ch
}

// Resolve resolves the oldest pending approval (FIFO). Returns count resolved.
func (q *GatewayApprovalQueue) Resolve(sessionKey string, result ApprovalResult) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	queue, ok := q.queues[sessionKey]
	if !ok || len(queue) == 0 {
		return 0
	}

	pa := queue[0]
	q.queues[sessionKey] = queue[1:]
	if len(q.queues[sessionKey]) == 0 {
		delete(q.queues, sessionKey)
	}

	pa.ResultCh <- result
	close(pa.ResultCh)
	return 1
}

// ResolveAll resolves all pending approvals for a session.
func (q *GatewayApprovalQueue) ResolveAll(sessionKey string, result ApprovalResult) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	queue, ok := q.queues[sessionKey]
	if !ok || len(queue) == 0 {
		return 0
	}

	count := len(queue)
	for _, pa := range queue {
		pa.ResultCh <- result
		close(pa.ResultCh)
	}
	delete(q.queues, sessionKey)
	return count
}

// HasPending returns true if there are pending approvals for a session.
func (q *GatewayApprovalQueue) HasPending(sessionKey string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queues[sessionKey]) > 0
}

// PendingCount returns the number of pending approvals for a session.
func (q *GatewayApprovalQueue) PendingCount(sessionKey string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queues[sessionKey])
}

// ClearSession removes all pending approvals, signaling denial.
func (q *GatewayApprovalQueue) ClearSession(sessionKey string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, pa := range q.queues[sessionKey] {
		select {
		case pa.ResultCh <- ApprovalResult{Approved: false, Scope: ApproveDeny}:
		default:
		}
		close(pa.ResultCh)
	}
	delete(q.queues, sessionKey)
}

var globalGatewayQueue = NewGatewayApprovalQueue()

// GlobalGatewayApprovalQueue returns the global gateway approval queue.
func GlobalGatewayApprovalQueue() *GatewayApprovalQueue { return globalGatewayQueue }

// --- GatewayApprovalHandler ---

// NotifyFunc is called to send the approval request to the user.
type NotifyFunc func(req ApprovalRequest)

// GatewayApprovalHandler implements ApprovalHandler for messaging platforms.
type GatewayApprovalHandler struct {
	SessionKey string
	Timeout    time.Duration
	NotifyFn   NotifyFunc
}

// RequestApproval submits to the queue, notifies the user, and blocks.
func (h *GatewayApprovalHandler) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
	req.SessionKey = h.SessionKey

	timeout := h.Timeout
	if timeout == 0 {
		timeout = DefaultApprovalTimeout
	}

	resultCh := GlobalGatewayApprovalQueue().Submit(h.SessionKey, req)

	if h.NotifyFn != nil {
		h.NotifyFn(req)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case result := <-resultCh:
		return result, nil
	case <-ctx.Done():
		GlobalGatewayApprovalQueue().Resolve(h.SessionKey, ApprovalResult{
			Approved: false,
			Scope:    ApproveDeny,
		})
		return ApprovalResult{Approved: false, Scope: ApproveDeny},
			fmt.Errorf("approval timed out after %v", timeout)
	}
}

// --- CheckDangerousCommand (main entry point) ---

// SandboxedEnvironments are auto-approved (commands run in isolation).
var SandboxedEnvironments = map[string]bool{
	"docker":      true,
	"singularity": true,
	"modal":       true,
	"daytona":     true,
}

// CheckDangerousCommand detects dangerous patterns, checks session approvals,
// and requests user approval if needed.
//
// Returns a map with "approved" (bool) and "message" (string or nil).
func CheckDangerousCommand(command string, ctx *ToolContext) map[string]any {
	if ctx != nil && SandboxedEnvironments[ctx.Platform] {
		return map[string]any{"approved": true, "message": nil}
	}

	isDangerous, reason := IsDangerousCommand(command)
	if !isDangerous {
		return map[string]any{"approved": true, "message": nil}
	}

	allReasons := GetAllDangerousReasons(command)
	sessionKey := ""
	if ctx != nil {
		sessionKey = ctx.SessionID
	}

	if sessionKey != "" && globalApprovalStore.IsApproved(sessionKey, reason) {
		return map[string]any{"approved": true, "message": nil}
	}

	if ctx != nil && ctx.ApprovalHandler != nil {
		req := ApprovalRequest{
			Command:    command,
			Reason:     reason,
			PatternKey: reason,
			AllReasons: allReasons,
			SessionKey: sessionKey,
		}

		result, err := ctx.ApprovalHandler.RequestApproval(context.Background(), req)
		if err != nil {
			// 规范九：只 return，不 log（调用方决定是否 log）
			return map[string]any{
				"approved": false,
				"status":   "denied",
				"message": fmt.Sprintf(
					"dangerous command blocked (approval timed out): %s — command: %s",
					reason, command),
			}
		}

		if result.Approved {
			switch result.Scope {
			case ApproveSession:
				if sessionKey != "" {
					globalApprovalStore.ApproveForSession(sessionKey, reason)
				}
			case ApprovePermanent:
				if sessionKey != "" {
					globalApprovalStore.ApproveForSession(sessionKey, reason)
				}
				globalApprovalStore.ApprovePermanently(reason)
			}
			return map[string]any{"approved": true, "message": nil}
		}

		return map[string]any{
			"approved": false,
			"status":   "denied",
			"message": fmt.Sprintf(
				"blocked: user denied dangerous command (matched '%s' pattern) — do not retry",
				reason),
		}
	}

	// No approval handler — hard block (legacy behavior).
	// 规范九：只 return 结果，不 log（调用方负责）
	return map[string]any{
		"approved": false,
		"status":   "blocked",
		"command":  command,
		"reason":   reason,
		"message": fmt.Sprintf(
			"command flagged as dangerous (%s) and blocked — use /approve to allow",
			reason),
	}
}
