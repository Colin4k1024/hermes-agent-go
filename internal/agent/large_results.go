package agent

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

const largeResultThreshold = 100_000 // chars

// SaveOversizedResult saves a tool result that exceeds the threshold to disk.
// It returns a JSON string containing the file path and a truncated preview.
func SaveOversizedResult(toolName, result string) string {
	dir := filepath.Join(config.HermesHome(), "cache", "tool_responses")
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Warn("Failed to create tool response cache dir", "error", err)
		return result // fall back to raw result
	}

	hash := sha256.Sum256([]byte(result))
	filename := fmt.Sprintf("%s_%s_%.0f.txt",
		toolName,
		fmt.Sprintf("%x", hash[:4]),
		float64(time.Now().UnixMilli()),
	)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, []byte(result), 0644); err != nil {
		slog.Warn("Failed to save oversized result", "error", err)
		return result
	}

	preview := result
	previewLen := 2000
	if len(preview) > previewLen {
		preview = preview[:previewLen]
	}

	return fmt.Sprintf(
		`{"saved_to":"%s","total_chars":%d,"preview":"%s...","note":"Full result saved to file. Use read_file to view."}`,
		path, len(result), escapeJSON(preview),
	)
}

// IsOversizedResult returns true if the result exceeds the threshold.
func IsOversizedResult(result string) bool {
	return len(result) > largeResultThreshold
}

// escapeJSON does minimal escaping for embedding in a JSON string value.
func escapeJSON(s string) string {
	var out []byte
	for _, b := range []byte(s) {
		switch b {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			out = append(out, b)
		}
	}
	return string(out)
}
