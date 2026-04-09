package gateway

import (
	"sync"
)

// ChannelBinding maps a specific platform channel to a skill that should
// be automatically loaded for messages in that channel.
type ChannelBinding struct {
	// ChannelID is the platform-specific channel/chat identifier.
	ChannelID string `json:"channel_id" yaml:"channel_id"`

	// SkillName is the skill to auto-load for this channel.
	SkillName string `json:"skill_name" yaml:"skill_name"`

	// Platform restricts this binding to a specific platform.
	Platform string `json:"platform" yaml:"platform"`
}

// ChannelDirectory manages channel-to-skill bindings, allowing automatic
// skill loading based on which channel a message arrives in.
type ChannelDirectory struct {
	mu       sync.RWMutex
	bindings []ChannelBinding
}

// NewChannelDirectory creates an empty channel directory.
func NewChannelDirectory() *ChannelDirectory {
	return &ChannelDirectory{}
}

// LoadFromConfig populates the directory from a config map.
// Expected format:
//
//	channel_bindings:
//	  - channel_id: "C12345"
//	    skill_name: "customer_support"
//	    platform: "slack"
func (cd *ChannelDirectory) LoadFromConfig(cfg map[string]any) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	bindings, ok := cfg["channel_bindings"].([]any)
	if !ok {
		return
	}

	for _, item := range bindings {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}

		binding := ChannelBinding{
			ChannelID: stringFromMap(entry, "channel_id"),
			SkillName: stringFromMap(entry, "skill_name"),
			Platform:  stringFromMap(entry, "platform"),
		}

		if binding.ChannelID != "" && binding.SkillName != "" {
			cd.bindings = append(cd.bindings, binding)
		}
	}
}

// GetBinding returns the binding for a given platform and channel ID,
// or nil if no binding exists.
func (cd *ChannelDirectory) GetBinding(platform, channelID string) *ChannelBinding {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	for i := range cd.bindings {
		b := &cd.bindings[i]
		if b.ChannelID == channelID {
			// If the binding specifies a platform, it must match.
			if b.Platform != "" && b.Platform != platform {
				continue
			}
			return b
		}
	}
	return nil
}

// SetBinding adds or updates a channel binding.
func (cd *ChannelDirectory) SetBinding(platform, channelID, skillName string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	// Check for existing binding to update.
	for i := range cd.bindings {
		if cd.bindings[i].ChannelID == channelID && cd.bindings[i].Platform == platform {
			cd.bindings[i].SkillName = skillName
			return
		}
	}

	// Add new binding.
	cd.bindings = append(cd.bindings, ChannelBinding{
		ChannelID: channelID,
		SkillName: skillName,
		Platform:  platform,
	})
}

// RemoveBinding removes a channel binding.
func (cd *ChannelDirectory) RemoveBinding(platform, channelID string) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	for i := range cd.bindings {
		if cd.bindings[i].ChannelID == channelID &&
			(cd.bindings[i].Platform == "" || cd.bindings[i].Platform == platform) {
			cd.bindings = append(cd.bindings[:i], cd.bindings[i+1:]...)
			return true
		}
	}
	return false
}

// ListBindings returns all current bindings.
func (cd *ChannelDirectory) ListBindings() []ChannelBinding {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	result := make([]ChannelBinding, len(cd.bindings))
	copy(result, cd.bindings)
	return result
}

// stringFromMap safely extracts a string value from a map.
func stringFromMap(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
