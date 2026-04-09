package tools

func init() {
	rlTools := []struct {
		name        string
		description string
		params      map[string]any
	}{
		{
			name:        "rl_list_environments",
			description: "List available RL training environments from Tinker-Atropos. Returns names, descriptions, and configuration options for each environment.",
			params: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			name:        "rl_select_environment",
			description: "Select an RL environment for training. Must be called before starting a training run.",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"environment": map[string]any{
						"type":        "string",
						"description": "Name of the RL environment to select",
					},
				},
				"required": []string{"environment"},
			},
		},
		{
			name:        "rl_get_current_config",
			description: "Get the current training configuration including hyperparameters, environment settings, and model configuration.",
			params: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			name:        "rl_edit_config",
			description: "Edit RL training configuration. Modify hyperparameters such as learning rate, batch size, episodes, and reward settings.",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{
						"type":        "string",
						"description": "Configuration key to modify (e.g., 'learning_rate', 'batch_size', 'episodes')",
					},
					"value": map[string]any{
						"description": "New value for the configuration key",
					},
				},
				"required": []string{"key", "value"},
			},
		},
		{
			name:        "rl_start_training",
			description: "Start an RL training run with the current configuration. Returns a run ID for tracking.",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_name": map[string]any{
						"type":        "string",
						"description": "Optional name for this training run",
					},
				},
			},
		},
		{
			name:        "rl_check_status",
			description: "Check the status of a training run including progress, current episode, reward metrics, and estimated time remaining.",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_id": map[string]any{
						"type":        "string",
						"description": "Training run ID to check",
					},
				},
				"required": []string{"run_id"},
			},
		},
		{
			name:        "rl_stop_training",
			description: "Stop a running training session. The model checkpoint from the last completed episode is preserved.",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_id": map[string]any{
						"type":        "string",
						"description": "Training run ID to stop",
					},
				},
				"required": []string{"run_id"},
			},
		},
		{
			name:        "rl_get_results",
			description: "Get detailed results of a completed training run including reward curves, loss metrics, and evaluation scores.",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_id": map[string]any{
						"type":        "string",
						"description": "Training run ID to get results for",
					},
				},
				"required": []string{"run_id"},
			},
		},
		{
			name:        "rl_list_runs",
			description: "List all training runs with their status, environment, start time, and summary metrics.",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{
						"type":        "string",
						"description": "Filter by status: running, completed, failed, stopped",
						"enum":        []string{"running", "completed", "failed", "stopped", "all"},
					},
				},
			},
		},
		{
			name:        "rl_test_inference",
			description: "Run inference with a trained model checkpoint on test inputs to evaluate learned behavior.",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_id": map[string]any{
						"type":        "string",
						"description": "Training run ID whose checkpoint to use",
					},
					"test_input": map[string]any{
						"type":        "string",
						"description": "Test input to run through the trained model",
					},
				},
				"required": []string{"run_id", "test_input"},
			},
		},
	}

	for _, t := range rlTools {
		toolName := t.name
		toolDesc := t.description

		Register(&ToolEntry{
			Name:    toolName,
			Toolset: "rl",
			Schema: map[string]any{
				"name":        toolName,
				"description": toolDesc,
				"parameters":  t.params,
			},
			Handler: makeRLStubHandler(toolName),
			Emoji:   "\U0001f916",
		})
	}
}

// makeRLStubHandler returns a handler that explains the RL framework is not installed.
func makeRLStubHandler(toolName string) ToolHandler {
	return func(args map[string]any, ctx *ToolContext) string {
		return toJSON(map[string]any{
			"error": "Tinker-Atropos RL framework is not installed",
			"tool":  toolName,
			"hint":  "The RL training tools require the Tinker-Atropos framework. Install it with: pip install tinker-atropos",
			"docs":  "https://github.com/NousResearch/tinker-atropos",
			"setup": map[string]any{
				"step_1": "Install Python 3.10+ and pip",
				"step_2": "pip install tinker-atropos",
				"step_3": "Configure the RL environment in ~/.hermes/config.yaml under 'rl_training' section",
				"step_4": "Restart Hermes to enable RL tools",
			},
		})
	}
}
