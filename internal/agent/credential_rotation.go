package agent

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// RotatingCredential wraps a Credential with exhaustion tracking for
// rate-limit-aware rotation across multiple API keys.
type RotatingCredential struct {
	Credential
	exhaustedAt time.Time
	retryAfter  time.Duration
}

// IsExhausted returns true if the credential is currently rate-limited.
func (rc *RotatingCredential) IsExhausted() bool {
	if rc.exhaustedAt.IsZero() {
		return false
	}
	return time.Since(rc.exhaustedAt) < rc.retryAfter
}

// ExhaustedUntil returns when the credential becomes available again.
func (rc *RotatingCredential) ExhaustedUntil() time.Time {
	return rc.exhaustedAt.Add(rc.retryAfter)
}

// MarkExhausted marks the credential as rate-limited for the given duration.
func (rc *RotatingCredential) MarkExhausted(d time.Duration) {
	rc.exhaustedAt = time.Now()
	rc.retryAfter = d
	if rc.retryAfter < 5*time.Second {
		rc.retryAfter = 5 * time.Second
	}
}

// Reset clears the exhaustion state.
func (rc *RotatingCredential) Reset() {
	rc.exhaustedAt = time.Time{}
	rc.retryAfter = 0
}

// CredentialRotator manages rotation across multiple API keys within a
// single provider, with exhaustion tracking and automatic recovery.
// It sits on top of CredentialPool which handles provider-level lookup.
type CredentialRotator struct {
	mu         sync.Mutex
	keys       []*RotatingCredential
	currentIdx int
}

// NewCredentialRotator creates a rotator from a list of credentials.
func NewCredentialRotator(creds []Credential) *CredentialRotator {
	keys := make([]*RotatingCredential, len(creds))
	for i, c := range creds {
		keys[i] = &RotatingCredential{Credential: c}
	}
	return &CredentialRotator{keys: keys}
}

// Rotate returns the next available (non-exhausted) credential.
// Returns an error if all credentials are exhausted.
func (r *CredentialRotator) Rotate() (*Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.keys) == 0 {
		return nil, fmt.Errorf("credential rotator is empty")
	}

	n := len(r.keys)
	for i := 0; i < n; i++ {
		idx := (r.currentIdx + i) % n
		rc := r.keys[idx]
		if !rc.IsExhausted() {
			r.currentIdx = (idx + 1) % n
			slog.Debug("credential rotated", "label", labelFromKey(rc.APIKey), "provider", rc.Provider)
			return &rc.Credential, nil
		}
	}

	// All exhausted — find soonest recovery.
	var soonest *RotatingCredential
	for _, rc := range r.keys {
		if soonest == nil || rc.ExhaustedUntil().Before(soonest.ExhaustedUntil()) {
			soonest = rc
		}
	}
	wait := time.Until(soonest.ExhaustedUntil())
	return nil, fmt.Errorf("all %d credentials exhausted, soonest recovery in %v", n, wait.Round(time.Second))
}

// MarkExhausted marks the credential with the given API key as rate-limited.
func (r *CredentialRotator) MarkExhausted(apiKey string, retryAfter time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rc := range r.keys {
		if rc.APIKey == apiKey {
			rc.MarkExhausted(retryAfter)
			slog.Info("credential marked exhausted",
				"label", labelFromKey(apiKey),
				"retry_after", retryAfter.Round(time.Second),
			)
			return
		}
	}
}

// Available returns the number of non-exhausted credentials.
func (r *CredentialRotator) Available() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, rc := range r.keys {
		if !rc.IsExhausted() {
			count++
		}
	}
	return count
}

// Size returns the total number of credentials.
func (r *CredentialRotator) Size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.keys)
}

// ResetAll clears exhaustion state on all credentials.
func (r *CredentialRotator) ResetAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rc := range r.keys {
		rc.Reset()
	}
}

// Status returns a summary of all credentials' states.
func (r *CredentialRotator) Status() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]map[string]any, len(r.keys))
	for i, rc := range r.keys {
		entry := map[string]any{
			"label":     labelFromKey(rc.APIKey),
			"provider":  rc.Provider,
			"exhausted": rc.IsExhausted(),
		}
		if rc.IsExhausted() {
			entry["retry_after"] = time.Until(rc.ExhaustedUntil()).Round(time.Second).String()
		}
		result[i] = entry
	}
	return result
}

// labelFromKey generates a short display label from an API key.
func labelFromKey(key string) string {
	if len(key) <= 8 {
		return "key-***"
	}
	return "key-" + key[len(key)-4:]
}

// ParseRetryAfter extracts a retry duration from a "Retry-After" header value.
// Supports delta-seconds ("60") and HTTP-date formats.
func ParseRetryAfter(value string) time.Duration {
	if value == "" {
		return 30 * time.Second
	}

	var seconds int
	if _, err := fmt.Sscanf(value, "%d", &seconds); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	for _, layout := range []string{
		time.RFC1123,
		time.RFC1123Z,
		time.RFC850,
		time.ANSIC,
	} {
		if t, err := time.Parse(layout, value); err == nil {
			d := time.Until(t)
			if d > 0 {
				return d
			}
		}
	}

	return 30 * time.Second
}
