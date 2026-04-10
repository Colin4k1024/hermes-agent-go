package platforms

import (
	"testing"
)

func TestFormatSlackMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bold",
			input:    "this is **bold** text",
			expected: "this is *bold* text",
		},
		{
			name:     "strikethrough",
			input:    "this is ~~deleted~~ text",
			expected: "this is ~deleted~ text",
		},
		{
			name:     "link",
			input:    "visit [Google](https://google.com)",
			expected: "visit <https://google.com|Google>",
		},
		{
			name:     "header",
			input:    "# Title\n## Subtitle",
			expected: "*Title*\n*Subtitle*",
		},
		{
			name:     "code block preserved",
			input:    "```go\nfunc main() {\n  **not bold**\n}\n```",
			expected: "```go\nfunc main() {\n  **not bold**\n}\n```",
		},
		{
			name:     "inline code preserved",
			input:    "use `**literal**` here",
			expected: "use `**literal**` here",
		},
		{
			name:     "plain text unchanged",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "mixed formatting",
			input:    "# Header\n**bold** and ~~strike~~ with [link](http://x.com)",
			expected: "*Header*\n*bold* and ~strike~ with <http://x.com|link>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSlackMessage(tt.input)
			if got != tt.expected {
				t.Errorf("FormatSlackMessage(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHandleApprovalAction(t *testing.T) {
	tests := []struct {
		name     string
		actionID string
		handled  bool
	}{
		{"approve once", "hermes_approve_once", true},
		{"approve session", "hermes_approve_session", true},
		{"approve always", "hermes_approve_always", true},
		{"deny", "hermes_deny", true},
		{"unknown", "some_other_action", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleApprovalAction(tt.actionID, "test-session")
			if got != tt.handled {
				t.Errorf("HandleApprovalAction(%q) = %v, want %v", tt.actionID, got, tt.handled)
			}
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{0, 0, 0},
	}
	for _, tt := range tests {
		if got := min(tt.a, tt.b); got != tt.want {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
