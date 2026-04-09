package agent

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// ContextFile represents a context reference file discovered on disk.
type ContextFile struct {
	Path    string // absolute or relative path to the file
	Content string // file contents
	Type    string // "soul", "agents", "cursorrules", "copilot", "readme"
}

// contextFileCandidates defines the files to scan for, in priority order.
var contextFileCandidates = []struct {
	// rel is the path relative to the scan root.
	rel string
	// fileType is the ContextFile.Type value.
	fileType string
}{
	{"SOUL.md", "soul"},
	{"AGENTS.md", "agents"},
	{".cursorrules", "cursorrules"},
	{".github/copilot-instructions.md", "copilot"},
}

// LoadContextReferences scans the workspace and hermes home for context files.
// dir is the workspace directory (typically the current working directory).
func LoadContextReferences(dir string) []ContextFile {
	var files []ContextFile

	// 1. Global context from ~/.hermes/
	hermesHome := config.HermesHome()
	globalCandidates := []struct {
		rel      string
		fileType string
	}{
		{"SOUL.md", "soul"},
	}

	for _, c := range globalCandidates {
		path := filepath.Join(hermesHome, c.rel)
		content, err := os.ReadFile(path)
		if err != nil || len(content) == 0 {
			continue
		}
		files = append(files, ContextFile{
			Path:    path,
			Content: string(content),
			Type:    c.fileType,
		})
		slog.Debug("Loaded global context file", "path", path, "type", c.fileType)
	}

	// 2. Project context from the workspace directory.
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return files
		}
	}

	for _, c := range contextFileCandidates {
		path := filepath.Join(dir, c.rel)
		content, err := os.ReadFile(path)
		if err != nil || len(content) == 0 {
			continue
		}

		// Skip duplicates (e.g., SOUL.md if workspace == hermesHome).
		if isDuplicate(files, path) {
			continue
		}

		files = append(files, ContextFile{
			Path:    path,
			Content: string(content),
			Type:    c.fileType,
		})
		slog.Debug("Loaded project context file", "path", path, "type", c.fileType)
	}

	// 3. Check for README.md as a fallback if nothing else was found.
	if len(files) == 0 {
		readmePath := filepath.Join(dir, "README.md")
		content, err := os.ReadFile(readmePath)
		if err == nil && len(content) > 0 {
			// Only include a truncated version to avoid bloating the prompt.
			text := string(content)
			if len(text) > 2000 {
				text = text[:2000] + "\n\n... (truncated)"
			}
			files = append(files, ContextFile{
				Path:    readmePath,
				Content: text,
				Type:    "readme",
			})
		}
	}

	return files
}

// isDuplicate checks whether a path is already in the list.
func isDuplicate(files []ContextFile, path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for _, f := range files {
		fabs, err := filepath.Abs(f.Path)
		if err != nil {
			fabs = f.Path
		}
		if fabs == abs {
			return true
		}
	}
	return false
}
