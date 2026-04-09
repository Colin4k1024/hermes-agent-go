package tools

import (
	"os"
)

func init() {
	Register(&ToolEntry{
		Name:    "browser_navigate",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_navigate",
			"description": "Navigate the browser to a URL. Opens a new page or navigates the current one.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "URL to navigate to",
					},
				},
				"required": []string{"url"},
			},
		},
		Handler:     handleBrowserNavigate,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\U0001f310",
	})

	Register(&ToolEntry{
		Name:    "browser_snapshot",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_snapshot",
			"description": "Take a snapshot (screenshot) of the current browser page. Returns accessibility tree and screenshot.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler:     handleBrowserSnapshot,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\U0001f4f8",
	})

	Register(&ToolEntry{
		Name:    "browser_click",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_click",
			"description": "Click on an element in the browser page by its reference ID from the accessibility tree.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ref": map[string]any{
						"type":        "string",
						"description": "Reference ID of the element to click",
					},
				},
				"required": []string{"ref"},
			},
		},
		Handler:     handleBrowserClick,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\U0001f5b1\ufe0f",
	})

	Register(&ToolEntry{
		Name:    "browser_type",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_type",
			"description": "Type text into an input element in the browser page.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ref": map[string]any{
						"type":        "string",
						"description": "Reference ID of the input element",
					},
					"text": map[string]any{
						"type":        "string",
						"description": "Text to type",
					},
					"clear_first": map[string]any{
						"type":        "boolean",
						"description": "Clear existing text before typing",
						"default":     false,
					},
				},
				"required": []string{"ref", "text"},
			},
		},
		Handler:     handleBrowserType,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\u2328\ufe0f",
	})

	Register(&ToolEntry{
		Name:    "browser_scroll",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_scroll",
			"description": "Scroll the browser page in a direction.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"direction": map[string]any{
						"type":        "string",
						"description": "Scroll direction",
						"enum":        []string{"up", "down", "left", "right"},
					},
					"amount": map[string]any{
						"type":        "integer",
						"description": "Scroll amount in pixels (default: 500)",
						"default":     500,
					},
				},
				"required": []string{"direction"},
			},
		},
		Handler:     handleBrowserScroll,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\U0001f4dc",
	})

	Register(&ToolEntry{
		Name:    "browser_back",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_back",
			"description": "Navigate back in browser history.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler:     handleBrowserBack,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\u2b05\ufe0f",
	})

	Register(&ToolEntry{
		Name:    "browser_press",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_press",
			"description": "Press a keyboard key in the browser (e.g., Enter, Escape, Tab).",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{
						"type":        "string",
						"description": "Key to press (e.g., 'Enter', 'Escape', 'Tab', 'ArrowDown')",
					},
				},
				"required": []string{"key"},
			},
		},
		Handler:     handleBrowserPress,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\u2328\ufe0f",
	})

	Register(&ToolEntry{
		Name:    "browser_get_images",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_get_images",
			"description": "Get a list of images on the current browser page with their URLs and alt text.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler:     handleBrowserGetImages,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\U0001f5bc\ufe0f",
	})

	Register(&ToolEntry{
		Name:    "browser_vision",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_vision",
			"description": "Analyze the current browser page visually using a multimodal LLM. Takes a screenshot and describes what is visible.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "What to analyze about the page",
						"default":     "Describe what you see on this page.",
					},
				},
			},
		},
		Handler:     handleBrowserVision,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\U0001f441\ufe0f",
	})

	Register(&ToolEntry{
		Name:    "browser_console",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_console",
			"description": "Execute JavaScript in the browser console and return the result.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"script": map[string]any{
						"type":        "string",
						"description": "JavaScript code to execute",
					},
				},
				"required": []string{"script"},
			},
		},
		Handler:     handleBrowserConsole,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\U0001f4bb",
	})

	Register(&ToolEntry{
		Name:    "browser_close",
		Toolset: "browser",
		Schema: map[string]any{
			"name":        "browser_close",
			"description": "Close the current browser session and release resources.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler:     handleBrowserClose,
		CheckFn:     checkBrowserRequirements,
		RequiresEnv: []string{"BROWSERBASE_API_KEY"},
		Emoji:       "\u274c",
	})
}

func checkBrowserRequirements() bool {
	return os.Getenv("BROWSERBASE_API_KEY") != ""
}

func browserStubResponse(tool string) string {
	return toJSON(map[string]any{
		"error":   "Browser tool requires Browserbase API key",
		"tool":    tool,
		"hint":    "Set BROWSERBASE_API_KEY environment variable to enable browser tools",
		"docs":    "https://www.browserbase.com/docs",
	})
}

func handleBrowserNavigate(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_navigate")
}

func handleBrowserSnapshot(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_snapshot")
}

func handleBrowserClick(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_click")
}

func handleBrowserType(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_type")
}

func handleBrowserScroll(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_scroll")
}

func handleBrowserBack(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_back")
}

func handleBrowserPress(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_press")
}

func handleBrowserGetImages(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_get_images")
}

func handleBrowserVision(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_vision")
}

func handleBrowserConsole(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_console")
}

func handleBrowserClose(args map[string]any, ctx *ToolContext) string {
	return browserStubResponse("browser_close")
}
