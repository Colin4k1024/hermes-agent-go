package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

type quotaSessionStore struct {
	count int
	err   error
}

func (s *quotaSessionStore) Create(_ context.Context, _ string, _ *store.Session) error { return nil }
func (s *quotaSessionStore) Get(_ context.Context, _, _ string) (*store.Session, error) {
	return nil, nil
}
func (s *quotaSessionStore) End(_ context.Context, _, _, _ string) error { return nil }
func (s *quotaSessionStore) List(_ context.Context, _ string, _ store.ListOptions) ([]*store.Session, int, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	return nil, s.count, nil
}
func (s *quotaSessionStore) Delete(_ context.Context, _, _ string) error { return nil }
func (s *quotaSessionStore) UpdateTokens(_ context.Context, _, _ string, _ store.TokenDelta) error {
	return nil
}
func (s *quotaSessionStore) SetTitle(_ context.Context, _, _, _ string) error { return nil }

func TestQuotaMiddleware(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		maxFn      func(string) int
		count      int
		storeErr   error
		withAuth   bool
		tenantID   string
		wantStatus int
	}{
		{
			name:       "nil MaxSessionsFn passes through",
			maxFn:      nil,
			withAuth:   true,
			tenantID:   "t1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "no auth context passes through",
			maxFn:      func(_ string) int { return 10 },
			withAuth:   false,
			wantStatus: http.StatusOK,
		},
		{
			name:       "under limit passes through",
			maxFn:      func(_ string) int { return 10 },
			count:      5,
			withAuth:   true,
			tenantID:   "t1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "at limit returns 429",
			maxFn:      func(_ string) int { return 5 },
			count:      5,
			withAuth:   true,
			tenantID:   "t1",
			wantStatus: http.StatusTooManyRequests,
		},
		{
			name:       "over limit returns 429",
			maxFn:      func(_ string) int { return 3 },
			count:      5,
			withAuth:   true,
			tenantID:   "t1",
			wantStatus: http.StatusTooManyRequests,
		},
		{
			name:       "zero max passes through",
			maxFn:      func(_ string) int { return 0 },
			withAuth:   true,
			tenantID:   "t1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "store error returns 500",
			maxFn:      func(_ string) int { return 10 },
			storeErr:   context.DeadlineExceeded,
			withAuth:   true,
			tenantID:   "t1",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := &quotaSessionStore{count: tt.count, err: tt.storeErr}
			mw := QuotaMiddleware(QuotaConfig{
				Sessions:      ss,
				MaxSessionsFn: tt.maxFn,
			})

			handler := mw(ok)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			if tt.withAuth {
				ctx := auth.WithContext(req.Context(), &auth.AuthContext{TenantID: tt.tenantID})
				req = req.WithContext(ctx)
			}

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
