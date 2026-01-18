package middleware

import (
	"net/http"
	"slices"
)

type Middleware func(http.Handler) http.Handler

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, m := range slices.Backward(mws) {
		h = m(h)
	}
	return h
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
