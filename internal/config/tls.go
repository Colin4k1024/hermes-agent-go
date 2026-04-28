package config

import (
	"crypto/tls"
	"fmt"
)

// TLSConfig holds TLS-related configuration.
type TLSConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	CertFile   string `json:"cert_file" yaml:"cert_file"`
	KeyFile    string `json:"key_file" yaml:"key_file"`
	MinVersion string `json:"min_version" yaml:"min_version"` // "1.2" or "1.3"
}

// Build creates a *tls.Config from the settings.
func (c *TLSConfig) Build() (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}
	if c.CertFile == "" || c.KeyFile == "" {
		return nil, fmt.Errorf("tls enabled but cert_file or key_file missing")
	}

	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load tls keypair: %w", err)
	}

	minVer := uint16(tls.VersionTLS12)
	if c.MinVersion == "1.3" {
		minVer = tls.VersionTLS13
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   minVer,
	}, nil
}
