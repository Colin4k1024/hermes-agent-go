package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func init() {
	Register(&ToolEntry{
		Name:    "ha_list_entities",
		Toolset: "homeassistant",
		Schema: map[string]any{
			"name":        "ha_list_entities",
			"description": "List all entities in Home Assistant, optionally filtered by domain (e.g., 'light', 'switch', 'sensor').",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"domain": map[string]any{
						"type":        "string",
						"description": "Entity domain filter (e.g., 'light', 'switch', 'sensor', 'climate')",
					},
				},
			},
		},
		Handler:     handleHAListEntities,
		CheckFn:     checkHARequirements,
		RequiresEnv: []string{"HASS_URL", "HASS_TOKEN"},
		Emoji:       "\U0001f3e0",
	})

	Register(&ToolEntry{
		Name:    "ha_get_state",
		Toolset: "homeassistant",
		Schema: map[string]any{
			"name":        "ha_get_state",
			"description": "Get the current state and attributes of a Home Assistant entity.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity_id": map[string]any{
						"type":        "string",
						"description": "Entity ID (e.g., 'light.living_room', 'sensor.temperature')",
					},
				},
				"required": []string{"entity_id"},
			},
		},
		Handler:     handleHAGetState,
		CheckFn:     checkHARequirements,
		RequiresEnv: []string{"HASS_URL", "HASS_TOKEN"},
		Emoji:       "\U0001f4ca",
	})

	Register(&ToolEntry{
		Name:    "ha_list_services",
		Toolset: "homeassistant",
		Schema: map[string]any{
			"name":        "ha_list_services",
			"description": "List available services in Home Assistant, optionally filtered by domain.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"domain": map[string]any{
						"type":        "string",
						"description": "Service domain filter (e.g., 'light', 'switch')",
					},
				},
			},
		},
		Handler:     handleHAListServices,
		CheckFn:     checkHARequirements,
		RequiresEnv: []string{"HASS_URL", "HASS_TOKEN"},
		Emoji:       "\U0001f527",
	})

	Register(&ToolEntry{
		Name:    "ha_call_service",
		Toolset: "homeassistant",
		Schema: map[string]any{
			"name":        "ha_call_service",
			"description": "Call a Home Assistant service (e.g., turn on a light, set thermostat temperature).",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"domain": map[string]any{
						"type":        "string",
						"description": "Service domain (e.g., 'light', 'switch', 'climate')",
					},
					"service": map[string]any{
						"type":        "string",
						"description": "Service name (e.g., 'turn_on', 'turn_off', 'set_temperature')",
					},
					"entity_id": map[string]any{
						"type":        "string",
						"description": "Target entity ID",
					},
					"data": map[string]any{
						"type":        "object",
						"description": "Additional service data (e.g., brightness, temperature)",
					},
				},
				"required": []string{"domain", "service"},
			},
		},
		Handler:     handleHACallService,
		CheckFn:     checkHARequirements,
		RequiresEnv: []string{"HASS_URL", "HASS_TOKEN"},
		Emoji:       "\u26a1",
	})
}

func checkHARequirements() bool {
	return os.Getenv("HASS_URL") != "" && os.Getenv("HASS_TOKEN") != ""
}

func haRequest(method, path string, body io.Reader) ([]byte, int, error) {
	hassURL := strings.TrimRight(os.Getenv("HASS_URL"), "/")
	hassToken := os.Getenv("HASS_TOKEN")

	url := hassURL + "/api/" + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+hassToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	return data, resp.StatusCode, nil
}

