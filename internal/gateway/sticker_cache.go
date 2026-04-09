package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// StickerCache maps sticker IDs to their emoji/description for context injection.
type StickerCache struct {
	mu      sync.RWMutex
	entries map[string]StickerEntry
	path    string
}

// StickerEntry describes a cached sticker.
type StickerEntry struct {
	ID       string `json:"id"`
	Emoji    string `json:"emoji"`
	SetName  string `json:"set_name"`
	FileID   string `json:"file_id"`
	Platform string `json:"platform"`
}

// NewStickerCache creates or loads a sticker cache.
func NewStickerCache() *StickerCache {
	sc := &StickerCache{
		entries: make(map[string]StickerEntry),
		path:    filepath.Join(config.HermesHome(), "cache", "stickers.json"),
	}
	sc.load()
	return sc
}

// Get retrieves a cached sticker entry.
func (sc *StickerCache) Get(stickerID string) (StickerEntry, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	e, ok := sc.entries[stickerID]
	return e, ok
}

// Set stores a sticker entry and persists to disk.
func (sc *StickerCache) Set(entry StickerEntry) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.entries[entry.ID] = entry
	sc.save()
}

// DescribeSticker returns a text description for context injection.
func (sc *StickerCache) DescribeSticker(stickerID string) string {
	if e, ok := sc.Get(stickerID); ok {
		if e.Emoji != "" {
			return "[Sticker: " + e.Emoji + "]"
		}
		return "[Sticker from set: " + e.SetName + "]"
	}
	return "[Sticker]"
}

func (sc *StickerCache) load() {
	data, err := os.ReadFile(sc.path)
	if err != nil {
		return
	}
	var entries map[string]StickerEntry
	if json.Unmarshal(data, &entries) == nil {
		sc.entries = entries
	}
}

func (sc *StickerCache) save() {
	os.MkdirAll(filepath.Dir(sc.path), 0755)
	data, _ := json.MarshalIndent(sc.entries, "", "  ")
	os.WriteFile(sc.path, data, 0644)
}
