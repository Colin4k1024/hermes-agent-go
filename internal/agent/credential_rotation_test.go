package agent

import (
	"testing"
	"time"
)

func TestRotatingCredential_Exhaustion(t *testing.T) {
	rc := &RotatingCredential{
		Credential: Credential{APIKey: "sk-test", Provider: "openai"},
	}

	if rc.IsExhausted() {
		t.Error("new credential should not be exhausted")
	}

	rc.MarkExhausted(10 * time.Second)
	if !rc.IsExhausted() {
		t.Error("marked credential should be exhausted")
	}

	rc.Reset()
	if rc.IsExhausted() {
		t.Error("reset credential should not be exhausted")
	}
}

func TestRotatingCredential_MinimumCooldown(t *testing.T) {
	rc := &RotatingCredential{Credential: Credential{APIKey: "sk-test"}}
	rc.MarkExhausted(1 * time.Second)
	if rc.retryAfter < 5*time.Second {
		t.Errorf("retryAfter = %v, want >= 5s", rc.retryAfter)
	}
}

func TestCredentialRotator_Rotate(t *testing.T) {
	rotator := NewCredentialRotator([]Credential{
		{APIKey: "key-1", Provider: "openai"},
		{APIKey: "key-2", Provider: "openai"},
		{APIKey: "key-3", Provider: "openai"},
	})

	seen := make(map[string]bool)
	for i := 0; i < 3; i++ {
		cred, err := rotator.Rotate()
		if err != nil {
			t.Fatalf("rotate %d: %v", i, err)
		}
		seen[cred.APIKey] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 unique keys, got %d", len(seen))
	}
}

func TestCredentialRotator_SkipsExhausted(t *testing.T) {
	rotator := NewCredentialRotator([]Credential{
		{APIKey: "key-1", Provider: "openai"},
		{APIKey: "key-2", Provider: "openai"},
	})

	rotator.MarkExhausted("key-1", 1*time.Minute)

	cred, err := rotator.Rotate()
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if cred.APIKey != "key-2" {
		t.Errorf("expected key-2, got %s", cred.APIKey)
	}
}

func TestCredentialRotator_AllExhausted(t *testing.T) {
	rotator := NewCredentialRotator([]Credential{
		{APIKey: "key-1", Provider: "openai"},
		{APIKey: "key-2", Provider: "openai"},
	})

	rotator.MarkExhausted("key-1", 1*time.Minute)
	rotator.MarkExhausted("key-2", 2*time.Minute)

	_, err := rotator.Rotate()
	if err == nil {
		t.Error("expected error when all exhausted")
	}
}

func TestCredentialRotator_Empty(t *testing.T) {
	rotator := NewCredentialRotator(nil)
	_, err := rotator.Rotate()
	if err == nil {
		t.Error("expected error for empty rotator")
	}
}

func TestCredentialRotator_SizeAndAvailable(t *testing.T) {
	rotator := NewCredentialRotator([]Credential{
		{APIKey: "key-1", Provider: "openai"},
		{APIKey: "key-2", Provider: "openai"},
	})

	if rotator.Size() != 2 {
		t.Errorf("Size() = %d, want 2", rotator.Size())
	}
	if rotator.Available() != 2 {
		t.Errorf("Available() = %d, want 2", rotator.Available())
	}

	rotator.MarkExhausted("key-1", 1*time.Minute)
	if rotator.Available() != 1 {
		t.Errorf("Available() = %d, want 1", rotator.Available())
	}
}

func TestCredentialRotator_ResetAll(t *testing.T) {
	rotator := NewCredentialRotator([]Credential{
		{APIKey: "key-1", Provider: "openai"},
		{APIKey: "key-2", Provider: "openai"},
	})

	rotator.MarkExhausted("key-1", 1*time.Minute)
	rotator.MarkExhausted("key-2", 1*time.Minute)
	rotator.ResetAll()

	if rotator.Available() != 2 {
		t.Errorf("Available() = %d, want 2", rotator.Available())
	}
}

func TestCredentialRotator_Status(t *testing.T) {
	rotator := NewCredentialRotator([]Credential{
		{APIKey: "sk-test-1234567890", Provider: "openai"},
	})

	status := rotator.Status()
	if len(status) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(status))
	}
	if status[0]["label"] != "key-7890" {
		t.Errorf("label = %v, want 'key-7890'", status[0]["label"])
	}
}

func TestLabelFromKey(t *testing.T) {
	tests := []struct {
		key, want string
	}{
		{"sk-test-1234567890", "key-7890"},
		{"short", "key-***"},
		{"12345678", "key-***"},
		{"123456789", "key-6789"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := labelFromKey(tt.key)
			if got != tt.want {
				t.Errorf("labelFromKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"empty", "", 30 * time.Second},
		{"seconds", "60", 60 * time.Second},
		{"small", "5", 5 * time.Second},
		{"invalid", "not-a-number", 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRetryAfter(tt.value)
			if got != tt.want {
				t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
