package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/observability"
)

// RateLimiter checks whether a request should be allowed.
type RateLimiter interface {
	Allow(key string, limit int) (bool, int, error) // allowed, remaining, error
}

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	Limiter        RateLimiter
	DefaultRPM     int
	TenantLimitFn  func(tenantID string) int // optional: per-tenant override
}

// RateLimitMiddleware applies per-tenant rate limiting.
// Falls back to local in-memory limiter if the distributed limiter errors.
func RateLimitMiddleware(cfg RateLimitConfig) Middleware {
	local := newLocalLimiter()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var key string
			ac, ok := auth.FromContext(r.Context())
			if ok && ac != nil {
				key = "rl:" + ac.TenantID
			} else {
				ip := r.RemoteAddr
				if idx := strings.LastIndex(ip, ":"); idx != -1 {
					ip = ip[:idx]
				}
				key = "rl:anon:" + ip
			}
			limit := cfg.DefaultRPM
			if cfg.TenantLimitFn != nil && ac != nil {
				if tl := cfg.TenantLimitFn(ac.TenantID); tl > 0 {
					limit = tl
				}
			}

			var allowed bool
			var remaining int

			if cfg.Limiter != nil {
				var err error
				allowed, remaining, err = cfg.Limiter.Allow(key, limit)
				if err != nil {
					observability.ContextLogger(r.Context()).Warn("distributed rate limiter failed, falling back to local", "error", err)
					allowed, remaining = local.allow(key, limit)
				}
			} else {
				allowed, remaining = local.allow(key, limit)
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if !allowed {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// localLimiter is a simple in-memory sliding window fallback with bounded size.
type localLimiter struct {
	mu      sync.Mutex
	buckets *lru.Cache[string, *bucket]
}

type bucket struct {
	count    int
	limit    int
	windowAt time.Time
}

const maxRateLimitBuckets = 10000

func newLocalLimiter() *localLimiter {
	cache, _ := lru.New[string, *bucket](maxRateLimitBuckets)
	return &localLimiter{buckets: cache}
}

func (l *localLimiter) allow(key string, limit int) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets.Get(key)
	if !ok || now.Sub(b.windowAt) >= time.Minute {
		l.buckets.Add(key, &bucket{count: 1, limit: limit, windowAt: now})
		return true, limit - 1
	}

	if b.count >= limit {
		return false, 0
	}

	b.count++
	return true, limit - b.count
}
