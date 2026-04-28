package llm

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	llmRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hermes_llm_request_duration_seconds",
			Help:    "LLM API request latency in seconds.",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		},
		[]string{"model", "tenant_id"},
	)

	llmTokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hermes_llm_tokens_total",
			Help: "Total LLM tokens consumed.",
		},
		[]string{"model", "tenant_id", "direction"},
	)
)
