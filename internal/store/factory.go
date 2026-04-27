package store

import (
	"context"
	"fmt"
	"log/slog"
)

// StoreConfig holds database configuration.
type StoreConfig struct {
	Driver string // "postgres" or "sqlite" (default)
	URL    string // connection URL
}

// NewFunc is a factory function type for creating a Store.
type NewFunc func(ctx context.Context, cfg StoreConfig) (Store, error)

var registry = map[string]NewFunc{}

// RegisterDriver registers a store implementation by driver name.
func RegisterDriver(name string, fn NewFunc) {
	registry[name] = fn
}

// NewStore creates a Store based on configuration.
func NewStore(ctx context.Context, cfg StoreConfig) (Store, error) {
	driver := cfg.Driver
	if driver == "" {
		driver = "sqlite"
	}

	fn, ok := registry[driver]
	if !ok {
		return nil, fmt.Errorf("unknown store driver: %s (registered: %v)", driver, registeredDrivers())
	}

	slog.Info("Initializing store", "driver", driver)
	s, err := fn(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store init (%s): %w", driver, err)
	}

	if err := s.Migrate(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("store migrate (%s): %w", driver, err)
	}

	return s, nil
}

func registeredDrivers() []string {
	var names []string
	for k := range registry {
		names = append(names, k)
	}
	return names
}
