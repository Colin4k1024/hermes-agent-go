package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// StaticTokenExtractor validates Bearer tokens against a fixed secret.
// Used for single-tenant / dev mode backward compatibility.
type StaticTokenExtractor struct {
	token    string
	tenantID string
}

func NewStaticTokenExtractor(token, tenantID string) *StaticTokenExtractor {
	return &StaticTokenExtractor{token: token, tenantID: tenantID}
}

func (s *StaticTokenExtractor) Extract(r *http.Request) (*AuthContext, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, nil
	}

	provided := strings.TrimPrefix(auth, "Bearer ")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
		return nil, nil // not this extractor's concern; let chain try other extractors
	}

	return &AuthContext{
		Identity:   "system:acp-admin",
		TenantID:   s.tenantID,
		Roles:      []string{"admin"},
		AuthMethod: "static_token",
	}, nil
}
