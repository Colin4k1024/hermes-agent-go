package observability

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var pgxQueryDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "hermes_pgx_query_duration_seconds",
		Help:    "PostgreSQL query latency via pgx tracer.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
	},
	[]string{"sql_prefix"},
)

type pgxTracerStartKey struct{}

type pgxStartData struct {
	start     time.Time
	sqlPrefix string
}

// PGXTracer implements pgx.QueryTracer for distributed tracing and slow-query logging.
type PGXTracer struct{}

func (t *PGXTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	prefix := sqlPrefix(data.SQL)
	return context.WithValue(ctx, pgxTracerStartKey{}, &pgxStartData{
		start:     time.Now(),
		sqlPrefix: prefix,
	})
}

func (t *PGXTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	sd, ok := ctx.Value(pgxTracerStartKey{}).(*pgxStartData)
	if !ok || sd == nil {
		return
	}
	elapsed := time.Since(sd.start)
	pgxQueryDuration.WithLabelValues(sd.sqlPrefix).Observe(elapsed.Seconds())

	if elapsed > 500*time.Millisecond {
		ContextLogger(ctx).Warn("slow_query",
			"sql_prefix", sd.sqlPrefix,
			"latency_ms", elapsed.Milliseconds(),
			"err", data.Err,
		)
	}
}

func sqlPrefix(sql string) string {
	if len(sql) > 40 {
		return sql[:40]
	}
	return sql
}

var _ pgx.QueryTracer = (*PGXTracer)(nil)
