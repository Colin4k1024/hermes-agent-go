package agent

import (
	"encoding/base64"
	"net/url"
	"regexp"
	"strings"
)

// ExfilAttempt represents a detected exfiltration attempt.
type ExfilAttempt struct {
	Type    string // "url_token", "base64_secret", "prompt_injection", "data_uri"
	Pattern string
	Match   string
}

var (
	urlPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)
	b64Pattern = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
	dataURIPat = regexp.MustCompile(`data:[^;]+;base64,[A-Za-z0-9+/=]{20,}`)

	secretPrefixes = []string{
		"sk-", "sk-ant-", "ghp_", "gho_", "ghu_", "ghs_",
		"AKIA", "xoxb-", "xoxp-", "xoxo-", "xoxs-",
		"AIza", "ya29.", "fal_", "hf_", "gsk_", "pplx-", "r8_",
		"glpat-", "pypi-", "npm_", "sq0atp-", "rk_live_", "sk_live_",
	}

	injectionPatterns = []string{
		"ignore previous instructions",
		"ignore all previous",
		"disregard your instructions",
		"new instructions:",
		"system prompt:",
		"reveal your system",
		"what are your instructions",
		"output your prompt",
		"print your system message",
	}
)

// ScanForExfiltration detects potential secret exfiltration in text.
func ScanForExfiltration(text string) []ExfilAttempt {
	var attempts []ExfilAttempt

	// Check URLs for embedded tokens
	for _, u := range urlPattern.FindAllString(text, 20) {
		if a := checkURLForSecrets(u); a != nil {
			attempts = append(attempts, *a)
		}
	}

	// Check base64-encoded secrets
	for _, b := range b64Pattern.FindAllString(text, 10) {
		if a := checkBase64ForSecrets(b); a != nil {
			attempts = append(attempts, *a)
		}
	}

	// Check data URIs
	for _, d := range dataURIPat.FindAllString(text, 5) {
		attempts = append(attempts, ExfilAttempt{
			Type:    "data_uri",
			Pattern: "data:*;base64,*",
			Match:   truncateExfil(d, 80),
		})
	}

	// Check prompt injection patterns
	lower := strings.ToLower(text)
	for _, pat := range injectionPatterns {
		if strings.Contains(lower, pat) {
			attempts = append(attempts, ExfilAttempt{
				Type:    "prompt_injection",
				Pattern: pat,
				Match:   pat,
			})
		}
	}

	return attempts
}

func checkURLForSecrets(rawURL string) *ExfilAttempt {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	combined := parsed.RawQuery + parsed.Fragment + parsed.Path
	for _, prefix := range secretPrefixes {
		if strings.Contains(combined, prefix) {
			return &ExfilAttempt{
				Type:    "url_token",
				Pattern: prefix,
				Match:   truncateExfil(rawURL, 100),
			}
		}
	}
	return nil
}

func checkBase64ForSecrets(encoded string) *ExfilAttempt {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil
		}
	}

	text := string(decoded)
	for _, prefix := range secretPrefixes {
		if strings.Contains(text, prefix) {
			return &ExfilAttempt{
				Type:    "base64_secret",
				Pattern: prefix,
				Match:   truncateExfil(encoded, 60),
			}
		}
	}
	return nil
}

func truncateExfil(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
