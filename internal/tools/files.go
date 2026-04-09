package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/utils"
)

func init() {
	Register(&ToolEntry{
		Name:    "read_file",
		Toolset: "file",
		Schema: map[string]any{
			"name":        "read_file",
			"description": "Read the contents of a file. Returns the file content as text.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "Path to the file to read",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "Line number to start reading from (0-based)",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of lines to read",
					},
				},
				"required": []string{"file_path"},
			},
		},
		Handler: handleReadFile,
		Emoji:   "📖",
	})

	Register(&ToolEntry{
		Name:    "write_file",
		Toolset: "file",
		Schema: map[string]any{
			"name":        "write_file",
			"description": "Write content to a file. Creates the file if it doesn't exist, overwrites if it does.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "Path to the file to write",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Content to write to the file",
					},
				},
				"required": []string{"file_path", "content"},
			},
		},
		Handler: handleWriteFile,
		Emoji:   "✏️",
	})

	Register(&ToolEntry{
		Name:    "patch",
		Toolset: "file",
		Schema: map[string]any{
			"name":        "patch",
			"description": "Apply a targeted edit to a file by replacing a specific string with new content. The old_string must match exactly.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "Path to the file to edit",
					},
					"old_string": map[string]any{
						"type":        "string",
						"description": "The exact string to find and replace",
					},
					"new_string": map[string]any{
						"type":        "string",
						"description": "The replacement string",
					},
				},
				"required": []string{"file_path", "old_string", "new_string"},
			},
		},
		Handler: handlePatch,
		Emoji:   "🔧",
	})

	Register(&ToolEntry{
		Name:    "search_files",
		Toolset: "file",
		Schema: map[string]any{
			"name":        "search_files",
			"description": "Search for files by name pattern (glob) or search file contents with regex. Returns matching files or lines.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"directory": map[string]any{
						"type":        "string",
						"description": "Directory to search in (default: current directory)",
					},
					"pattern": map[string]any{
						"type":        "string",
						"description": "Glob pattern for file names (e.g., '*.go', '**/*.py')",
					},
					"content_regex": map[string]any{
						"type":        "string",
						"description": "Regex pattern to search in file contents",
					},
					"max_results": map[string]any{
						"type":        "integer",
						"description": "Maximum number of results (default: 50)",
						"default":     50,
					},
				},
			},
		},
		Handler: handleSearchFiles,
		Emoji:   "🔍",
	})
}

func handleReadFile(args map[string]any, ctx *ToolContext) string {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return `{"error":"file_path is required"}`
	}
	filePath = absPath(filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Cannot read file: %v", err)})
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// Apply offset and limit
	offset := 0
	if o, ok := args["offset"].(float64); ok && o > 0 {
		offset = int(o)
	}

	limit := len(lines)
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if offset >= len(lines) {
		return toJSON(map[string]any{
			"content":     "",
			"total_lines": len(lines),
			"message":     "Offset exceeds file length",
		})
	}

	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}

	// Add line numbers
	var numbered []string
	for i := offset; i < end; i++ {
		numbered = append(numbered, fmt.Sprintf("%d\t%s", i+1, lines[i]))
	}

	return toJSON(map[string]any{
		"content":     strings.Join(numbered, "\n"),
		"total_lines": len(lines),
		"offset":      offset,
		"lines_read":  end - offset,
	})
}

func handleWriteFile(args map[string]any, ctx *ToolContext) string {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return `{"error":"file_path is required"}`
	}
	filePath = absPath(filePath)

	content, _ := args["content"].(string)

	// Safety check
	if !utils.IsPathSafe(filePath) {
		return toJSON(map[string]any{"error": "Write denied: path is in the deny list"})
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Cannot create directory: %v", err)})
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Cannot write file: %v", err)})
	}

	lines := strings.Count(content, "\n") + 1
	return toJSON(map[string]any{
		"success":   true,
		"file_path": filePath,
		"lines":     lines,
		"bytes":     len(content),
	})
}

