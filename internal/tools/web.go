package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func init() {
	Register(&ToolEntry{
		Name:    "web_search",
		Toolset: "web",
		Schema: map[string]any{
			"name":        "web_search",
			"description": "Search the web for information. Returns search results with titles, URLs, and snippets.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query",
					},
					"num_results": map[string]any{
						"type":        "integer",
						"description": "Number of results to return (default: 5, max: 20)",
						"default":     5,
					},
				},
				"required": []string{"query"},
			},
		},
		Handler:     handleWebSearch,
		CheckFn:     checkWebRequirements,
		RequiresEnv: []string{"EXA_API_KEY"},
		Emoji:       "🔍",
	})

	Register(&ToolEntry{
		Name:    "web_extract",
		Toolset: "web",
		Schema: map[string]any{
			"name":        "web_extract",
			"description": "Extract and read content from a web page URL. Returns the page content as text.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"urls": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "URLs to extract content from",
					},
					"format": map[string]any{
						"type":        "string",
						"description": "Output format: 'markdown' or 'text'",
						"default":     "markdown",
					},
				},
				"required": []string{"urls"},
			},
		},
		Handler:     handleWebExtract,
		CheckFn:     checkWebRequirements,
		RequiresEnv: []string{"FIRECRAWL_API_KEY"},
		Emoji:       "🌐",
	})

	Register(&ToolEntry{
		Name:    "web_crawl",
		Toolset: "web",
		Schema: map[string]any{
			"name":        "web_crawl",
			"description": "Crawl a website starting from a URL, following links to discover and extract content from multiple pages.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "Starting URL to crawl from",
					},
					"max_pages": map[string]any{
						"type":        "integer",
						"description": "Maximum number of pages to crawl (default: 5, max: 20)",
						"default":     5,
					},
					"format": map[string]any{
						"type":        "string",
						"description": "Output format: 'markdown' or 'text'",
						"default":     "markdown",
						"enum":        []string{"markdown", "text"},
					},
				},
				"required": []string{"url"},
			},
		},
		Handler:     handleWebCrawl,
		CheckFn:     checkFirecrawlRequirements,
		RequiresEnv: []string{"FIRECRAWL_API_KEY"},
		Emoji:       "🕸️",
	})
}

func checkWebRequirements() bool {
	return os.Getenv("EXA_API_KEY") != "" || os.Getenv("FIRECRAWL_API_KEY") != ""
}

func handleWebSearch(args map[string]any, ctx *ToolContext) string {
	query, _ := args["query"].(string)
	if query == "" {
		return `{"error":"query is required"}`
	}

	numResults := 5
	if n, ok := args["num_results"].(float64); ok && n > 0 {
		numResults = int(n)
		if numResults > 20 {
			numResults = 20
		}
	}

	apiKey := os.Getenv("EXA_API_KEY")
	if apiKey == "" {
		// Fallback to simple HTTP search
		return fallbackSearch(query, numResults)
	}

	return exaSearch(query, numResults, apiKey)
}

func exaSearch(query string, numResults int, apiKey string) string {
	payload := map[string]any{
		"query":      query,
		"numResults": numResults,
		"type":       "auto",
		"contents": map[string]any{
			"text": map[string]any{
				"maxCharacters": 1000,
			},
		},
	}

	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "https://api.exa.ai/search", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Search failed: %v", err)})
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return toJSON(map[string]any{"error": "Failed to parse search results"})
	}

	results, ok := result["results"].([]any)
	if !ok {
		return toJSON(map[string]any{"error": "No results found", "raw": string(respBody)})
	}

	var searchResults []map[string]any
	for _, r := range results {
		if rm, ok := r.(map[string]any); ok {
			searchResults = append(searchResults, map[string]any{
				"title":   rm["title"],
				"url":     rm["url"],
				"snippet": truncateOutput(fmt.Sprintf("%v", rm["text"]), 500),
			})
		}
	}

	return toJSON(map[string]any{
		"query":   query,
		"results": searchResults,
		"count":   len(searchResults),
	})
}

func fallbackSearch(query string, numResults int) string {
	return toJSON(map[string]any{
		"error":   "No search API configured",
		"message": "Set EXA_API_KEY in ~/.hermes/.env to enable web search",
		"query":   query,
	})
}

func handleWebExtract(args map[string]any, ctx *ToolContext) string {
	urlsRaw, ok := args["urls"].([]any)
	if !ok || len(urlsRaw) == 0 {
		return `{"error":"urls is required"}`
	}

	var urls []string
	for _, u := range urlsRaw {
		if s, ok := u.(string); ok {
			urls = append(urls, s)
		}
	}

	firecrawlKey := os.Getenv("FIRECRAWL_API_KEY")

	var results []map[string]any
	for _, u := range urls {
		if firecrawlKey != "" {
			result := firecrawlExtract(u, firecrawlKey)
			results = append(results, result)
		} else {
			result := simpleExtract(u)
			results = append(results, result)
		}
	}

	return toJSON(map[string]any{
		"results": results,
		"count":   len(results),
	})
}

