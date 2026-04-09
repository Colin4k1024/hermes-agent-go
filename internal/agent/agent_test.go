package agent

import (
	"os"
	"testing"
)

func TestAgentOptions(t *testing.T) {
	a := &AIAgent{}

	WithModel("test-model")(a)
	if a.model != "test-model" {
		t.Errorf("Expected model 'test-model', got '%s'", a.model)
	}

	WithMaxIterations(50)(a)
	if a.maxIterations != 50 {
		t.Errorf("Expected 50 iterations, got %d", a.maxIterations)
	}

	WithPlatform("telegram")(a)
	if a.platform != "telegram" {
		t.Errorf("Expected platform 'telegram', got '%s'", a.platform)
	}

	WithSessionID("sess-123")(a)
	if a.sessionID != "sess-123" {
		t.Errorf("Expected session 'sess-123', got '%s'", a.sessionID)
	}

	WithQuietMode(true)(a)
	if !a.quietMode {
		t.Error("Expected quiet mode on")
	}

	WithBaseURL("https://api.example.com")(a)
	if a.baseURL != "https://api.example.com" {
		t.Errorf("Expected base URL, got '%s'", a.baseURL)
	}

	WithAPIKey("sk-test")(a)
	if a.apiKey != "sk-test" {
		t.Errorf("Expected API key, got '%s'", a.apiKey)
	}

	WithProvider("openai")(a)
	if a.provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", a.provider)
	}

	WithAPIMode("anthropic")(a)
	if a.apiMode != "anthropic" {
		t.Errorf("Expected apiMode 'anthropic', got '%s'", a.apiMode)
	}

	WithSkipContextFiles(true)(a)
	if !a.skipContextFiles {
		t.Error("Expected skipContextFiles true")
	}

	WithSkipMemory(true)(a)
	if !a.skipMemory {
		t.Error("Expected skipMemory true")
	}

	WithPersistSession(false)(a)
	if a.persistSession {
		t.Error("Expected persistSession false")
	}

	WithEnabledToolsets([]string{"web", "terminal"})(a)
	if len(a.enabledToolsets) != 2 {
		t.Errorf("Expected 2 enabled toolsets, got %d", len(a.enabledToolsets))
	}

	WithDisabledToolsets([]string{"browser"})(a)
	if len(a.disabledToolsets) != 1 {
		t.Errorf("Expected 1 disabled toolset, got %d", len(a.disabledToolsets))
	}

	WithSystemPrompt("Custom prompt")(a)
	if a.ephemeralSystemPrompt != "Custom prompt" {
		t.Errorf("Expected custom prompt, got '%s'", a.ephemeralSystemPrompt)
	}
}

func TestStreamCallbacksFiring(t *testing.T) {
	var deltaReceived string
	var toolStartReceived string
	var stepReceived int

	a := &AIAgent{
		callbacks: &StreamCallbacks{
			OnStreamDelta: func(text string) { deltaReceived = text },
			OnToolStart:   func(name string) { toolStartReceived = name },
			OnStep:        func(i int, _ []string) { stepReceived = i },
		},
	}

	a.fireStreamDelta("hello")
	if deltaReceived != "hello" {
		t.Errorf("Expected 'hello', got '%s'", deltaReceived)
	}

	a.fireToolStart("terminal")
	if toolStartReceived != "terminal" {
		t.Errorf("Expected 'terminal', got '%s'", toolStartReceived)
	}

	a.fireStep(5, nil)
	if stepReceived != 5 {
		t.Errorf("Expected step 5, got %d", stepReceived)
	}
}

func TestStreamCallbacksNil(t *testing.T) {
	a := &AIAgent{}

	// Should not panic with nil callbacks
	a.fireStreamDelta("test")
	a.fireReasoning("test")
	a.fireToolGenStarted("test")
	a.fireToolProgress("test", "args")
	a.fireToolStart("test")
	a.fireToolComplete("test")
	a.fireStep(1, nil)
	a.fireStatus("test")
}

func TestHasStreamConsumers(t *testing.T) {
	a := &AIAgent{}
	if a.hasStreamConsumers() {
		t.Error("Expected false with nil callbacks")
	}

	a.callbacks = &StreamCallbacks{}
	if a.hasStreamConsumers() {
		t.Error("Expected false with empty callbacks")
	}

	a.callbacks = &StreamCallbacks{OnStreamDelta: func(s string) {}}
	if !a.hasStreamConsumers() {
		t.Error("Expected true with OnStreamDelta set")
	}
}

func TestInterrupt(t *testing.T) {
	a := &AIAgent{}
	if a.isInterrupted() {
		t.Error("Expected not interrupted initially")
	}

	a.Interrupt()
	if !a.isInterrupted() {
		t.Error("Expected interrupted after Interrupt()")
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("Short string should not be truncated")
	}
	result := truncate("hello world this is long", 10)
	if len(result) > 14 { // 10 + "..."
		t.Errorf("Expected truncated result, got '%s'", result)
	}
}

func TestBuildSystemPromptWithOverride(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	a := &AIAgent{
		ephemeralSystemPrompt: "Custom system prompt override",
		platform:              "cli",
	}
	prompt := a.buildSystemPrompt()
	if prompt != "Custom system prompt override" {
		t.Errorf("Expected override prompt, got '%s'", prompt)
	}
}

func TestBuildSystemPromptDefault(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")
	os.MkdirAll(tmpDir+"/skills", 0755)

	a := &AIAgent{
		platform: "cli",
	}
	prompt := a.buildSystemPrompt()
	if prompt == "" {
		t.Error("Expected non-empty default prompt")
	}
	if len(prompt) < 100 {
		t.Errorf("Default prompt too short (%d chars)", len(prompt))
	}
}
