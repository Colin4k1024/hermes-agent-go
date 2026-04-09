package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// maxLogFiles is the number of rotated log files to keep.
const maxLogFiles = 5

// SetupLogging configures the global slog logger.
// When debug is true, the log level is set to DEBUG; otherwise INFO.
// Logs are written to both stderr and the log file.
func SetupLogging(debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	// Try to open a log file; fall back to stderr only.
	logPath := GetLogFile()
	if err := config.EnsureDir(filepath.Dir(logPath)); err == nil {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level}))
			slog.SetDefault(logger)
			return logger
		}
	}

	// Fallback: log to stderr.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}

// GetLogFile returns the path to the current log file.
func GetLogFile() string {
	return filepath.Join(config.HermesHome(), "logs", "hermes.log")
}

// RotateLogs rotates the current log file and keeps the last maxLogFiles backups.
// hermes.log -> hermes.log.1 -> hermes.log.2 -> ... -> hermes.log.5 (deleted)
func RotateLogs() error {
	logPath := GetLogFile()

	// If the current log file does not exist, nothing to rotate.
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return nil
	}

	logDir := filepath.Dir(logPath)
	baseName := filepath.Base(logPath)

	// Collect existing rotated files.
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return fmt.Errorf("read log directory: %w", err)
	}

	var rotated []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, baseName+".") && name != baseName {
			rotated = append(rotated, name)
		}
	}

	// Sort descending so we rename highest numbered first.
	sort.Sort(sort.Reverse(sort.StringSlice(rotated)))

	// Remove excess rotated files.
	for i, name := range rotated {
		fullPath := filepath.Join(logDir, name)
		if i >= maxLogFiles-1 {
			os.Remove(fullPath)
		}
	}

	// Shift existing rotated files: .4 -> .5, .3 -> .4, etc.
	for i := maxLogFiles - 1; i >= 1; i-- {
		src := filepath.Join(logDir, fmt.Sprintf("%s.%d", baseName, i))
		dst := filepath.Join(logDir, fmt.Sprintf("%s.%d", baseName, i+1))
		if _, err := os.Stat(src); err == nil {
			os.Rename(src, dst)
		}
	}

	// Rename current log to .1.
	dst := filepath.Join(logDir, fmt.Sprintf("%s.1", baseName))
	if err := os.Rename(logPath, dst); err != nil {
		return fmt.Errorf("rotate log file: %w", err)
	}

	return nil
}
