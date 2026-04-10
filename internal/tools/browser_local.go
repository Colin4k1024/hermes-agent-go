package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// LocalBrowserBackend connects to a locally running Chrome/Chromium via CDP.
// It uses the Chrome DevTools Protocol over HTTP, no external service required.
type LocalBrowserBackend struct {
	mu         sync.Mutex
	client     *http.Client
	cdpURL     string
	targetID   string
	sessionID  string
	currenturl string
	pagetitle  string
}

// Compile-time interface check.
var _ BrowserBackend = (*LocalBrowserBackend)(nil)

func (b *LocalBrowserBackend) Name() string { return "local" }

func (b *LocalBrowserBackend) CurrentURL() string { return b.currenturl }

func (b *LocalBrowserBackend) PageTitle() string { return b.pagetitle }

// Connect discovers or launches a local Chrome and connects via CDP.
func (b *LocalBrowserBackend) Connect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.client = &http.Client{Timeout: 30 * time.Second}

	// Try user-specified CDP URL first, then default.
	cdpURL := os.Getenv("BROWSER_CDP_URL")
	if cdpURL == "" {
		cdpURL = "http://127.0.0.1:9222"
	}
	b.cdpURL = cdpURL

	// Try to connect to existing Chrome.
	if err := b.discoverTarget(); err == nil {
		return nil
	}

	// Try to launch Chrome with remote debugging.
	if launchErr := b.launchChrome(); launchErr != nil {
		return fmt.Errorf("no browser found at %s and failed to launch: %w", cdpURL, launchErr)
	}

	// Wait for Chrome to start and retry discovery.
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if err := b.discoverTarget(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("chrome launched but CDP not ready at %s", cdpURL)
}

func (b *LocalBrowserBackend) discoverTarget() error {
	resp, err := b.client.Get(b.cdpURL + "/json/list")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read target list: %w", err)
	}

	var targets []struct {
		ID                string `json:"id"`
		Type              string `json:"type"`
		WebSocketDebugURL string `json:"webSocketDebuggerUrl"`
		URL               string `json:"url"`
		Title             string `json:"title"`
	}
	if err := json.Unmarshal(body, &targets); err != nil {
		return fmt.Errorf("parse target list: %w", err)
	}

	// Find a page target.
	for _, t := range targets {
		if t.Type == "page" {
			b.targetID = t.ID
			b.currenturl = t.URL
			b.pagetitle = t.Title
			return nil
		}
	}

	// No page target — create a new one.
	resp2, err := b.client.Get(b.cdpURL + "/json/new")
	if err != nil {
		return fmt.Errorf("create new tab: %w", err)
	}
	defer resp2.Body.Close()

	var newTarget struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&newTarget); err != nil {
		return fmt.Errorf("parse new tab: %w", err)
	}
	b.targetID = newTarget.ID
	return nil
}

func (b *LocalBrowserBackend) launchChrome() error {
	chromePath := findChromeBinary()
	if chromePath == "" {
		return fmt.Errorf("chrome/chromium not found in PATH or standard locations")
	}

	cmd := exec.Command(chromePath,
		"--remote-debugging-port=9222",
		"--no-first-run",
		"--no-default-browser-check",
		"--headless=new",
		"about:blank",
	)
	return cmd.Start()
}

