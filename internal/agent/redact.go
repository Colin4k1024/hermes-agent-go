package agent

import (
	"regexp"
	"strings"
)

// redactionPatterns are compiled regexps for detecting secrets.
var redactionPatterns []*regexp.Regexp

func init() {
	patterns := []string{
		// OpenAI API keys
		`sk-[A-Za-z0-9_-]{20,}`,
		// OpenAI project keys
		`sk-proj-[A-Za-z0-9_-]{20,}`,
		// Anthropic API keys
		`sk-ant-[A-Za-z0-9_-]{20,}`,
		// GitHub personal access tokens
		`ghp_[A-Za-z0-9]{36,}`,
		// GitHub OAuth tokens
		`gho_[A-Za-z0-9]{36,}`,
		// GitHub user-to-server tokens
		`ghu_[A-Za-z0-9]{36,}`,
		// GitHub server-to-server tokens
		`ghs_[A-Za-z0-9]{36,}`,
		// GitHub refresh tokens
		`ghr_[A-Za-z0-9]{36,}`,
		// GitHub fine-grained tokens
		`github_pat_[A-Za-z0-9_]{22,}`,
		// Slack tokens
		`xoxb-[A-Za-z0-9-]+`,
		`xoxp-[A-Za-z0-9-]+`,
		`xapp-[A-Za-z0-9-]+`,
		`xoxa-[A-Za-z0-9-]+`,
		// Bearer tokens in headers
		`(?i)Bearer\s+[A-Za-z0-9_.~+/=-]{20,}`,
		// x-api-key headers
		`(?i)x-api-key:\s*[A-Za-z0-9_.~+/=-]{10,}`,
		// Authorization headers
		`(?i)Authorization:\s*[A-Za-z0-9_.~+/ =-]{10,}`,
		// AWS access key IDs
		`AKIA[A-Z0-9]{16}`,
		// Generic long hex/base64 secrets that look like API keys
		// (40+ char hex strings, common in many providers)
		`(?i)(?:api[_-]?key|secret|token|password)\s*[:=]\s*["']?[A-Za-z0-9_.~+/=-]{20,}["']?`,
	}

	for _, p := range patterns {
		redactionPatterns = append(redactionPatterns, regexp.MustCompile(p))
	}
}

// RedactSecrets removes API keys and tokens from text, replacing them
// with [REDACTED]. This is used before logging or displaying tool results
// to prevent accidental credential leakage.
func RedactSecrets(text string) string {
	if text == "" {
		return text
	}

	result := text
	for _, re := range redactionPatterns {
		result = re.ReplaceAllStringFunc(result, func(match string) string {
			// For header-style matches, preserve the header name.
			if idx := strings.IndexByte(match, ':'); idx >= 0 {
				return match[:idx+1] + " [REDACTED]"
			}
			if idx := strings.IndexByte(match, '='); idx >= 0 {
				return match[:idx+1] + " [REDACTED]"
			}
			if strings.HasPrefix(strings.ToLower(match), "bearer") {
				return "Bearer [REDACTED]"
			}
			return "[REDACTED]"
		})
	}

	return result
}

// ContainsSecret returns true if the text appears to contain a secret.
func ContainsSecret(text string) bool {
	for _, re := range redactionPatterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}
