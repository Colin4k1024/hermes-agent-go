package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// SubdirectoryHints provides working directory context for tools.
type SubdirectoryHints struct {
	WorkingDir string
	ProjectRoot string
	GitRoot    string
	Language   string
}

// DetectSubdirectoryHints analyzes the current directory for project context.
func DetectSubdirectoryHints() *SubdirectoryHints {
	cwd, err := os.Getwd()
	if err != nil {
		return &SubdirectoryHints{}
	}

	hints := &SubdirectoryHints{
		WorkingDir: cwd,
	}

	// Find git root
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			hints.GitRoot = dir
			hints.ProjectRoot = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if hints.ProjectRoot == "" {
		hints.ProjectRoot = cwd
	}

	// Detect primary language
	hints.Language = detectLanguage(hints.ProjectRoot)

	return hints
}

func detectLanguage(dir string) string {
	indicators := map[string]string{
		"go.mod":         "go",
		"Cargo.toml":     "rust",
		"package.json":   "javascript",
		"pyproject.toml": "python",
		"setup.py":       "python",
		"Gemfile":        "ruby",
		"pom.xml":        "java",
		"build.gradle":   "java",
		"mix.exs":        "elixir",
		"composer.json":  "php",
	}

	for file, lang := range indicators {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			return lang
		}
	}

	// Check by file extension prevalence
	extCounts := make(map[string]int)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		extCounts[ext]++
		return nil
	})

	langMap := map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript",
		".rs": "rust", ".rb": "ruby", ".java": "java", ".cs": "csharp",
	}

	maxCount := 0
	bestLang := ""
	for ext, lang := range langMap {
		if count, ok := extCounts[ext]; ok && count > maxCount {
			maxCount = count
			bestLang = lang
		}
	}

	return bestLang
}
