package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hermes_http_requests_total",
			Help: "Total HTTP requests by method, path, status code, and tenant.",
		},
		[]string{"method", "path", "status", "tenant_id"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hermes_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "tenant_id"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "hermes_http_requests_in_flight",
			Help: "Current number of HTTP requests being processed.",
		},
	)
)

// MetricsMiddleware records request count, latency, and in-flight gauge.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sw, r)

		duration := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		tenantID := "anonymous"
		if ac, ok := auth.FromContext(r.Context()); ok && ac != nil && ac.TenantID != "" {
			tenantID = ac.TenantID
		}

		httpRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(sw.status), tenantID).Inc()
		httpRequestDuration.WithLabelValues(r.Method, path, tenantID).Observe(duration)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wrote {
		sw.status = code
		sw.wrote = true
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if !sw.wrote {
		sw.wrote = true
	}
	return sw.ResponseWriter.Write(b)
}

// normalizePath collapses path segments with IDs to reduce cardinality.
func normalizePath(p string) string {
	if len(p) > 64 {
		p = p[:64]
	}
	return p
}
