package main

import (
	"net/http"
	"slices"
	"strconv"
	"streamer/internal/observability"
	"time"
)

type Middleware func(http.Handler) http.Handler

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func wrapWriter(w http.ResponseWriter) *statusRecorder {
	if recorder, ok := w.(*statusRecorder); ok {
		return recorder
	}
	// Default to 200 OK in case WriteHeader isn't called explicitly
	return &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
}

func middlewareChain(h http.Handler, mws ...Middleware) http.Handler {
	for _, m := range slices.Backward(mws) {
		h = m(h)
	}
	return h
}

func (a *App) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// notifies the shutdown monitor activity
		if a.monitor != nil {
			a.monitor.NotifyActivity()
		}

		recorder := wrapWriter(w)

		start := time.Now()
		next.ServeHTTP(recorder, r)
		duration := time.Since(start).Seconds()

		a.logger.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"status", recorder.statusCode,
			"duration_ms", duration,
		)
	})
}

func (a *App) withObservability(next http.Handler) http.Handler {
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
