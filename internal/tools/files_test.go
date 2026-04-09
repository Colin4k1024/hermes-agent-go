package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644)

	result := handleReadFile(map[string]any{
		"file_path": testFile,
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	if m["error"] != nil {
		t.Errorf("Unexpected error: %v", m["error"])
	}
	totalLines, _ := m["total_lines"].(float64)
	if totalLines < 3 {
		t.Errorf("Expected at least 3 lines, got %v", totalLines)
	}
}

func TestReadFileNotFound(t *testing.T) {
	result := handleReadFile(map[string]any{
		"file_path": "/nonexistent/file.txt",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	if m["error"] == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestReadFileWithOffset(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644)

	result := handleReadFile(map[string]any{
		"file_path": testFile,
		"offset":    float64(2),
		"limit":     float64(2),
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	linesRead, _ := m["lines_read"].(float64)
	if linesRead != 2 {
		t.Errorf("Expected 2 lines read, got %v", linesRead)
	}
}

func TestWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "output.txt")

	result := handleWriteFile(map[string]any{
		"file_path": testFile,
		"content":   "Hello, World!\nSecond line.",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	if m["success"] != true {
		t.Errorf("Expected success, got: %s", result)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "Hello, World!\nSecond line." {
		t.Errorf("File content mismatch: %s", string(data))
	}
}

func TestWriteFileCreatesDirs(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "sub", "dir", "output.txt")

	result := handleWriteFile(map[string]any{
		"file_path": testFile,
		"content":   "nested content",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	if m["success"] != true {
		t.Errorf("Expected success for nested write, got: %s", result)
	}
}

func TestPatch(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "patch.txt")
	os.WriteFile(testFile, []byte("Hello World"), 0644)

	result := handlePatch(map[string]any{
		"file_path":  testFile,
		"old_string": "Hello",
		"new_string": "Goodbye",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	if m["success"] != true {
		t.Errorf("Expected patch success, got: %s", result)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "Goodbye World" {
		t.Errorf("Patch not applied correctly: %s", string(data))
	}
}

func TestPatchNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "patch.txt")
	os.WriteFile(testFile, []byte("Hello World"), 0644)

	result := handlePatch(map[string]any{
		"file_path":  testFile,
		"old_string": "Nonexistent text",
		"new_string": "Replacement",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	if m["error"] == nil {
		t.Error("Expected error when old_string not found")
	}
}

func TestSearchFiles(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("package test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "c.txt"), []byte("not go"), 0644)

	result := handleSearchFiles(map[string]any{
		"directory": tmpDir,
		"pattern":   "*.go",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	count, _ := m["result_count"].(float64)
	if count != 2 {
		t.Errorf("Expected 2 .go files, got %v", count)
	}
}

func TestSearchFilesContent(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("func test() {}\n"), 0644)

	result := handleSearchFiles(map[string]any{
		"directory":     tmpDir,
		"content_regex": "Println",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	count, _ := m["result_count"].(float64)
	if count != 1 {
		t.Errorf("Expected 1 match for 'Println', got %v", count)
	}
}
