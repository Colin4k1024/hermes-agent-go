package tools

import (
	"testing"
)

func TestSelectBackend_Explicit(t *testing.T) {
	tests := []struct {
		name       string
		envBackend string
		envAPIKey  string
		envCDPURL  string
		wantName   string
		wantErr    bool
	}{
		{
			name:       "explicit browserbase with key",
			envBackend: "browserbase",
			envAPIKey:  "test-key",
			wantName:   "browserbase",
		},
		{
			name:       "explicit browserbase without key",
			envBackend: "browserbase",
			envAPIKey:  "",
			wantErr:    true,
		},
		{
			name:       "explicit local",
			envBackend: "local",
			wantName:   "local",
		},
		{
			name:       "unknown backend",
			envBackend: "firefox",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BROWSER_BACKEND", tt.envBackend)
			if tt.envAPIKey != "" {
				t.Setenv("BROWSERBASE_API_KEY", tt.envAPIKey)
			} else {
				t.Setenv("BROWSERBASE_API_KEY", "")
			}
			if tt.envCDPURL != "" {
				t.Setenv("BROWSER_CDP_URL", tt.envCDPURL)
			}

			backend, err := selectBackend()
			if (err != nil) != tt.wantErr {
				t.Errorf("selectBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && backend.Name() != tt.wantName {
				t.Errorf("selectBackend().Name() = %q, want %q", backend.Name(), tt.wantName)
			}
		})
	}
}

func TestSelectBackend_AutoDetect(t *testing.T) {
	tests := []struct {
		name      string
		envAPIKey string
		envCDPURL string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "auto detect browserbase",
			envAPIKey: "test-key",
			wantName:  "browserbase",
		},
		{
			name:      "auto detect local via CDP URL",
			envCDPURL: "http://127.0.0.1:9222",
			wantName:  "local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BROWSER_BACKEND", "")
			if tt.envAPIKey != "" {
				t.Setenv("BROWSERBASE_API_KEY", tt.envAPIKey)
			} else {
				t.Setenv("BROWSERBASE_API_KEY", "")
			}
			if tt.envCDPURL != "" {
				t.Setenv("BROWSER_CDP_URL", tt.envCDPURL)
			} else {
				t.Setenv("BROWSER_CDP_URL", "")
			}

			backend, err := selectBackend()
			if (err != nil) != tt.wantErr {
				t.Errorf("selectBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && backend.Name() != tt.wantName {
				t.Errorf("selectBackend().Name() = %q, want %q", backend.Name(), tt.wantName)
			}
		})
	}
}

func TestBrowserBackendInterface(t *testing.T) {
	// Compile-time checks (also verified by var _ lines in source).
	var _ BrowserBackend = (*BrowserbaseBackend)(nil)
	var _ BrowserBackend = (*LocalBrowserBackend)(nil)
}

func TestLocalBrowserBackend_Name(t *testing.T) {
	b := &LocalBrowserBackend{}
	if b.Name() != "local" {
		t.Errorf("Name() = %q, want \"local\"", b.Name())
	}
}

func TestBrowserbaseBackend_Name(t *testing.T) {
	b := &BrowserbaseBackend{}
	if b.Name() != "browserbase" {
		t.Errorf("Name() = %q, want \"browserbase\"", b.Name())
	}
}

func TestFindChromeBinary(t *testing.T) {
	// This test is environment-dependent; just verify it doesn't panic.
	_ = findChromeBinary()
}
