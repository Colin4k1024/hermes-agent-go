package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// PairingStore manages DM pairing and user authorization per platform.
type PairingStore struct {
	mu sync.RWMutex

	// allowedUsers maps platform -> set of allowed user IDs.
	// A "*" entry means public access for that platform.
	allowedUsers map[Platform]map[string]bool

	// pendingCodes maps pair code -> pairing request.
	pendingCodes map[string]*PairingRequest

	// codeExpiry controls how long a pairing code is valid.
	codeExpiry time.Duration
}

// PairingRequest represents a pending pairing code.
type PairingRequest struct {
	Code      string
	Platform  Platform
	CreatedAt time.Time
}

// NewPairingStore creates a new pairing store.
func NewPairingStore() *PairingStore {
	return &PairingStore{
		allowedUsers: make(map[Platform]map[string]bool),
		pendingCodes: make(map[string]*PairingRequest),
		codeExpiry:   10 * time.Minute,
	}
}

// LoadAllowedUsers loads allowed users from configuration.
// Expected format in config.yaml:
//
//	messaging:
//	  allowed_users:
//	    telegram: ["user123", "user456"]
//	    discord: ["*"]  # public access
func (p *PairingStore) LoadAllowedUsers(allowedCfg map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if allowedCfg == nil {
		return
	}

	for platformStr, users := range allowedCfg {
		platform := Platform(platformStr)
		userSet := make(map[string]bool)

		switch v := users.(type) {
		case []any:
			for _, u := range v {
				if s, ok := u.(string); ok {
					userSet[s] = true
				}
			}
		case []string:
			for _, s := range v {
				userSet[s] = true
			}
		case string:
			// Single user or wildcard.
			userSet[v] = true
		}

		if len(userSet) > 0 {
			p.allowedUsers[platform] = userSet
			slog.Info("Loaded allowed users", "platform", platform, "count", len(userSet))
		}
	}
}

// IsUserAllowed checks if a user is authorized for the given platform.
// Returns true if:
//   - The wildcard "*" is set for the platform (explicit open access)
//   - The user's ID matches an allowed entry
//   - The user was paired via /pair command
//
// Returns false if no restrictions are configured (deny-by-default).
// Use "*" wildcard to explicitly allow open access.
func (p *PairingStore) IsUserAllowed(platform Platform, userID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	platformUsers, exists := p.allowedUsers[platform]
	if !exists {
		// Deny by default when no allowed_users configured for this platform.
		// To allow open access, set allowed_users: { platform: ["*"] } in config.
		slog.Warn("No allowed_users configured for platform, denying access. "+
			"Set allowed_users with \"*\" for open access.",
			"platform", platform, "user_id", userID)
		return false
	}

	// Check wildcard.
	if platformUsers["*"] {
		return true
	}

	// Check exact user ID match.
	if platformUsers[userID] {
		return true
	}

	// Check case-insensitive match.
	for allowed := range platformUsers {
		if strings.EqualFold(allowed, userID) {
			return true
		}
	}

	return false
}

// PairUser pairs a user to a platform using a pairing code.
func (p *PairingStore) PairUser(platform Platform, userID string, pairCode string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Validate the pairing code.
	request, exists := p.pendingCodes[pairCode]
	if !exists {
		return fmt.Errorf("invalid pairing code")
	}

	// Check expiry.
	if time.Since(request.CreatedAt) > p.codeExpiry {
		delete(p.pendingCodes, pairCode)
		return fmt.Errorf("pairing code has expired")
	}

	// Check platform matches.
	if request.Platform != "" && request.Platform != platform {
		return fmt.Errorf("pairing code is for platform %s, not %s", request.Platform, platform)
	}

	// Add user to allowed list.
	if p.allowedUsers[platform] == nil {
		p.allowedUsers[platform] = make(map[string]bool)
	}
	p.allowedUsers[platform][userID] = true

	// Remove used code.
	delete(p.pendingCodes, pairCode)

	slog.Info("User paired successfully", "platform", platform, "user_id", userID)
	return nil
}

// GeneratePairCode generates a new pairing code for a platform.
func (p *PairingStore) GeneratePairCode() string {
	return p.GeneratePairCodeForPlatform("")
}

// GeneratePairCodeForPlatform generates a pairing code optionally bound to a platform.
func (p *PairingStore) GeneratePairCodeForPlatform(platform Platform) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Clean up expired codes.
	p.cleanupExpiredCodes()

	// Generate a random 6-byte hex code.
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based code.
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	code := hex.EncodeToString(b)

	p.pendingCodes[code] = &PairingRequest{
		Code:      code,
		Platform:  platform,
		CreatedAt: time.Now(),
	}

	return code
}

// AddAllowedUser directly adds a user to the allowed list without a pairing code.
func (p *PairingStore) AddAllowedUser(platform Platform, userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.allowedUsers[platform] == nil {
		p.allowedUsers[platform] = make(map[string]bool)
	}
	p.allowedUsers[platform][userID] = true
}

// RemoveAllowedUser removes a user from the allowed list.
func (p *PairingStore) RemoveAllowedUser(platform Platform, userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if users, ok := p.allowedUsers[platform]; ok {
		delete(users, userID)
	}
}

// ListAllowedUsers returns the list of allowed user IDs for a platform.
func (p *PairingStore) ListAllowedUsers(platform Platform) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	users, ok := p.allowedUsers[platform]
	if !ok {
		return nil
	}

	result := make([]string, 0, len(users))
	for u := range users {
		result = append(result, u)
	}
	return result
}

// cleanupExpiredCodes removes expired pairing codes. Must be called with lock held.
func (p *PairingStore) cleanupExpiredCodes() {
	for code, req := range p.pendingCodes {
		if time.Since(req.CreatedAt) > p.codeExpiry {
			delete(p.pendingCodes, code)
		}
	}
}
