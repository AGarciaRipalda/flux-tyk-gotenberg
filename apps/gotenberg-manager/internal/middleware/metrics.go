package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HttpRequestsTotal counts all HTTP requests by status, method and path
	HttpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed, partitioned by status code, method and HTTP path.",
		},
		[]string{"status", "method", "path"},
	)

	// HttpRequestDuration measures the latency of HTTP requests
	HttpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Latency of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// PdfConversionsTotal counts PDF generation attempts
	PdfConversionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pdf_conversions_total",
			Help: "Total number of PDF conversions processed (via Tyk or Portal).",
		},
		[]string{"mode", "status"},
	)
)

// MetricsMiddleware tracks metrics for all incoming requests.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(start).Seconds()

		// Get the registered route pattern (e.g. /api/clients/{id}) instead of full URL
		routeCtx := chi.RouteContext(r.Context())
		path := r.URL.Path
		if routeCtx != nil && routeCtx.RoutePattern() != "" {
			path = routeCtx.RoutePattern()
		}

		status := strconv.Itoa(ww.Status())

		HttpRequestsTotal.WithLabelValues(status, r.Method, path).Inc()
		HttpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}
