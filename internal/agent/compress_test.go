package agent

import (
	"testing"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

func TestIsKeyMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  llm.Message
		want bool
	}{
		{"user correction", llm.Message{Role: "user", Content: "No, use the other file"}, true},
		{"user decision", llm.Message{Role: "user", Content: "Decision: we go with plan B"}, true},
		{"user remember", llm.Message{Role: "user", Content: "Remember: always use UTC"}, true},
		{"user short msg", llm.Message{Role: "user", Content: "yes"}, true},
		{"user long boring", llm.Message{Role: "user", Content: string(make([]byte, 200))}, false},
		{"tool message", llm.Message{Role: "tool", Content: "No, this is wrong"}, false},
		{"system message", llm.Message{Role: "system", Content: "important: do X"}, false},
		{"assistant with stop", llm.Message{Role: "assistant", Content: "Stop doing that approach"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isKeyMessage(tt.msg)
			if got != tt.want {
				t.Errorf("isKeyMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompressSlidingWindow(t *testing.T) {
	messages := make([]llm.Message, 10)
	for i := range messages {
		messages[i] = llm.Message{
			Role:    "user",
			Content: "message " + string(rune('A'+i)),
		}
	}

	cfg := CompressionConfig{KeepCount: 4}
	result := compressSlidingWindow(messages, cfg)

	// Should have 1 system note + 4 kept messages = 5
	if len(result) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(result))
	}

	// First message should be the context note
	if result[0].Role != "system" {
		t.Errorf("first message should be system, got %s", result[0].Role)
	}

	// Last 4 should be the original last 4
	for i := 1; i < 5; i++ {
		if result[i].Content != messages[6+i-1].Content {
			t.Errorf("result[%d] = %q, want %q", i, result[i].Content, messages[6+i-1].Content)
		}
	}
}

func TestCompressSlidingWindow_FewerThanKeep(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
	}

	cfg := CompressionConfig{KeepCount: 10}
	result := compressSlidingWindow(messages, cfg)

	// Should keep all + 1 system note
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
}

func TestEstimateConversationTokens(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "hello world"},
		{Role: "assistant", Content: "hi there"},
	}

	tokens := estimateConversationTokens(messages, "you are helpful")
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
}

func TestDefaultCompressionConfig(t *testing.T) {
	cfg := DefaultCompressionConfig()

	if cfg.Threshold != 0.75 {
		t.Errorf("threshold = %v, want 0.75", cfg.Threshold)
	}
	if cfg.Strategy != StrategyHybrid {
		t.Errorf("strategy = %v, want hybrid", cfg.Strategy)
	}
	if cfg.KeepCount != 6 {
		t.Errorf("keep_count = %d, want 6", cfg.KeepCount)
	}
	if cfg.SummaryMaxWords != 500 {
		t.Errorf("summary_max_words = %d, want 500", cfg.SummaryMaxWords)
	}
}
