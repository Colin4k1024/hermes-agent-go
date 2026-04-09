package tools

import (
	"regexp"
	"strings"
)

// DangerousPatterns contains regex patterns for dangerous commands.
// Each entry has a pattern and a human-readable reason.
var DangerousPatterns = []struct {
	Pattern *regexp.Regexp
	Reason  string
}{
	{
		Pattern: regexp.MustCompile(`\brm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+|--force\s+)?(-[a-zA-Z]*r[a-zA-Z]*|--recursive)\s`),
		Reason:  "Recursive file deletion (rm -rf)",
	},
	{
		Pattern: regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*\s+|--recursive\s+)?(-[a-zA-Z]*f[a-zA-Z]*|--force)\s`),
		Reason:  "Forced file deletion (rm -f)",
	},
	{
		Pattern: regexp.MustCompile(`(?i)\bDROP\s+(TABLE|DATABASE|SCHEMA|INDEX)\b`),
		Reason:  "SQL DROP statement",
	},
	{
		Pattern: regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+\S+\s*(;|$)`),
		Reason:  "SQL DELETE without WHERE clause",
	},
	{
		Pattern: regexp.MustCompile(`(?i)\bTRUNCATE\s+(TABLE\s+)?\S+`),
		Reason:  "SQL TRUNCATE statement",
	},
	{
		Pattern: regexp.MustCompile(`\bgit\s+push\s+.*--force\b`),
		Reason:  "Git force push",
	},
	{
		Pattern: regexp.MustCompile(`\bgit\s+push\s+-f\b`),
		Reason:  "Git force push (-f)",
	},
	{
		Pattern: regexp.MustCompile(`\bgit\s+reset\s+--hard\b`),
		Reason:  "Git hard reset",
	},
	{
		Pattern: regexp.MustCompile(`\bgit\s+clean\s+.*-f`),
		Reason:  "Git clean with force",
	},
	{
		Pattern: regexp.MustCompile(`\bchmod\s+(-[a-zA-Z]*R[a-zA-Z]*\s+)?777\b`),
		Reason:  "Setting world-writable permissions",
	},
	{
		Pattern: regexp.MustCompile(`\b(mkfs|fdisk|dd\s+if=)\b`),
		Reason:  "Disk formatting or low-level write",
	},
	{
		Pattern: regexp.MustCompile(`>\s*/dev/sd[a-z]`),
		Reason:  "Writing directly to disk device",
	},
	{
		Pattern: regexp.MustCompile(`(?i)\b(shutdown|reboot|halt|poweroff)\b`),
		Reason:  "System shutdown/reboot",
	},
	{
		Pattern: regexp.MustCompile(`\bkubectl\s+delete\s+(namespace|ns|deployment|pod)\b`),
		Reason:  "Kubernetes resource deletion",
	},
	{
		Pattern: regexp.MustCompile(`\bcurl\s+.*\|\s*(bash|sh|zsh)\b`),
		Reason:  "Piping remote script to shell",
	},
	{
		Pattern: regexp.MustCompile(`\bwget\s+.*-O\s*-\s*\|\s*(bash|sh|zsh)\b`),
		Reason:  "Piping remote script to shell",
	},
	{
		Pattern: regexp.MustCompile(`\b:\(\)\s*\{\s*:\|\:&\s*\}\s*;`),
		Reason:  "Fork bomb",
	},
	{
		Pattern: regexp.MustCompile(`>\s*/etc/(passwd|shadow|sudoers|hosts)\b`),
		Reason:  "Overwriting critical system file",
	},
	{
		Pattern: regexp.MustCompile(`\bsudo\s+rm\b`),
		Reason:  "Deleting files with sudo",
	},
	{
		Pattern: regexp.MustCompile(`\brm\s+.*(/|~|\$HOME)\s*$`),
		Reason:  "Deleting root or home directory",
	},
}

// IsDangerousCommand checks if a command string matches any dangerous patterns.
// Returns true and the reason if a match is found.
func IsDangerousCommand(cmd string) (bool, string) {
	// Normalize whitespace
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false, ""
	}

	for _, dp := range DangerousPatterns {
		if dp.Pattern.MatchString(cmd) {
			return true, dp.Reason
		}
	}

	return false, ""
}

// GetAllDangerousReasons returns all matching danger reasons for a command.
func GetAllDangerousReasons(cmd string) []string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}

	var reasons []string
	for _, dp := range DangerousPatterns {
		if dp.Pattern.MatchString(cmd) {
			reasons = append(reasons, dp.Reason)
		}
	}
	return reasons
}
