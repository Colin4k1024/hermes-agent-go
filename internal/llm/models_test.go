package llm

import "testing"

func TestGetModelMeta(t *testing.T) {
	meta := GetModelMeta("anthropic/claude-opus-4-20250514")
	if meta.ContextLength != 200000 {
		t.Errorf("Expected 200000 context, got %d", meta.ContextLength)
	}
	if !meta.SupportsTools {
		t.Error("Expected tools support")
	}
	if !meta.SupportsVision {
		t.Error("Expected vision support")
	}
}

func TestGetModelMetaUnknown(t *testing.T) {
	meta := GetModelMeta("unknown/model-xyz")
	if meta.ContextLength != 128000 {
		t.Errorf("Expected default 128000, got %d", meta.ContextLength)
	}
	if !meta.SupportsTools {
		t.Error("Expected default tools support")
	}
}

func TestEstimateTokens(t *testing.T) {
	tokens := EstimateTokens("Hello, world!")
	if tokens <= 0 {
		t.Error("Expected positive token count")
	}
	// ~13 chars / 4 = ~3 tokens
	if tokens > 10 {
		t.Errorf("Token estimate too high for short string: %d", tokens)
	}

	longText := ""
	for i := 0; i < 1000; i++ {
		longText += "word "
	}
	tokens = EstimateTokens(longText)
	if tokens < 500 {
		t.Errorf("Token estimate too low for long text: %d", tokens)
	}
}

func TestGetModelMeta_AllKnownModels(t *testing.T) {
	for name, expected := range KnownModels {
		meta := GetModelMeta(name)
		if meta.ContextLength != expected.ContextLength {
			t.Errorf("Model %s: expected context %d, got %d", name, expected.ContextLength, meta.ContextLength)
		}
		if meta.MaxOutput != expected.MaxOutput {
			t.Errorf("Model %s: expected max output %d, got %d", name, expected.MaxOutput, meta.MaxOutput)
		}
		if meta.SupportsTools != expected.SupportsTools {
			t.Errorf("Model %s: tools support mismatch", name)
		}
		if meta.SupportsVision != expected.SupportsVision {
			t.Errorf("Model %s: vision support mismatch", name)
		}
	}
}

func TestGetModelMeta_SpecificModels(t *testing.T) {
	tests := []struct {
		model          string
		context        int
		supportsVision bool
	}{
		{"openai/gpt-4o", 128000, true},
		{"openai/gpt-4o-mini", 128000, true},
		{"google/gemini-2.5-pro", 1048576, true},
		{"deepseek/deepseek-chat", 65536, false},
		{"meta-llama/llama-4-maverick", 1048576, true},
	}

	for _, tt := range tests {
		meta := GetModelMeta(tt.model)
		if meta.ContextLength != tt.context {
			t.Errorf("Model %s: expected context %d, got %d", tt.model, tt.context, meta.ContextLength)
		}
		if meta.SupportsVision != tt.supportsVision {
			t.Errorf("Model %s: expected vision=%v, got %v", tt.model, tt.supportsVision, meta.SupportsVision)
		}
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	tokens := EstimateTokens("")
	if tokens != 0 {
		t.Errorf("Expected 0 tokens for empty string, got %d", tokens)
	}
}

func TestEstimateTokens_Short(t *testing.T) {
	// "ab" = 2 chars / 4 = 0 tokens (integer division)
	tokens := EstimateTokens("ab")
	if tokens != 0 {
		t.Errorf("Expected 0 tokens for 2-char string, got %d", tokens)
	}
}

func TestKnownModels_NonEmpty(t *testing.T) {
	if len(KnownModels) == 0 {
		t.Error("KnownModels should not be empty")
	}
	if len(KnownModels) < 10 {
		t.Errorf("Expected at least 10 known models, got %d", len(KnownModels))
	}
}
