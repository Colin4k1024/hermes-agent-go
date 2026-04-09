package cli

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// KawaiiSpinner provides an animated spinner with kawaii faces and thinking verbs.
type KawaiiSpinner struct {
	mu       sync.Mutex
	active   bool
	stopCh   chan struct{}
	message  string
	toolName string

	// Customizable faces and verbs (loaded from skin).
	waitingFaces  []string
	thinkingFaces []string
	thinkingVerbs []string
	wings         [][2]string

	// Output callback.
	onFrame func(frame string)
}

// Default kawaii faces.
var defaultWaitingFaces = []string{
	"(>_<)", "(^_^)", "(o_o)", "(-_-)", "(._.).",
	"(>.<)", "(*_*)", "(~_~)", "(._.).", "(^o^)",
}

var defaultThinkingFaces = []string{
	"(>_<)", "(^_^)", "(o_o)", "(-_-)", "(._.)",
	"(>.<)", "(*_*)", "(~_~)", "(._.)", "(^o^)",
}

var defaultThinkingVerbs = []string{
	"pondering", "considering", "reflecting", "contemplating",
	"musing", "analyzing", "processing", "thinking",
	"deliberating", "evaluating", "examining", "studying",
	"weighing", "assessing", "meditating", "cogitating",
}

// NewKawaiiSpinner creates a new spinner with optional skin customization.
func NewKawaiiSpinner(skin *SkinConfig, onFrame func(frame string)) *KawaiiSpinner {
	s := &KawaiiSpinner{
		waitingFaces:  defaultWaitingFaces,
		thinkingFaces: defaultThinkingFaces,
		thinkingVerbs: defaultThinkingVerbs,
		onFrame:       onFrame,
	}

	if skin != nil {
		if len(skin.Spinner.WaitingFaces) > 0 {
			s.waitingFaces = skin.Spinner.WaitingFaces
		}
		if len(skin.Spinner.ThinkingFaces) > 0 {
			s.thinkingFaces = skin.Spinner.ThinkingFaces
		}
		if len(skin.Spinner.ThinkingVerbs) > 0 {
			s.thinkingVerbs = skin.Spinner.ThinkingVerbs
		}
		s.wings = skin.GetWings()
	}

	return s
}

// Start begins the spinner animation.
func (s *KawaiiSpinner) Start(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		return
	}

	s.active = true
	s.message = message
	s.stopCh = make(chan struct{})

	go s.animate()
}

// Stop stops the spinner animation.
func (s *KawaiiSpinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return
	}

	s.active = false
	close(s.stopCh)
}

// SetMessage updates the spinner message.
func (s *KawaiiSpinner) SetMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// SetToolName sets the tool currently being executed.
func (s *KawaiiSpinner) SetToolName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolName = name
}

// ToolProgress displays a tool progress message.
func (s *KawaiiSpinner) ToolProgress(toolName, preview string) string {
	prefix := s.getToolPrefix()
	if preview != "" {
		return fmt.Sprintf("%s %s: %s", prefix, toolName, preview)
	}
	return fmt.Sprintf("%s %s", prefix, toolName)
}

func (s *KawaiiSpinner) getToolPrefix() string {
	return "|"
}

func (s *KawaiiSpinner) animate() {
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	frameIdx := 0
	verbIdx := rand.Intn(len(s.thinkingVerbs))

	for {
		select {
		case <-s.stopCh:
			// Clear the spinner line.
			if s.onFrame != nil {
				s.onFrame("")
			}
			return
		case <-ticker.C:
			s.mu.Lock()
			msg := s.message
			s.mu.Unlock()

			face := s.waitingFaces[frameIdx%len(s.waitingFaces)]
			verb := s.thinkingVerbs[verbIdx%len(s.thinkingVerbs)]

			var frame string
			if len(s.wings) > 0 {
				wingPair := s.wings[frameIdx%len(s.wings)]
				frame = fmt.Sprintf(" %s %s %s %s...", wingPair[0], face, wingPair[1], verb)
			} else {
				frame = fmt.Sprintf(" %s %s...", face, verb)
			}

			if msg != "" {
				frame = fmt.Sprintf(" %s %s", face, msg)
			}

			if s.onFrame != nil {
				s.onFrame(frame)
			}

			frameIdx++
			if frameIdx%3 == 0 {
				verbIdx++
			}
		}
	}
}

// FormatToolStart returns a formatted tool start message.
func FormatToolStart(toolName string) string {
	return fmt.Sprintf("| %s", toolName)
}

// FormatToolProgress returns a formatted tool progress message with preview.
func FormatToolProgress(toolName, argsPreview string) string {
	if argsPreview == "" {
		return fmt.Sprintf("| %s", toolName)
	}
	// Truncate preview.
	if len(argsPreview) > 80 {
		argsPreview = argsPreview[:77] + "..."
	}
	return fmt.Sprintf("| %s: %s", toolName, argsPreview)
}

// FormatToolComplete returns a formatted tool completion message.
func FormatToolComplete(toolName string) string {
	return fmt.Sprintf("| %s done", toolName)
}

// RandomThinkingVerb returns a random thinking verb for display.
func RandomThinkingVerb() string {
	return defaultThinkingVerbs[rand.Intn(len(defaultThinkingVerbs))]
}

// RandomFace returns a random kawaii face.
func RandomFace() string {
	return defaultWaitingFaces[rand.Intn(len(defaultWaitingFaces))]
}

// FormatSpinnerFrame builds a single spinner frame string.
func FormatSpinnerFrame(face, verb string) string {
	return fmt.Sprintf(" %s %s...", face, verb)
}

// EllipsisAnimation returns progressively longer ellipsis strings.
func EllipsisAnimation(step int) string {
	dots := (step % 3) + 1
	return strings.Repeat(".", dots)
}
