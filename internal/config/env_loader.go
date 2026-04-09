package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadEnvFile loads a .env file with support for:
//   - Comments (lines starting with #)
//   - Quoted values (single and double quotes)
//   - Multi-line values (using backslash continuation)
//   - Variable expansion ($VAR and ${VAR})
//
// Variables are set in the process environment. Existing environment
// variables are NOT overwritten (env takes precedence over .env file).
func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open env file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var continuationKey string
	var continuationValue strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Handle backslash continuation from previous line.
		if continuationKey != "" {
			trimmed := strings.TrimSpace(line)
			if strings.HasSuffix(trimmed, "\\") {
				continuationValue.WriteString(trimmed[:len(trimmed)-1])
				continue
			}
			continuationValue.WriteString(trimmed)
			setEnvIfNotPresent(continuationKey, expandVars(continuationValue.String()))
			continuationKey = ""
			continuationValue.Reset()
			continue
		}

		// Strip leading/trailing whitespace.
		line = strings.TrimSpace(line)

		// Skip empty lines and comments.
		if line == "" || line[0] == '#' {
			continue
		}

		// Split on first '=' sign.
		eqIdx := strings.IndexByte(line, '=')
		if eqIdx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:eqIdx])
		value := strings.TrimSpace(line[eqIdx+1:])

		// Strip optional "export " prefix.
		key = strings.TrimPrefix(key, "export ")
		key = strings.TrimSpace(key)

		if key == "" {
			continue
		}

		// Handle quoted values.
		value = unquoteValue(value)

		// Handle backslash continuation.
		if strings.HasSuffix(value, "\\") {
			continuationKey = key
			continuationValue.Reset()
			continuationValue.WriteString(value[:len(value)-1])
			continue
		}

		// Expand variables and set.
		value = expandVars(value)
		setEnvIfNotPresent(key, value)
	}

	// Handle any trailing continuation.
	if continuationKey != "" {
		setEnvIfNotPresent(continuationKey, expandVars(continuationValue.String()))
	}

	return scanner.Err()
}

// GetAllConfiguredKeys returns a list of all environment variable keys from
// OptionalEnvVars that are currently set (non-empty) in the process environment.
func GetAllConfiguredKeys() []string {
	var keys []string
	for key := range OptionalEnvVars {
		if os.Getenv(key) != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// unquoteValue removes surrounding quotes from a value string.
// Handles both single quotes ('...') and double quotes ("...").
// Single-quoted values are returned as-is (no variable expansion).
// Double-quoted values support escape sequences.
func unquoteValue(s string) string {
	if len(s) < 2 {
		return s
	}

	// Double quotes.
	if s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		// Process basic escape sequences.
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		inner = strings.ReplaceAll(inner, `\n`, "\n")
		inner = strings.ReplaceAll(inner, `\t`, "\t")
		return inner
	}

	// Single quotes (literal, no escaping).
	if s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}

	return s
}

// expandVars expands $VAR and ${VAR} references in a string using
// the current process environment.
func expandVars(s string) string {
	return os.Expand(s, os.Getenv)
}

// setEnvIfNotPresent sets an environment variable only if it is not
// already set in the process environment.
func setEnvIfNotPresent(key, value string) {
	if os.Getenv(key) == "" {
		os.Setenv(key, value)
	}
}
