package tools

// BrowserBackend is the interface for pluggable browser automation providers.
// Implementations handle session lifecycle and CDP-like operations.
type BrowserBackend interface {
	// Name returns the backend identifier (e.g. "browserbase", "local").
	Name() string

	// Connect creates or connects to a browser session.
	Connect() error

	// Close terminates the browser session.
	Close()

	// Navigate loads a URL.
	Navigate(url string) (map[string]any, error)

	// Snapshot returns the current page DOM/accessibility tree.
	Snapshot() (map[string]any, error)

	// Click clicks an element by reference.
	Click(ref string) (map[string]any, error)

	// Type types text into an element.
	Type(ref, text string, clearFirst bool) (map[string]any, error)

	// Scroll scrolls the page.
	Scroll(direction string, amount int) (map[string]any, error)

	// GoBack navigates back.
	GoBack() (map[string]any, error)

	// PressKey presses a key.
	PressKey(key string) (map[string]any, error)

	// GetImages returns images on the page.
	GetImages() (map[string]any, error)

	// ExecuteScript runs JavaScript in the page.
	ExecuteScript(script string) (map[string]any, error)

	// CurrentURL returns the current page URL.
	CurrentURL() string

	// PageTitle returns the current page title.
	PageTitle() string
}
