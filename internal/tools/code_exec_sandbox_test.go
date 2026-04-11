package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- LimitedWriter tests ---

func TestLimitedWriter_UnderLimit(t *testing.T) {
	lw := NewLimitedWriter(100)
	n, err := lw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected n=5, got %d", n)
	}
	if lw.String() != "hello" {
		t.Errorf("expected 'hello', got %q", lw.String())
	}
	if lw.Exceeded() {
		t.Error("should not have exceeded limit")
	}
}

func TestLimitedWriter_ExactLimit(t *testing.T) {
	lw := NewLimitedWriter(5)
	lw.Write([]byte("abcde"))
	if lw.Exceeded() {
		t.Error("should not be exceeded at exactly the limit")
	}
	if lw.String() != "abcde" {
		t.Errorf("expected 'abcde', got %q", lw.String())
	}
}

func TestLimitedWriter_Truncation(t *testing.T) {
	lw := NewLimitedWriter(10)
	// Write more than 10 bytes
	n, err := lw.Write([]byte("0123456789extra"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 15 {
		t.Errorf("expected n=15 (all accepted), got %d", n)
	}
	if !lw.Exceeded() {
		t.Error("should have exceeded limit")
	}
	if lw.Len() != 10 {
		t.Errorf("expected Len()=10, got %d", lw.Len())
	}
	out := lw.String()
	if !strings.HasPrefix(out, "0123456789") {
		t.Errorf("expected prefix '0123456789', got %q", out)
	}
	if !strings.Contains(out, "[output truncated at 0KB]") {
		t.Errorf("expected truncation notice, got %q", out)
	}
}

func TestLimitedWriter_MultipleWrites(t *testing.T) {
	lw := NewLimitedWriter(10)
	lw.Write([]byte("12345"))
	lw.Write([]byte("67890"))
	// Exactly at limit
	if lw.Exceeded() {
		t.Error("should not be exceeded yet")
	}
	// One more byte pushes over
	lw.Write([]byte("X"))
	if !lw.Exceeded() {
		t.Error("should be exceeded now")
	}
	if lw.Len() != 10 {
		t.Errorf("expected Len()=10, got %d", lw.Len())
	}
}

func TestLimitedWriter_LargeLimit(t *testing.T) {
	lw := NewLimitedWriter(50 * 1024) // 50 KB
	data := strings.Repeat("A", 50*1024)
	lw.Write([]byte(data))
	if lw.Exceeded() {
		t.Error("should not exceed at exactly 50KB")
	}
	lw.Write([]byte("B"))
	if !lw.Exceeded() {
		t.Error("should exceed after 50KB+1")
	}
	out := lw.String()
	if !strings.Contains(out, "[output truncated at 50KB]") {
		t.Errorf("expected 50KB truncation notice, got suffix %q", out[len(out)-60:])
	}
}

// --- SandboxConfig tests ---

func TestDefaultSandboxConfig(t *testing.T) {
	cfg := DefaultSandboxConfig()

	if cfg.MaxStdoutBytes != 50*1024 {
		t.Errorf("expected MaxStdoutBytes=51200, got %d", cfg.MaxStdoutBytes)
	}
	if cfg.MaxStderrBytes != 10*1024 {
		t.Errorf("expected MaxStderrBytes=10240, got %d", cfg.MaxStderrBytes)
	}
	if cfg.MaxToolCalls != 50 {
		t.Errorf("expected MaxToolCalls=50, got %d", cfg.MaxToolCalls)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected Timeout=30s, got %v", cfg.Timeout)
	}
	if cfg.RestrictNetwork {
		t.Error("expected RestrictNetwork=false")
	}
	if len(cfg.AllowedTools) != len(DefaultAllowedTools) {
		t.Errorf("expected %d allowed tools, got %d", len(DefaultAllowedTools), len(cfg.AllowedTools))
	}
}

func TestDefaultSandboxConfig_IndependentCopy(t *testing.T) {
	cfg1 := DefaultSandboxConfig()
	cfg2 := DefaultSandboxConfig()
	cfg1.AllowedTools = append(cfg1.AllowedTools, "extra_tool")
	if len(cfg2.AllowedTools) == len(cfg1.AllowedTools) {
		t.Error("modifying one config's AllowedTools should not affect another")
	}
}

// --- Allowlist tests ---

func TestIsToolAllowed(t *testing.T) {
	cfg := DefaultSandboxConfig()

	for _, tool := range DefaultAllowedTools {
		if !cfg.IsToolAllowed(tool) {
			t.Errorf("expected %q to be allowed", tool)
		}
	}

	if cfg.IsToolAllowed("dangerous_tool") {
		t.Error("expected 'dangerous_tool' to be rejected")
	}
	if cfg.IsToolAllowed("") {
		t.Error("expected empty string to be rejected")
	}
}

func TestIsToolAllowed_CustomList(t *testing.T) {
	cfg := SandboxConfig{
		AllowedTools: []string{"only_this"},
	}
	if !cfg.IsToolAllowed("only_this") {
		t.Error("expected 'only_this' to be allowed")
	}
	if cfg.IsToolAllowed("read_file") {
		t.Error("expected 'read_file' to be rejected with custom list")
	}
}

func TestIsToolAllowed_EmptyList(t *testing.T) {
	cfg := SandboxConfig{
		AllowedTools: []string{},
	}
	if cfg.IsToolAllowed("read_file") {
		t.Error("expected all tools rejected with empty allowlist")
	}
}

// --- RPC tests ---

func TestProcessToolCallRequest_NoRequest(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultSandboxConfig()
	metrics := ExecMetrics{}

	processed := ProcessToolCallRequest(dir, &cfg, &metrics)
	if processed {
		t.Error("expected false when no request.json exists")
	}
}

func TestProcessToolCallRequest_AllowlistReject(t *testing.T) {
	dir := t.TempDir()
	cfg := SandboxConfig{
		AllowedTools: []string{"read_file"},
		MaxToolCalls: 50,
	}
	metrics := ExecMetrics{}

	req := ToolCallRequest{ToolName: "dangerous_tool", Args: map[string]any{}}
	data, _ := json.Marshal(req)
	os.WriteFile(filepath.Join(dir, "request.json"), data, 0644)

	processed := ProcessToolCallRequest(dir, &cfg, &metrics)
	if !processed {
		t.Fatal("expected request to be processed")
	}

	respData, err := os.ReadFile(filepath.Join(dir, "response.json"))
	if err != nil {
		t.Fatalf("expected response.json: %v", err)
	}

	var resp ToolCallResponse
	json.Unmarshal(respData, &resp)

	if resp.Error == "" {
		t.Error("expected error in response for disallowed tool")
	}
	if !strings.Contains(resp.Error, "not in sandbox allowlist") {
		t.Errorf("expected allowlist error, got: %s", resp.Error)
	}
	if metrics.ToolCallCount != 1 {
		t.Errorf("expected ToolCallCount=1, got %d", metrics.ToolCallCount)
	}
}

func TestProcessToolCallRequest_MaxToolCallsExceeded(t *testing.T) {
	dir := t.TempDir()
	cfg := SandboxConfig{
		AllowedTools: []string{"read_file"},
		MaxToolCalls: 2,
	}
	metrics := ExecMetrics{ToolCallCount: 2} // Already at limit

	req := ToolCallRequest{ToolName: "read_file", Args: map[string]any{}}
	data, _ := json.Marshal(req)
	os.WriteFile(filepath.Join(dir, "request.json"), data, 0644)

	processed := ProcessToolCallRequest(dir, &cfg, &metrics)
	if !processed {
		t.Fatal("expected request to be processed")
	}

	respData, _ := os.ReadFile(filepath.Join(dir, "response.json"))
	var resp ToolCallResponse
	json.Unmarshal(respData, &resp)

	if !strings.Contains(resp.Error, "max tool calls") {
		t.Errorf("expected max tool calls error, got: %s", resp.Error)
	}
}

func TestProcessToolCallRequest_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultSandboxConfig()
	metrics := ExecMetrics{}

	os.WriteFile(filepath.Join(dir, "request.json"), []byte("{bad json"), 0644)

	processed := ProcessToolCallRequest(dir, &cfg, &metrics)
	if !processed {
		t.Fatal("expected request to be processed")
	}

	respData, _ := os.ReadFile(filepath.Join(dir, "response.json"))
	var resp ToolCallResponse
	json.Unmarshal(respData, &resp)

	if !strings.Contains(resp.Error, "malformed request") {
		t.Errorf("expected malformed request error, got: %s", resp.Error)
	}
}

func TestSetupAndCleanupRPCDir(t *testing.T) {
	dir, err := SetupRPCDir("test-session-123")
	if err != nil {
		t.Fatalf("SetupRPCDir failed: %v", err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected RPC directory to exist")
	}

	CleanupRPCDir(dir)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected RPC directory to be removed")
	}
}

// --- ExecMetrics tests ---

func TestExecMetrics_JSONRoundTrip(t *testing.T) {
	m := ExecMetrics{
		WallTimeMs:    1234,
		ExitCode:      0,
		StdoutBytes:   500,
		StderrBytes:   100,
		ToolCallCount: 3,
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var m2 ExecMetrics
	if err := json.Unmarshal(data, &m2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if m != m2 {
		t.Errorf("round-trip mismatch: %+v != %+v", m, m2)
	}
}
