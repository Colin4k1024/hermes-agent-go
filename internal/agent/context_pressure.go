package agent

import "log/slog"

// ContextPressureLevel represents the severity of context window pressure.
type ContextPressureLevel int

const (
	PressureNone     ContextPressureLevel = iota
	PressureLow                           // >50% used
	PressureMedium                        // >70% used
	PressureHigh                          // >85% used
	PressureCritical                      // >95% used
)

// CheckContextPressure evaluates token usage against the model's context window
// and returns the current pressure level.
func CheckContextPressure(usedTokens, contextLength int) ContextPressureLevel {
	if contextLength <= 0 {
		return PressureNone
	}

	ratio := float64(usedTokens) / float64(contextLength)

	switch {
	case ratio > 0.95:
		return PressureCritical
	case ratio > 0.85:
		return PressureHigh
	case ratio > 0.70:
		return PressureMedium
	case ratio > 0.50:
		return PressureLow
	default:
		return PressureNone
	}
}

// LogContextPressure logs a warning if context pressure is elevated.
// Returns true if any warning was emitted.
func LogContextPressure(usedTokens, contextLength int, sessionID string) bool {
	level := CheckContextPressure(usedTokens, contextLength)
	pct := 0
	if contextLength > 0 {
		pct = usedTokens * 100 / contextLength
	}

	switch level {
	case PressureCritical:
		slog.Warn("Context pressure CRITICAL — consider /compress", "used_pct", pct, "session", sessionID)
		return true
	case PressureHigh:
		slog.Warn("Context pressure HIGH", "used_pct", pct, "session", sessionID)
		return true
	case PressureMedium:
		slog.Info("Context pressure MEDIUM", "used_pct", pct, "session", sessionID)
		return true
	default:
		return false
	}
}
