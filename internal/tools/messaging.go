package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func init() {
	Register(&ToolEntry{
		Name:    "send_message",
		Toolset: "messaging",
		Schema: map[string]any{
			"name":        "send_message",
			"description": "Send a message to a user or chat on a supported platform via the messaging gateway.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"platform": map[string]any{
						"type":        "string",
						"description": "Target platform",
						"enum":        []string{"telegram", "discord", "slack", "wechat"},
					},
					"chat_id": map[string]any{
						"type":        "string",
						"description": "Chat/channel/user ID on the target platform",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Message content to send",
					},
					"reply_to": map[string]any{
						"type":        "string",
						"description": "Optional message ID to reply to",
					},
				},
				"required": []string{"platform", "chat_id", "content"},
			},
		},
		Handler: handleSendMessage,
		CheckFn: checkMessagingRequirements,
		Emoji:   "\U0001f4e8",
	})
}

func checkMessagingRequirements() bool {
	// Gateway must be running on localhost
	gatewayURL := getGatewayURL()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(gatewayURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func getGatewayURL() string {
	if url := os.Getenv("HERMES_GATEWAY_URL"); url != "" {
		return url
	}
	return "http://localhost:8765"
}

func handleSendMessage(args map[string]any, ctx *ToolContext) string {
	platform, _ := args["platform"].(string)
	chatID, _ := args["chat_id"].(string)
	content, _ := args["content"].(string)
	replyTo, _ := args["reply_to"].(string)

	if platform == "" {
		return `{"error":"platform is required"}`
	}
	if chatID == "" {
		return `{"error":"chat_id is required"}`
	}
	if content == "" {
		return `{"error":"content is required"}`
	}

	gatewayURL := getGatewayURL()

	payload := map[string]any{
		"platform": platform,
		"chat_id":  chatID,
		"content":  content,
	}
	if replyTo != "" {
		payload["reply_to"] = replyTo
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", gatewayURL+"/api/send", bytes.NewReader(body))
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to create request: %v", err)})
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return toJSON(map[string]any{
			"error":   fmt.Sprintf("Gateway request failed: %v", err),
			"hint":    "Ensure the messaging gateway is running",
			"gateway": gatewayURL,
		})
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return toJSON(map[string]any{
			"error":       "Message delivery failed",
			"status_code": resp.StatusCode,
			"response":    truncateOutput(string(respBody), 500),
		})
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		result = map[string]any{"raw": string(respBody)}
	}

	result["success"] = true
	result["platform"] = platform
	result["chat_id"] = chatID
	result["message"] = fmt.Sprintf("Message sent to %s:%s", platform, chatID)

	return toJSON(result)
}
