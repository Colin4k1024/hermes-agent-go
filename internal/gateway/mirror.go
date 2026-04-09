package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// MirrorDirection represents the direction of message mirroring.
type MirrorDirection string

const (
	MirrorOneWay        MirrorDirection = "one-way"
	MirrorBidirectional MirrorDirection = "bidirectional"
)

// MirrorRule defines a single message mirroring rule.
type MirrorRule struct {
	SourcePlatform string          `yaml:"source_platform" json:"source_platform"`
	SourceChat     string          `yaml:"source_chat" json:"source_chat"`
	DestPlatform   string          `yaml:"dest_platform" json:"dest_platform"`
	DestChat       string          `yaml:"dest_chat" json:"dest_chat"`
	Direction      MirrorDirection `yaml:"direction" json:"direction"`
}

// MessageMirror manages message mirroring across platforms.
type MessageMirror struct {
	mu       sync.RWMutex
	rules    []MirrorRule
	adapters map[string]PlatformAdapter
}

// NewMessageMirror creates a new MessageMirror with no rules or adapters.
func NewMessageMirror() *MessageMirror {
	return &MessageMirror{
		adapters: make(map[string]PlatformAdapter),
	}
}

// RegisterAdapter adds a platform adapter for message delivery.
func (m *MessageMirror) RegisterAdapter(platform string, adapter PlatformAdapter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapters[platform] = adapter
}

// LoadRules parses mirror rules from a configuration map.
// Expected structure:
//
//	mirrors:
//	  - source_platform: telegram
//	    source_chat: "-100123456"
//	    dest_platform: discord
//	    dest_chat: "987654321"
//	    direction: "one-way"
func (m *MessageMirror) LoadRules(cfg map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rules = nil

	mirrorsRaw, ok := cfg["mirrors"]
	if !ok {
		return
	}

	mirrorList, ok := mirrorsRaw.([]any)
	if !ok {
		return
	}

	for _, item := range mirrorList {
		ruleMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		rule := MirrorRule{
			Direction: MirrorOneWay, // default
		}

		if v, ok := ruleMap["source_platform"].(string); ok {
			rule.SourcePlatform = v
		}
		if v, ok := ruleMap["source_chat"].(string); ok {
			rule.SourceChat = v
		}
		if v, ok := ruleMap["dest_platform"].(string); ok {
			rule.DestPlatform = v
		}
		if v, ok := ruleMap["dest_chat"].(string); ok {
			rule.DestChat = v
		}
		if v, ok := ruleMap["direction"].(string); ok {
			switch strings.ToLower(v) {
			case "bidirectional", "bidi", "two-way":
				rule.Direction = MirrorBidirectional
			default:
				rule.Direction = MirrorOneWay
			}
		}

		if rule.SourcePlatform != "" && rule.DestPlatform != "" {
			m.rules = append(m.rules, rule)
		}
	}

	slog.Info("Loaded mirror rules", "count", len(m.rules))
}

// ShouldMirror returns all mirror rules that match the given message source.
// For bidirectional rules, both source->dest and dest->source matches are returned.
func (m *MessageMirror) ShouldMirror(source SessionSource) []MirrorRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matching []MirrorRule
	platform := string(source.Platform)
	chatID := source.ChatID

	for _, rule := range m.rules {
		// Check forward match: source -> dest.
		if rule.SourcePlatform == platform && matchChat(rule.SourceChat, chatID) {
			matching = append(matching, rule)
			continue
		}

		// Check reverse match for bidirectional rules: dest -> source.
		if rule.Direction == MirrorBidirectional {
			if rule.DestPlatform == platform && matchChat(rule.DestChat, chatID) {
				// Return a reversed rule.
				matching = append(matching, MirrorRule{
					SourcePlatform: rule.DestPlatform,
					SourceChat:     rule.DestChat,
					DestPlatform:   rule.SourcePlatform,
					DestChat:       rule.SourceChat,
					Direction:      MirrorBidirectional,
				})
			}
		}
	}

	return matching
}

// MirrorMessage sends a message to all destinations specified by the given rules.
// Returns the first error encountered, or nil if all deliveries succeeded.
func (m *MessageMirror) MirrorMessage(content string, rules []MirrorRule) error {
	m.mu.RLock()
	adapters := m.adapters
	m.mu.RUnlock()

	var firstErr error
	for _, rule := range rules {
		adapter, ok := adapters[rule.DestPlatform]
		if !ok {
			slog.Warn("Mirror: no adapter for destination platform",
				"dest_platform", rule.DestPlatform)
			if firstErr == nil {
				firstErr = fmt.Errorf("no adapter for platform %q", rule.DestPlatform)
			}
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), mirrorTimeout)
		result, err := adapter.Send(ctx, rule.DestChat, content, map[string]string{
			"mirrored_from": rule.SourcePlatform,
		})
		cancel()

		if err != nil {
			slog.Warn("Mirror: send failed",
				"dest_platform", rule.DestPlatform,
				"dest_chat", rule.DestChat,
				"error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		if result != nil && !result.Success {
			slog.Warn("Mirror: send returned failure",
				"dest_platform", rule.DestPlatform,
				"dest_chat", rule.DestChat,
				"error", result.Error)
			if firstErr == nil {
				firstErr = fmt.Errorf("mirror send failed: %s", result.Error)
			}
		}
	}

	return firstErr
}

// Rules returns a copy of the currently loaded rules.
func (m *MessageMirror) Rules() []MirrorRule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MirrorRule, len(m.rules))
	copy(result, m.rules)
	return result
}

// matchChat checks whether a rule's chat pattern matches a concrete chatID.
// Supports "*" as a wildcard to match any chat.
func matchChat(pattern, chatID string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	return pattern == chatID
}

// mirrorTimeout is the maximum time to wait for a single mirrored delivery.
const mirrorTimeout = 15 * time.Second