func firecrawlExtract(targetURL, apiKey string) map[string]any {
	payload := map[string]any{
		"url":     targetURL,
		"formats": []string{"markdown"},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "https://api.firecrawl.dev/v1/scrape", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"url": targetURL, "error": fmt.Sprintf("Extract failed: %v", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result map[string]any
	json.Unmarshal(respBody, &result)

	if data, ok := result["data"].(map[string]any); ok {
		content := ""
		if md, ok := data["markdown"].(string); ok {
			content = md
		}
		return map[string]any{
			"url":     targetURL,
			"content": truncateOutput(content, 50000),
			"title":   data["metadata"],
		}
	}

	return map[string]any{"url": targetURL, "error": "No content extracted"}
}

func simpleExtract(targetURL string) map[string]any {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return map[string]any{"url": targetURL, "error": "Invalid URL"}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, _ := http.NewRequest("GET", parsed.String(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HermesAgent/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"url": targetURL, "error": fmt.Sprintf("Fetch failed: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 500000))
	content := string(body)

	// Basic HTML stripping
	content = stripHTML(content)

	return map[string]any{
		"url":     targetURL,
		"content": truncateOutput(content, 50000),
		"status":  resp.StatusCode,
	}
}

func checkFirecrawlRequirements() bool {
	return os.Getenv("FIRECRAWL_API_KEY") != ""
}

func handleWebCrawl(args map[string]any, ctx *ToolContext) string {
	crawlURL, _ := args["url"].(string)
	if crawlURL == "" {
		return `{"error":"url is required"}`
	}

	maxPages := 5
	if mp, ok := args["max_pages"].(float64); ok && mp > 0 {
		maxPages = int(mp)
		if maxPages > 20 {
			maxPages = 20
		}
	}

	outputFormat := "markdown"
	if f, ok := args["format"].(string); ok && (f == "markdown" || f == "text") {
		outputFormat = f
	}

	firecrawlKey := os.Getenv("FIRECRAWL_API_KEY")
	if firecrawlKey == "" {
		return toJSON(map[string]any{
			"error":   "FIRECRAWL_API_KEY is not set",
			"message": "Set FIRECRAWL_API_KEY in ~/.hermes/.env to enable web crawling",
		})
	}

	// Start crawl via Firecrawl API.
	payload := map[string]any{
		"url":   crawlURL,
		"limit": maxPages,
		"scrapeOptions": map[string]any{
			"formats": []string{outputFormat},
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "https://api.firecrawl.dev/v1/crawl", strings.NewReader(string(body)))
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to create request: %v", err)})
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+firecrawlKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Crawl request failed: %v", err)})
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var crawlResp map[string]any
	if err := json.Unmarshal(respBody, &crawlResp); err != nil {
		return toJSON(map[string]any{"error": "Failed to parse crawl response"})
	}

	// Firecrawl v1 crawl returns an async job. Check if we got a job ID.
	jobID, hasJobID := crawlResp["id"].(string)
	if !hasJobID {
		// Some responses return data directly.
		if data, ok := crawlResp["data"].([]any); ok {
			return formatCrawlResults(crawlURL, data, outputFormat)
		}
		return toJSON(map[string]any{
			"error": "Unexpected crawl response",
			"raw":   string(respBody),
		})
	}

	// Poll for crawl completion.
	pollURL := fmt.Sprintf("https://api.firecrawl.dev/v1/crawl/%s", jobID)
	pollClient := &http.Client{Timeout: 15 * time.Second}

	for attempt := 0; attempt < 30; attempt++ {
		time.Sleep(2 * time.Second)

		pollReq, _ := http.NewRequest("GET", pollURL, nil)
		pollReq.Header.Set("Authorization", "Bearer "+firecrawlKey)

		pollResp, err := pollClient.Do(pollReq)
		if err != nil {
			continue
		}

		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var statusResp map[string]any
		if err := json.Unmarshal(pollBody, &statusResp); err != nil {
			continue
		}

		status, _ := statusResp["status"].(string)
		if status == "completed" {
			if data, ok := statusResp["data"].([]any); ok {
				return formatCrawlResults(crawlURL, data, outputFormat)
			}
			return toJSON(map[string]any{
				"status":  "completed",
				"message": "Crawl completed but no data returned",
			})
		}

		if status == "failed" {
			errMsg, _ := statusResp["error"].(string)
			return toJSON(map[string]any{
				"error":  "Crawl failed",
				"detail": errMsg,
			})
		}

		// Still in progress, keep polling.
	}

	return toJSON(map[string]any{
		"error":  "Crawl timed out",
		"job_id": jobID,
		"hint":   "The crawl is still running. Try again later or reduce max_pages.",
	})
}

func formatCrawlResults(startURL string, data []any, format string) string {
	var pages []map[string]any
	for _, item := range data {
		if page, ok := item.(map[string]any); ok {
			pageResult := map[string]any{
				"url": page["url"],
			}

			// Extract content based on format.
			if format == "markdown" {
				if md, ok := page["markdown"].(string); ok {
					pageResult["content"] = truncateOutput(md, 30000)
				}
			} else {
				if text, ok := page["text"].(string); ok {
					pageResult["content"] = truncateOutput(text, 30000)
				} else if md, ok := page["markdown"].(string); ok {
					pageResult["content"] = truncateOutput(md, 30000)
				}
			}

			if metadata, ok := page["metadata"].(map[string]any); ok {
				if title, ok := metadata["title"].(string); ok {
					pageResult["title"] = title
				}
			}

			pages = append(pages, pageResult)
		}
	}

	return toJSON(map[string]any{
		"start_url": startURL,
		"pages":     pages,
		"count":     len(pages),
	})
}

func stripHTML(s string) string {
	// Remove script and style tags with content
	for _, tag := range []string{"script", "style"} {
		for {
			start := strings.Index(strings.ToLower(s), "<"+tag)
			if start == -1 {
				break
			}
			end := strings.Index(strings.ToLower(s[start:]), "</"+tag+">")
			if end == -1 {
				break
			}
			s = s[:start] + s[start+end+len("</"+tag+">"):]
		}
	}

	// Remove HTML tags
	var result strings.Builder
	inTag := false
	for _, ch := range s {
		if ch == '<' {
			inTag = true
		} else if ch == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(ch)
		}
	}

	// Clean up whitespace
	lines := strings.Split(result.String(), "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	return strings.Join(cleaned, "\n")
}
