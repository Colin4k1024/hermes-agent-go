package agent

import "fmt"

// ModelPricing holds per-million-token prices for a model.
type ModelPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// knownPricing maps model identifiers to their USD pricing (per million tokens).
var knownPricing = map[string]ModelPricing{
	// Anthropic
	"anthropic/claude-opus-4-20250514":   {InputPerMillion: 15.0, OutputPerMillion: 75.0},
	"anthropic/claude-sonnet-4-20250514": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	"anthropic/claude-haiku-4-20250414":  {InputPerMillion: 0.80, OutputPerMillion: 4.0},

	// OpenAI
	"openai/gpt-4o":      {InputPerMillion: 2.50, OutputPerMillion: 10.0},
	"openai/gpt-4o-mini": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
	"openai/o1":          {InputPerMillion: 15.0, OutputPerMillion: 60.0},
	"openai/o3":          {InputPerMillion: 10.0, OutputPerMillion: 40.0},

	// Google
	"google/gemini-2.5-pro":   {InputPerMillion: 1.25, OutputPerMillion: 10.0},
	"google/gemini-2.5-flash": {InputPerMillion: 0.15, OutputPerMillion: 0.60},

	// DeepSeek
	"deepseek/deepseek-chat": {InputPerMillion: 0.27, OutputPerMillion: 1.10},
	"deepseek/deepseek-r1":   {InputPerMillion: 0.55, OutputPerMillion: 2.19},

	// Meta
	"meta-llama/llama-4-maverick": {InputPerMillion: 0.20, OutputPerMillion: 0.60},
}

// EstimateCost returns the estimated USD cost for the given token counts.
// Returns 0 if the model is not in the pricing table.
func EstimateCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := knownPricing[model]
	if !ok {
		return 0
	}
	return (float64(inputTokens)/1_000_000)*p.InputPerMillion +
		(float64(outputTokens)/1_000_000)*p.OutputPerMillion
}

// GetPricing returns the pricing entry for a model, and whether it was found.
func GetPricing(model string) (ModelPricing, bool) {
	p, ok := knownPricing[model]
	return p, ok
}

// FormatCost formats a USD cost as a human-readable string.
func FormatCost(costUSD float64) string {
	if costUSD == 0 {
		return ""
	}
	if costUSD < 0.01 {
		return fmt.Sprintf("$%.4f", costUSD)
	}
	return fmt.Sprintf("$%.2f", costUSD)
}
