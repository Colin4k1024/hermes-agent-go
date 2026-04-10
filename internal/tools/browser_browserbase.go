package tools

// BrowserbaseBackend wraps the existing BrowserSession to implement BrowserBackend.
type BrowserbaseBackend struct {
	session *BrowserSession
}

// Compile-time interface check.
var _ BrowserBackend = (*BrowserbaseBackend)(nil)

func (b *BrowserbaseBackend) Name() string { return "browserbase" }

func (b *BrowserbaseBackend) Connect() error {
	session, err := newBrowserbaseSession()
	if err != nil {
		return err
	}
	b.session = session
	return nil
}

func (b *BrowserbaseBackend) Close() {
	if b.session != nil {
		b.session.close()
	}
}

func (b *BrowserbaseBackend) Navigate(url string) (map[string]any, error) {
	return b.session.navigate(url)
}

func (b *BrowserbaseBackend) Snapshot() (map[string]any, error) {
	return b.session.snapshot()
}

func (b *BrowserbaseBackend) Click(ref string) (map[string]any, error) {
	return b.session.click(ref)
}

func (b *BrowserbaseBackend) Type(ref, text string, clearFirst bool) (map[string]any, error) {
	return b.session.typeText(ref, text, clearFirst)
}

func (b *BrowserbaseBackend) Scroll(direction string, amount int) (map[string]any, error) {
	return b.session.scroll(direction, amount)
}

func (b *BrowserbaseBackend) GoBack() (map[string]any, error) {
	return b.session.goBack()
}

func (b *BrowserbaseBackend) PressKey(key string) (map[string]any, error) {
	return b.session.pressKey(key)
}

func (b *BrowserbaseBackend) GetImages() (map[string]any, error) {
	return b.session.getImages()
}

func (b *BrowserbaseBackend) ExecuteScript(script string) (map[string]any, error) {
	return b.session.executeScript(script)
}

func (b *BrowserbaseBackend) CurrentURL() string {
	if b.session == nil {
		return ""
	}
	return b.session.currentURL
}

func (b *BrowserbaseBackend) PageTitle() string {
	if b.session == nil {
		return ""
	}
	return b.session.pageTitle
}
