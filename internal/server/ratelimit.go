package server

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimiter implements per-key token-bucket rate limiting.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a RateLimiter with the given requests-per-second and burst size.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rps),
		burst:    burst,
	}
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if l, ok := rl.limiters[key]; ok {
		return l
	}
	l := rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[key] = l
	return l
}

// Middleware returns an http.Handler that enforces rate limits per client.
// The rate limit key is derived from the API key prefix (first 10 chars of
// the Authorization header) or the remote address for unauthenticated requests.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use API key prefix or IP as the rate limit key.
		key := r.RemoteAddr
		if auth := r.Header.Get("Authorization"); len(auth) > 10 {
			key = auth[:10] // key prefix
		}
		if !rl.getLimiter(key).Allow() {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
