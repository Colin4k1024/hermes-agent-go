package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SecurityIssue represents a potential security concern in a skill.
type SecurityIssue struct {
	Severity string // "critical", "warning", "info"
	File     string
	Line     int
	Pattern  string
	Message  string
}

// ScanSkill scans a skill directory for security issues.
func ScanSkill(skillDir string) ([]SecurityIssue, error) {
	var issues []SecurityIssue

	err := filepath.Walk(skillDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Skip non-text files
		ext := strings.ToLower(filepath.Ext(path))
		textExts := map[string]bool{".md": true, ".py": true, ".sh": true, ".yaml": true, ".yml": true, ".json": true, ".txt": true}
		if !textExts[ext] {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(data)
		relPath, _ := filepath.Rel(skillDir, path)

		// Check for dangerous patterns
		fileIssues := scanContent(relPath, content)
		issues = append(issues, fileIssues...)

		return nil
	})

	return issues, err
}

// DangerousPatterns for skill security scanning.
var dangerousSkillPatterns = []struct {
	Pattern  *regexp.Regexp
	Severity string
	Message  string
}{
	{regexp.MustCompile(`(?i)rm\s+-rf\s+[/~]`), "critical", "Destructive file deletion"},
	{regexp.MustCompile(`(?i)curl.*\|\s*(bash|sh|zsh)`), "critical", "Remote code execution via pipe"},
	{regexp.MustCompile(`(?i)wget.*-O\s*-\s*\|\s*(bash|sh)`), "critical", "Remote code execution via pipe"},
	{regexp.MustCompile(`(?i)eval\s*\(`), "warning", "Dynamic code evaluation"},
	{regexp.MustCompile(`(?i)exec\s*\(`), "warning", "Dynamic code execution"},
	{regexp.MustCompile(`(?i)os\.system\s*\(`), "warning", "System command execution"},
	{regexp.MustCompile(`(?i)subprocess\.(call|run|Popen)`), "info", "Subprocess execution"},
	{regexp.MustCompile(`(?i)__import__`), "warning", "Dynamic import"},
	{regexp.MustCompile(`(?i)ignore.*previous.*instructions`), "critical", "Prompt injection attempt"},
	{regexp.MustCompile(`(?i)forget.*everything`), "critical", "Prompt injection attempt"},
	{regexp.MustCompile(`(?i)you\s+are\s+now`), "warning", "Potential identity override"},
	{regexp.MustCompile(`(?i)api[_-]?key|secret[_-]?key|password\s*=\s*["']`), "warning", "Hardcoded credential"},
	{regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`), "critical", "Exposed API key"},
	{regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`), "critical", "Exposed GitHub token"},
}

func scanContent(relPath, content string) []SecurityIssue {
	var issues []SecurityIssue

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		for _, pattern := range dangerousSkillPatterns {
			if pattern.Pattern.MatchString(line) {
				issues = append(issues, SecurityIssue{
					Severity: pattern.Severity,
					File:     relPath,
					Line:     i + 1,
					Pattern:  pattern.Pattern.String(),
					Message:  pattern.Message,
				})
			}
		}
	}

	return issues
}

// FormatIssues returns a human-readable summary of security issues.
func FormatIssues(issues []SecurityIssue) string {
	if len(issues) == 0 {
		return "No security issues found."
	}

	var sb strings.Builder
	criticals := 0
	warnings := 0
	infos := 0

	for _, issue := range issues {
		switch issue.Severity {
		case "critical":
			criticals++
		case "warning":
			warnings++
		case "info":
			infos++
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s:%d - %s\n", issue.Severity, issue.File, issue.Line, issue.Message))
	}

	header := fmt.Sprintf("Security scan: %d critical, %d warning, %d info\n", criticals, warnings, infos)
	return header + sb.String()
}
