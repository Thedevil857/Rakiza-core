// Package middleware contains cross-cutting HTTP middleware for the API
// server: rate limiting, request logging glue, and (in later phases)
// authentication.
package middleware

import (
	"encoding/json"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// visitorLimiter pairs a per-client token-bucket limiter with a mutex so it
// can be safely read/written across concurrent goroutines handling
// different in-flight requests from the same client.
type visitorLimiter struct {
	limiter *rate.Limiter
}

// IPRateLimiter is a global, in-memory, per-client-IP rate limiter built on
// top of golang.org/x/time/rate's token-bucket algorithm. It is intentionally
// simple (no external dependency like Redis) because this API is expected to
// run as a single process close to its database in Phase 1; a distributed
// limiter can replace this in a later horizontal-scaling phase without
// changing the handler-facing API.
type IPRateLimiter struct {
	mu             sync.Mutex
	visitors       map[string]*visitorLimiter
	requestsPerSec rate.Limit
	burst          int
}

// NewIPRateLimiter constructs a rate limiter allowing requestsPerSecond
// sustained requests per client IP, with the ability to burst up to burst
// requests instantaneously before throttling kicks in.
func NewIPRateLimiter(requestsPerSecond float64, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		visitors:       make(map[string]*visitorLimiter),
		requestsPerSec: rate.Limit(requestsPerSecond),
		burst:          burst,
	}
}

// getLimiter returns the token-bucket limiter for a given client key
// (typically the remote IP address), creating one on first sight of that
// client. This lazily-growing map is acceptable for Phase 1's expected
// traffic volume; a production hardening pass should add an eviction policy
// (e.g. a periodic sweep of idle entries) to bound memory growth under a
// sustained flood of distinct source IPs.
func (rl *IPRateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[key]
	if !exists {
		newLimiter := rate.NewLimiter(rl.requestsPerSec, rl.burst)
		rl.visitors[key] = &visitorLimiter{limiter: newLimiter}
		return newLimiter
	}
	return v.limiter
}

// Middleware returns a chi/net-http compatible middleware function that
// rejects requests exceeding the configured rate with HTTP 429 Too Many
// Requests and a small JSON error body, before the request ever reaches
// application handlers or touches the database.
func (rl *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientKey := clientIdentifier(r)
		limiter := rl.getLimiter(clientKey)

		if !limiter.Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests. Please slow down and try again shortly.",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// clientIdentifier extracts the best-effort client identity to rate-limit
// on. It prefers the X-Forwarded-For header (set by a reverse proxy/load
// balancer in front of the API) and falls back to RemoteAddr for direct
// connections, which is the common case during local development.
func clientIdentifier(r *http.Request) string {
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		return forwardedFor
	}
	return r.RemoteAddr
}
