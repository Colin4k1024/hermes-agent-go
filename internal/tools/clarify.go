package tools

func init() {
	Register(&ToolEntry{
		Name:    "clarify",
		Toolset: "clarify",
		Schema: map[string]any{
			"name":        "clarify",
			"description": "Ask the user a clarifying question when you need more information to proceed. Optionally provide multiple-choice options.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": map[string]any{
						"type":        "string",
						"description": "The clarifying question to ask the user",
					},
					"choices": map[string]any{
						"type":        "array",
						"description": "Optional list of choices for the user to pick from",
						"items":       map[string]any{"type": "string"},
					},
					"context": map[string]any{
						"type":        "string",
						"description": "Optional context explaining why you need this clarification",
					},
				},
				"required": []string{"question"},
			},
		},
		Handler: handleClarify,
		Emoji:   "\u2753",
	})
}

func handleClarify(args map[string]any, ctx *ToolContext) string {
	question, _ := args["question"].(string)
	if question == "" {
		return `{"error":"question is required"}`
	}

	result := map[string]any{
		"type":     "clarification",
		"question": question,
	}

	if choicesRaw, ok := args["choices"].([]any); ok && len(choicesRaw) > 0 {
		var choices []string
		for _, c := range choicesRaw {
			if s, ok := c.(string); ok {
				choices = append(choices, s)
			}
		}
		result["choices"] = choices
	}

	if context, ok := args["context"].(string); ok && context != "" {
		result["context"] = context
	}

	return toJSON(result)
}
