package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ModelDevInfo holds metadata for a model fetched from models.dev.
type ModelDevInfo struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Provider      string  `json:"provider"`
	ContextLength int     `json:"context_length"`
	MaxOutput     int     `json:"max_output"`
	InputPrice    float64 `json:"input_price"`  // USD per million tokens
	OutputPrice   float64 `json:"output_price"` // USD per million tokens
}

const modelsDevBaseURL = "https://models.dev/api/models"
const modelsDevTimeout = 10 * time.Second

var (
	modelDevCache   = make(map[string]*ModelDevInfo)
	modelDevCacheMu sync.RWMutex
)

// FetchModelInfo queries models.dev for model metadata.
// Results are cached in memory for the lifetime of the process.
//
// The modelID should be in "provider/model-name" format, e.g.
// "anthropic/claude-sonnet-4-20250514".
func FetchModelInfo(modelID string) (*ModelDevInfo, error) {
	// Check cache first.
	modelDevCacheMu.RLock()
	if info, ok := modelDevCache[modelID]; ok {
		modelDevCacheMu.RUnlock()
		return info, nil
	}
	modelDevCacheMu.RUnlock()

	// Build the API URL.
	apiURL := modelsDevBaseURL
	if strings.Contains(modelID, "/") {
		parts := strings.SplitN(modelID, "/", 2)
		apiURL = fmt.Sprintf("%s/%s/%s", modelsDevBaseURL, url.PathEscape(parts[0]), url.PathEscape(parts[1]))
	} else {
		apiURL = fmt.Sprintf("%s/%s", modelsDevBaseURL, url.PathEscape(modelID))
	}

	client := &http.Client{Timeout: modelsDevTimeout}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("models.dev request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned status %d for %s", resp.StatusCode, modelID)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read models.dev response: %w", err)
	}

	// The API can return different shapes; try parsing as a single model object.
	var info ModelDevInfo
	if err := json.Unmarshal(body, &info); err != nil {
		// Try parsing as a wrapper with a "data" field.
		var wrapper struct {
			Data ModelDevInfo `json:"data"`
		}
		if err2 := json.Unmarshal(body, &wrapper); err2 != nil {
			return nil, fmt.Errorf("parse models.dev response: %w", err)
		}
		info = wrapper.Data
	}

	// Populate ID if empty.
	if info.ID == "" {
		info.ID = modelID
	}

	// Cache the result.
	modelDevCacheMu.Lock()
	modelDevCache[modelID] = &info
	modelDevCacheMu.Unlock()

	return &info, nil
}

// LookupModelInfo first checks the local cache, then falls back to the
// built-in pricing table, and finally queries models.dev. Returns nil if
// the model cannot be resolved from any source.
func LookupModelInfo(modelID string) *ModelDevInfo {
	// Check models.dev cache.
	modelDevCacheMu.RLock()
	if info, ok := modelDevCache[modelID]; ok {
		modelDevCacheMu.RUnlock()
		return info
	}
	modelDevCacheMu.RUnlock()

	// Check built-in pricing.
	if p, ok := knownPricing[modelID]; ok {
		return &ModelDevInfo{
			ID:          modelID,
			Name:        modelID,
			InputPrice:  p.InputPerMillion,
			OutputPrice: p.OutputPerMillion,
		}
	}

	// Try fetching from models.dev (best effort, do not block on failure).
	info, err := FetchModelInfo(modelID)
	if err != nil {
		return nil
	}
	return info
}

// ClearModelDevCache empties the in-process models.dev cache.
func ClearModelDevCache() {
	modelDevCacheMu.Lock()
	modelDevCache = make(map[string]*ModelDevInfo)
	modelDevCacheMu.Unlock()
}
