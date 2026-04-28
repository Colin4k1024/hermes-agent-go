package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TracingMiddleware creates a root span for each HTTP request and injects
// trace_id into the response header X-Trace-ID.
func TracingMiddleware(next http.Handler) http.Handler {
	tracer := otel.Tracer("hermes-http")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), r.Method+" "+r.URL.Path,
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.path", r.URL.Path),
			),
		)
		defer span.End()

		traceID := span.SpanContext().TraceID()
		if traceID.IsValid() {
			w.Header().Set("X-Trace-ID", traceID.String())
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
