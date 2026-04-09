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
