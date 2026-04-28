package observability

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var pgQueryDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "hermes_pg_query_duration_seconds",
		Help:    "PostgreSQL query latency in seconds.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
	},
	[]string{"operation"},
)

type pgQueryStartKey struct{}

// PGQueryStart records the start of a database query. Call PGQueryEnd when done.
func PGQueryStart(ctx context.Context, operation string) context.Context {
	return context.WithValue(ctx, pgQueryStartKey{}, &pgQueryState{
		start:     time.Now(),
		operation: operation,
	})
}

// PGQueryEnd records the end of a database query, logging slow queries.
func PGQueryEnd(ctx context.Context) {
	v, ok := ctx.Value(pgQueryStartKey{}).(*pgQueryState)
	if !ok || v == nil {
		return
	}
	elapsed := time.Since(v.start)
	pgQueryDuration.WithLabelValues(v.operation).Observe(elapsed.Seconds())
	if elapsed > 500*time.Millisecond {
		ContextLogger(ctx).Warn("slow_query",
			"operation", v.operation,
			"latency_ms", elapsed.Milliseconds(),
		)
	}
}

type pgQueryState struct {
	start     time.Time
	operation string
}
