package llm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sony/gobreaker/v2"
)

// ResilientTransport wraps a Transport with per-model circuit breaker protection.
type ResilientTransport struct {
	inner   Transport
	breaker *gobreaker.CircuitBreaker[*ChatResponse]
}

// NewResilientTransport wraps a transport with circuit breaker.
func NewResilientTransport(inner Transport, model string) *ResilientTransport {
	settings := gobreaker.Settings{
		Name:        "llm-" + model,
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5 ||
				(counts.Requests > 10 && counts.TotalFailures*2 > counts.Requests)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("circuit_breaker_state_change", "breaker", name, "from", from.String(), "to", to.String())
		},
	}

	return &ResilientTransport{
		inner:   inner,
		breaker: gobreaker.NewCircuitBreaker[*ChatResponse](settings),
	}
}

func (r *ResilientTransport) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	resp, err := r.breaker.Execute(func() (*ChatResponse, error) {
		return r.inner.Chat(ctx, req)
	})
	if err != nil {
		return nil, fmt.Errorf("circuit breaker [%s]: %w", r.breaker.Name(), err)
	}
	return resp, nil
}

func (r *ResilientTransport) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamDelta, <-chan error) {
	state := r.breaker.State()
	if state == gobreaker.StateOpen {
		eCh := make(chan error, 1)
		eCh <- fmt.Errorf("circuit breaker open for %s", r.breaker.Name())
		close(eCh)
		dCh := make(chan StreamDelta)
		close(dCh)
		return dCh, eCh
	}
	return r.inner.ChatStream(ctx, req)
}

func (r *ResilientTransport) Name() string { return r.inner.Name() + "+breaker" }

var _ Transport = (*ResilientTransport)(nil)
