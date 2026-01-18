package middleware

import (
	"log/slog"
	"net/http"
	"strconv"
	"streamer/internal/observability"
	"time"
)

type ActivityNotifier interface {
	NotifyActivity()
}

func WithLogging(logger *slog.Logger, monitor ActivityNotifier) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// notifies the shutdown monitor activity
			if monitor != nil {
				monitor.NotifyActivity()
			}

			recorder := wrapWriter(w)

			start := time.Now()
			next.ServeHTTP(recorder, r)
			duration := time.Since(start).Seconds()

			logger.Debug("request",
				"method", r.Method,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
				"status", recorder.statusCode,
				"duration_ms", duration,
			)
		})
	}
}

func WithObservability() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			recorder := wrapWriter(w)

			start := time.Now()
			next.ServeHTTP(recorder, r)
			duration := time.Since(start).Seconds()

			observability.RequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)

			statusStr := strconv.Itoa(recorder.statusCode)
			observability.RequestsTotal.WithLabelValues(r.Method, r.URL.Path, statusStr).Inc()
		})
	}
}