func handlePatch(args map[string]any, ctx *ToolContext) string {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return `{"error":"file_path is required"}`
	}
	filePath = absPath(filePath)

	oldString, _ := args["old_string"].(string)
	newString, _ := args["new_string"].(string)

	if oldString == "" {
		return `{"error":"old_string is required"}`
	}

	// Safety check
	if !utils.IsPathSafe(filePath) {
		return toJSON(map[string]any{"error": "Write denied: path is in the deny list"})
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Cannot read file: %v", err)})
	}

	content := string(data)

	// Check for exact match
	count := strings.Count(content, oldString)
	if count == 0 {
		// Try fuzzy match (ignore whitespace differences)
		return toJSON(map[string]any{
			"error":   "old_string not found in file",
			"hint":    "Make sure the old_string matches exactly, including whitespace and indentation",
			"file":    filePath,
		})
	}
	if count > 1 {
		return toJSON(map[string]any{
			"error":       "old_string matches multiple locations",
			"match_count": count,
			"hint":        "Provide more context in old_string to make it unique",
		})
	}

	// Apply the patch
	newContent := strings.Replace(content, oldString, newString, 1)
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Cannot write file: %v", err)})
	}

	return toJSON(map[string]any{
		"success":   true,
		"file_path": filePath,
		"message":   "Patch applied successfully",
	})
}

func handleSearchFiles(args map[string]any, ctx *ToolContext) string {
	dir, _ := args["directory"].(string)
	if dir == "" {
		dir, _ = os.Getwd()
	}
	dir = absPath(dir)

	pattern, _ := args["pattern"].(string)
	contentRegex, _ := args["content_regex"].(string)

	maxResults := 50
	if m, ok := args["max_results"].(float64); ok && m > 0 {
		maxResults = int(m)
	}

	var results []map[string]any

	if pattern != "" {
		// File name search with glob
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			// Try recursive glob
			err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil || len(results) >= maxResults {
					return err
				}
				matched, _ := filepath.Match(pattern, filepath.Base(path))
				if matched {
					relPath, _ := filepath.Rel(dir, path)
					results = append(results, map[string]any{
						"path": relPath,
						"size": info.Size(),
						"type": fileType(info),
					})
				}
				return nil
			})
			if err != nil {
				return toJSON(map[string]any{"error": fmt.Sprintf("Search error: %v", err)})
			}
		} else {
			for _, m := range matches {
				if len(results) >= maxResults {
					break
				}
				info, err := os.Stat(m)
				if err != nil {
					continue
				}
				relPath, _ := filepath.Rel(dir, m)
				results = append(results, map[string]any{
					"path": relPath,
					"size": info.Size(),
					"type": fileType(info),
				})
			}
		}
	}

	if contentRegex != "" {
		re, err := regexp.Compile(contentRegex)
		if err != nil {
			return toJSON(map[string]any{"error": fmt.Sprintf("Invalid regex: %v", err)})
		}

		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || len(results) >= maxResults {
				return nil
			}
			// Skip binary files and large files
			if info.Size() > 1024*1024 { // 1MB
				return nil
			}
			// Skip hidden directories
			if strings.HasPrefix(filepath.Base(path), ".") && path != dir {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if re.MatchString(line) {
					relPath, _ := filepath.Rel(dir, path)
					results = append(results, map[string]any{
						"path":    relPath,
						"line":    i + 1,
						"content": truncateOutput(strings.TrimSpace(line), 200),
					})
					if len(results) >= maxResults {
						return filepath.SkipAll
					}
				}
			}
			return nil
		})
	}

	b, _ := json.Marshal(map[string]any{
		"results":      results,
		"result_count": len(results),
		"directory":    dir,
	})
	return string(b)
}

func fileType(info os.FileInfo) string {
	if info.IsDir() {
		return "directory"
	}
	return "file"
}
