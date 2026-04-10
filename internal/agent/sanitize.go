package agent

import (
	"regexp"
)

// surrogateRE matches lone surrogate code points (U+D800 through U+DFFF).
// These are invalid in UTF-8 and will crash json.Marshal.
var surrogateRE = regexp.MustCompile(`[\x{D800}-\x{DFFF}]`)

// SanitizeSurrogates replaces lone surrogate code points with U+FFFD
// (replacement character). This is a fast no-op when no surrogates are present.
func SanitizeSurrogates(text string) string {
	if !surrogateRE.MatchString(text) {
		return text
	}
	return surrogateRE.ReplaceAllString(text, "\uFFFD")
}
