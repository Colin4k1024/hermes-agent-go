package gateway

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// MediaCache handles downloading and caching of media files.
type MediaCache struct {
	baseDir string
	client  *http.Client
}

// NewMediaCache creates a new media cache.
func NewMediaCache() *MediaCache {
	baseDir := filepath.Join(config.HermesHome(), "cache")
	// Ensure subdirectories exist.
	for _, sub := range []string{"images", "audio", "documents"} {
		os.MkdirAll(filepath.Join(baseDir, sub), 0755)
	}

	return &MediaCache{
		baseDir: baseDir,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// CacheImageFromURL downloads an image from a URL and caches it locally.
func (mc *MediaCache) CacheImageFromURL(url string) (string, error) {
	ext := guessExtension(url, ".jpg")
	return mc.cacheFromURL(url, "images", ext)
}

// CacheImageFromBytes saves image bytes to the cache.
func (mc *MediaCache) CacheImageFromBytes(data []byte, ext string) (string, error) {
	if ext == "" {
		ext = ".jpg"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return mc.cacheFromBytes(data, "images", ext)
}

// CacheAudioFromURL downloads an audio file from a URL and caches it locally.
func (mc *MediaCache) CacheAudioFromURL(url string) (string, error) {
	ext := guessExtension(url, ".ogg")
	return mc.cacheFromURL(url, "audio", ext)
}

// CacheDocumentFromBytes saves document bytes to the cache.
func (mc *MediaCache) CacheDocumentFromBytes(data []byte, filename string) (string, error) {
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".bin"
	}
	return mc.cacheFromBytes(data, "documents", ext)
}

// CleanupCache removes cached files older than maxAgeHours.
// Returns the number of files removed.
func (mc *MediaCache) CleanupCache(maxAgeHours int) int {
	if maxAgeHours <= 0 {
		maxAgeHours = 24
	}

	cutoff := time.Now().Add(-time.Duration(maxAgeHours) * time.Hour)
	removed := 0

	for _, subdir := range []string{"images", "audio", "documents"} {
		dir := filepath.Join(mc.baseDir, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				path := filepath.Join(dir, entry.Name())
				if err := os.Remove(path); err == nil {
					removed++
					slog.Debug("Removed cached file", "path", path)
				}
			}
		}
	}

	if removed > 0 {
		slog.Info("Media cache cleanup", "removed", removed, "max_age_hours", maxAgeHours)
	}

	return removed
}

// CacheDir returns the base cache directory.
func (mc *MediaCache) CacheDir() string {
	return mc.baseDir
}

// --- Internal helpers ---

func (mc *MediaCache) cacheFromURL(url, subdir, ext string) (string, error) {
	// Generate a deterministic filename from the URL.
	hash := sha256.Sum256([]byte(url))
	filename := fmt.Sprintf("%x%s", hash[:8], ext)
	destPath := filepath.Join(mc.baseDir, subdir, filename)

	// Check if already cached.
	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil
	}

	// Download.
	resp, err := mc.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	// Write to temp file then rename for atomicity.
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("write file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename file: %w", err)
	}

	slog.Debug("Cached media file", "url", url, "path", destPath)
	return destPath, nil
}

func (mc *MediaCache) cacheFromBytes(data []byte, subdir, ext string) (string, error) {
	// Generate filename from content hash.
	hash := sha256.Sum256(data)
	filename := fmt.Sprintf("%x%s", hash[:8], ext)
	destPath := filepath.Join(mc.baseDir, subdir, filename)

	// Check if already cached.
	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil
	}

	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	slog.Debug("Cached media from bytes", "path", destPath, "size", len(data))
	return destPath, nil
}

// guessExtension tries to determine the file extension from a URL.
func guessExtension(url, defaultExt string) string {
	// Remove query parameters.
	clean := url
	if idx := strings.Index(clean, "?"); idx >= 0 {
		clean = clean[:idx]
	}
	if idx := strings.Index(clean, "#"); idx >= 0 {
		clean = clean[:idx]
	}

	ext := filepath.Ext(clean)
	if ext != "" && len(ext) <= 5 {
		return strings.ToLower(ext)
	}
	return defaultExt
}
