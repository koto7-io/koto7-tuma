package api

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

type tokenBucket struct {
	tokens   float64
	last     time.Time
	rate     float64
	burst    float64
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: make(map[string]*tokenBucket)}
}

func (rl *rateLimiter) allow(key string, perHour, burst int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[key]
	if !ok {
		b = &tokenBucket{
			tokens: float64(burst),
			last:   time.Now(),
			rate:   float64(perHour) / 3600.0,
			burst:  float64(burst),
		}
		rl.buckets[key] = b
	}
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * b.rate
	if b.tokens > b.burst {
		b.tokens = b.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type streamLimiter struct {
	mu      sync.Mutex
	active  map[string]int
}

func newStreamLimiter() *streamLimiter {
	return &streamLimiter{active: make(map[string]int)}
}

func (sl *streamLimiter) acquire(ip string, max int) bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if sl.active[ip] >= max {
		return false
	}
	sl.active[ip]++
	return true
}

func (sl *streamLimiter) release(ip string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.active[ip]--
	if sl.active[ip] <= 0 {
		delete(sl.active, ip)
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func writeRateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
}
