package utils

import (
	"os"
	"strings"
	"testing"
)

func TestIsPathSafe(t *testing.T) {
	tests := []struct {
		path string
		safe bool
	}{
		{"/tmp/test.txt", true},
		{"/home/user/file.go", true},
		{os.Getenv("HOME") + "/.ssh/authorized_keys", false},
		{"/etc/shadow", false},
		{"/etc/passwd", false},
	}

	for _, tt := range tests {
		result := IsPathSafe(tt.path)
		if result != tt.safe {
			t.Errorf("IsPathSafe(%q) = %v, want %v", tt.path, result, tt.safe)
		}
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	result := ExpandPath("~/test.txt")
	if !strings.HasPrefix(result, home) {
		t.Errorf("Expected path starting with %s, got %s", home, result)
	}

	result = ExpandPath("/absolute/path")
	if result != "/absolute/path" {
		t.Errorf("Expected /absolute/path, got %s", result)
	}

	result = ExpandPath("relative/path")
	if result != "relative/path" {
		t.Errorf("Expected relative/path, got %s", result)
	}
}
