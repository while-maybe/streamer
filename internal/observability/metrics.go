package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Counter: Total HTTP requests
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "streamer_http_requests_total",
			Help: "The total number of processed HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// Histogram: Response time
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "streamer_http_request_duration_seconds",
			Help:    "The latency of the HTTP requests",
			Buckets: prometheus.DefBuckets, // .005s to 10s
		},
		[]string{"method", "path"},
	)

	// Gauge: Active Streams (Goes up and down)
	ActiveStreams = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "streamer_active_streams_current",
			Help: "The current number of active media streams",
		},
	)
)
