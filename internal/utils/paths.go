package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// WriteDenyList contains paths that should never be written to.
var WriteDenyList = []string{
	".ssh/authorized_keys",
	".ssh/id_rsa",
	".ssh/id_ed25519",
	"/etc/shadow",
	"/etc/passwd",
	"/etc/sudoers",
}

// IsPathSafe checks if a path is safe to write to.
func IsPathSafe(path string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If the file doesn't exist yet, resolve parent
		dir := filepath.Dir(path)
		resolved, err = filepath.EvalSymlinks(dir)
		if err != nil {
			resolved = dir
		}
		resolved = filepath.Join(resolved, filepath.Base(path))
	}

	for _, denied := range WriteDenyList {
		if strings.HasSuffix(resolved, denied) {
			return false
		}
		// Also check full path
		home, _ := os.UserHomeDir()
		if home != "" {
			fullDenied := filepath.Join(home, denied)
			if resolved == fullDenied {
				return false
			}
		}
	}
	return true
}

// ExpandPath expands ~ to the user's home directory.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