func handleHAListEntities(args map[string]any, ctx *ToolContext) string {
	domain, _ := args["domain"].(string)

	data, statusCode, err := haRequest("GET", "states", nil)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Home Assistant API error: %v", err)})
	}
	if statusCode != http.StatusOK {
		return toJSON(map[string]any{
			"error":       "Home Assistant API returned error",
			"status_code": statusCode,
			"response":    truncateOutput(string(data), 500),
		})
	}

	var states []map[string]any
	if err := json.Unmarshal(data, &states); err != nil {
		return toJSON(map[string]any{"error": "Failed to parse Home Assistant response"})
	}

	var entities []map[string]any
	for _, state := range states {
		entityID, _ := state["entity_id"].(string)
		if domain != "" && !strings.HasPrefix(entityID, domain+".") {
			continue
		}

		friendlyName := ""
		if attrs, ok := state["attributes"].(map[string]any); ok {
			friendlyName, _ = attrs["friendly_name"].(string)
		}

		entities = append(entities, map[string]any{
			"entity_id":     entityID,
			"state":         state["state"],
			"friendly_name": friendlyName,
		})
	}

	return toJSON(map[string]any{
		"entities": entities,
		"count":    len(entities),
		"domain":   domain,
	})
}

func handleHAGetState(args map[string]any, ctx *ToolContext) string {
	entityID, _ := args["entity_id"].(string)
	if entityID == "" {
		return `{"error":"entity_id is required"}`
	}

	data, statusCode, err := haRequest("GET", "states/"+entityID, nil)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Home Assistant API error: %v", err)})
	}
	if statusCode == http.StatusNotFound {
		return toJSON(map[string]any{"error": fmt.Sprintf("Entity not found: %s", entityID)})
	}
	if statusCode != http.StatusOK {
		return toJSON(map[string]any{
			"error":       "Home Assistant API returned error",
			"status_code": statusCode,
		})
	}

	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return toJSON(map[string]any{"error": "Failed to parse state response"})
	}

	return toJSON(map[string]any{
		"entity_id":  entityID,
		"state":      state["state"],
		"attributes": state["attributes"],
		"last_changed": state["last_changed"],
		"last_updated": state["last_updated"],
	})
}

func handleHAListServices(args map[string]any, ctx *ToolContext) string {
	domain, _ := args["domain"].(string)

	data, statusCode, err := haRequest("GET", "services", nil)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Home Assistant API error: %v", err)})
	}
	if statusCode != http.StatusOK {
		return toJSON(map[string]any{
			"error":       "Home Assistant API returned error",
			"status_code": statusCode,
		})
	}

	var services []map[string]any
	if err := json.Unmarshal(data, &services); err != nil {
		return toJSON(map[string]any{"error": "Failed to parse services response"})
	}

	var filtered []map[string]any
	for _, svc := range services {
		svcDomain, _ := svc["domain"].(string)
		if domain != "" && svcDomain != domain {
			continue
		}

		svcEntry := map[string]any{
			"domain": svcDomain,
		}

		if svcServices, ok := svc["services"].(map[string]any); ok {
			var svcNames []string
			for name := range svcServices {
				svcNames = append(svcNames, name)
			}
			svcEntry["services"] = svcNames
		}

		filtered = append(filtered, svcEntry)
	}

	return toJSON(map[string]any{
		"services": filtered,
		"count":    len(filtered),
	})
}

func handleHACallService(args map[string]any, ctx *ToolContext) string {
	domain, _ := args["domain"].(string)
	service, _ := args["service"].(string)
	entityID, _ := args["entity_id"].(string)

	if domain == "" || service == "" {
		return `{"error":"domain and service are required"}`
	}

	payload := map[string]any{}
	if entityID != "" {
		payload["entity_id"] = entityID
	}

	// Merge additional data
	if data, ok := args["data"].(map[string]any); ok {
		for k, v := range data {
			payload[k] = v
		}
	}

	body, _ := json.Marshal(payload)
	path := fmt.Sprintf("services/%s/%s", domain, service)

	respData, statusCode, err := haRequest("POST", path, strings.NewReader(string(body)))
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Service call failed: %v", err)})
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return toJSON(map[string]any{
			"error":       "Service call returned error",
			"status_code": statusCode,
			"response":    truncateOutput(string(respData), 500),
		})
	}

	return toJSON(map[string]any{
		"success":   true,
		"domain":    domain,
		"service":   service,
		"entity_id": entityID,
		"message":   fmt.Sprintf("Service %s.%s called successfully", domain, service),
	})
}
