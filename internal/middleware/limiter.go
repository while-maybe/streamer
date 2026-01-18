package middleware

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type IPRateLimiter struct {
	ips          map[string]*client
	mu           sync.Mutex
	rate         rate.Limit
	burst        int
	trustedProxy bool
}

func NewIPRateLimiter(ctx context.Context, rps, burst int, trustedProxy bool) *IPRateLimiter {
	limiter := &IPRateLimiter{
		ips:          make(map[string]*client),
		rate:         rate.Limit(rps),
		burst:        burst,
		trustedProxy: trustedProxy,
	}

	// cleanup stale entries
	go func() {
		cleanupFrequency := 1 * time.Minute

		ticker := time.NewTicker(cleanupFrequency)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				limiter.cleanup()
			}
		}
	}()
	return limiter
}

func (i *IPRateLimiter) getLimiter(ip string) (*rate.Limiter, error) {
	cleanIP := net.ParseIP(ip)
	if cleanIP == nil {
		return nil, errors.New("invalid IP")
	}
	cleanIPStr := cleanIP.String()

	i.mu.Lock()
	defer i.mu.Unlock()

	c, ok := i.ips[cleanIPStr]
	if !ok {
		limiter := rate.NewLimiter(i.rate, i.burst)
		// and a new client then add it to the ips map
		c = &client{
			limiter:  limiter,
			lastSeen: time.Now().UTC(),
		}
		i.ips[cleanIPStr] = c
		return limiter, nil
	}

	c.lastSeen = time.Now().UTC()
	return c.limiter, nil
}

func (i *IPRateLimiter) cleanup() {
	inactiveLimit := 3 * time.Minute

	i.mu.Lock()
	defer i.mu.Unlock()

	for ip, client := range i.ips {
		if time.Since(client.lastSeen) > inactiveLimit {
			delete(i.ips, ip)
		}
	}
}

func (i *IPRateLimiter) getClientIP(r *http.Request) string {
	if i.trustedProxy {
		// Check X-Forwarded-For first
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if idx := strings.Index(xff, ","); idx > 0 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}

		// Check X-Real-IP as fallback
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return xri
		}
	}

	// Fallback to RemoteAddr
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

func (i *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// grab the source ip address
		ip := i.getClientIP(r)

		limiter, err := i.getLimiter(ip)
		if err != nil {
			http.Error(w, "invalid ip address", http.StatusBadRequest)
			return
		}

		if !limiter.Allow() {
			// Peek at when next token available (without consuming)
			reservation := limiter.Reserve()
			delay := reservation.Delay()
			reservation.Cancel() // Don't consume

			retrySeconds := int(delay.Seconds())
			retrySeconds = max(1, retrySeconds)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(i.burst))
			w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))
			w.Header().Set("X-RateLimit-Remaining", "0")

			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// X-RateLimit-Limit: 100           # Maximum requests allowed
// X-RateLimit-Remaining: 42        # Requests remaining in window
// Retry-After: 12                  # Seconds until retry (when limited)
// X-RateLimit-Reset: 1704672000    # Unix timestamp when limit resets