func findChromeBinary() string {
	// Check PATH first.
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}

	// Platform-specific paths.
	switch runtime.GOOS {
	case "darwin":
		paths := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	case "windows":
		paths := []string{
			os.Getenv("LOCALAPPDATA") + `\Google\Chrome\Application\chrome.exe`,
			os.Getenv("PROGRAMFILES") + `\Google\Chrome\Application\chrome.exe`,
			os.Getenv("PROGRAMFILES(X86)") + `\Google\Chrome\Application\chrome.exe`,
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// cdpHTTPCommand sends a CDP command via the HTTP debugging protocol.
func (b *LocalBrowserBackend) cdpHTTPCommand(method string, params map[string]any) (json.RawMessage, error) {
	payload := map[string]any{
		"method": method,
		"params": params,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal cdp command: %w", err)
	}

	url := fmt.Sprintf("%s/json/protocol/%s", b.cdpURL, b.targetID)
	resp, err := b.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		// Fallback: use simpler /json/activate + evaluate approach.
		return b.cdpViaEvaluate(method, params)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read cdp response: %w", err)
	}
	return json.RawMessage(respBody), nil
}

// cdpViaEvaluate uses Runtime.evaluate as a fallback for CDP commands
// when the protocol endpoint isn't available.
func (b *LocalBrowserBackend) cdpViaEvaluate(method string, params map[string]any) (json.RawMessage, error) {
	// For simple navigation/scripts, use direct HTTP endpoints.
	switch method {
	case "Page.navigate":
		urlStr, _ := params["url"].(string)
		activateURL := fmt.Sprintf("%s/json/activate/%s", b.cdpURL, b.targetID)
		b.client.Get(activateURL) //nolint:errcheck
		navigateURL := fmt.Sprintf("%s/json/navigate?url=%s&id=%s", b.cdpURL, urlStr, b.targetID)
		resp, err := b.client.Get(navigateURL)
		if err != nil {
			return nil, fmt.Errorf("navigate: %w", err)
		}
		resp.Body.Close()
		time.Sleep(2 * time.Second) // wait for page load
		return json.RawMessage(`{"success":true}`), nil
	default:
		return nil, fmt.Errorf("cdp method %s not supported via http fallback", method)
	}
}

// Navigate loads a URL in the local browser.
func (b *LocalBrowserBackend) Navigate(url string) (map[string]any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	_, err := b.cdpHTTPCommand("Page.navigate", map[string]any{"url": url})
	if err != nil {
		return nil, fmt.Errorf("navigate to %s: %w", url, err)
	}

	b.currenturl = url
	return map[string]any{
		"success": true,
		"url":     url,
		"message": fmt.Sprintf("navigated to %s", url),
	}, nil
}

// Snapshot returns page information (simplified for HTTP-only CDP).
func (b *LocalBrowserBackend) Snapshot() (map[string]any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Get page info via /json/list.
	resp, err := b.client.Get(b.cdpURL + "/json/list")
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	var targets []struct {
		ID    string `json:"id"`
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}

	for _, t := range targets {
		if t.ID == b.targetID {
			b.currenturl = t.URL
			b.pagetitle = t.Title
			return map[string]any{
				"url":   t.URL,
				"title": t.Title,
			}, nil
		}
	}

	return map[string]any{
		"url":   b.currenturl,
		"title": b.pagetitle,
		"note":  "target not found in tab list, returning cached state",
	}, nil
}

// Click is not directly supported via HTTP CDP protocol.
func (b *LocalBrowserBackend) Click(ref string) (map[string]any, error) {
	return nil, fmt.Errorf("click requires websocket CDP connection (not available in http-only mode); consider using browserbase backend")
}

// Type is not directly supported via HTTP CDP protocol.
func (b *LocalBrowserBackend) Type(ref, text string, clearFirst bool) (map[string]any, error) {
	return nil, fmt.Errorf("type requires websocket CDP connection (not available in http-only mode); consider using browserbase backend")
}

// Scroll via JavaScript evaluation.
func (b *LocalBrowserBackend) Scroll(direction string, amount int) (map[string]any, error) {
	var script string
	switch direction {
	case "down":
		script = fmt.Sprintf("window.scrollBy(0, %d)", amount)
	case "up":
		script = fmt.Sprintf("window.scrollBy(0, -%d)", amount)
	default:
		script = fmt.Sprintf("window.scrollBy(0, %d)", amount)
	}
	return b.ExecuteScript(script)
}

func (b *LocalBrowserBackend) GoBack() (map[string]any, error) {
	return b.ExecuteScript("window.history.back()")
}

func (b *LocalBrowserBackend) PressKey(key string) (map[string]any, error) {
	return nil, fmt.Errorf("press key requires websocket CDP connection; consider using browserbase backend")
}

func (b *LocalBrowserBackend) GetImages() (map[string]any, error) {
	return b.ExecuteScript(`JSON.stringify(Array.from(document.images).map(i => ({src: i.src, alt: i.alt})))`)
}

func (b *LocalBrowserBackend) ExecuteScript(script string) (map[string]any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Use /json/evaluate endpoint if available, otherwise error.
	payload := map[string]any{
		"expression": script,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal script: %w", err)
	}

	url := fmt.Sprintf("%s/json/evaluate?id=%s", b.cdpURL, b.targetID)
	resp, err := b.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return map[string]any{
			"error": fmt.Sprintf("script execution not available via http: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read script result: %w", err)
	}

	return map[string]any{
		"result": string(respBody),
	}, nil
}

func (b *LocalBrowserBackend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.targetID != "" {
		url := fmt.Sprintf("%s/json/close/%s", b.cdpURL, b.targetID)
		b.client.Get(url) //nolint:errcheck
		b.targetID = ""
	}
}
