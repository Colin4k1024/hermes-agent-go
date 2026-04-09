package config

import (
	"os"
	"strings"
	"testing"
)

func TestHermesHome(t *testing.T) {
	// Test default
	os.Unsetenv("HERMES_HOME")
	home := HermesHome()
	if !strings.HasSuffix(home, ".hermes") {
		t.Errorf("Expected path ending in .hermes, got %s", home)
	}

	// Test override
	os.Setenv("HERMES_HOME", "/tmp/test-hermes")
	defer os.Unsetenv("HERMES_HOME")
	home = HermesHome()
	if home != "/tmp/test-hermes" {
		t.Errorf("Expected /tmp/test-hermes, got %s", home)
	}
}

func TestDisplayHermesHome(t *testing.T) {
	os.Unsetenv("HERMES_HOME")
	display := DisplayHermesHome()
	if !strings.HasPrefix(display, "~/.hermes") {
		t.Errorf("Expected display starting with ~/.hermes, got %s", display)
	}
}

func TestGetHermesDir(t *testing.T) {
	os.Setenv("HERMES_HOME", t.TempDir())
	defer os.Unsetenv("HERMES_HOME")

	// New path (old doesn't exist)
	dir := GetHermesDir("cache/images", "image_cache")
	if !strings.HasSuffix(dir, "cache/images") {
		t.Errorf("Expected new path, got %s", dir)
	}

	// Create old path
	home := HermesHome()
	os.MkdirAll(home+"/image_cache", 0755)

	dir = GetHermesDir("cache/images", "image_cache")
	if !strings.HasSuffix(dir, "image_cache") {
		t.Errorf("Expected old path when it exists, got %s", dir)
	}
}
