package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter tracks per-IP request counts over a rolling window.
type Limiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	visitors map[string]*visitor
	stopCh   chan struct{}
}

type visitor struct {
	tokens    int
	resetAt   time.Time
}

// New creates a per-IP rate limiter.
// limit is the maximum number of requests allowed per window.
func New(limit int, window time.Duration) *Limiter {
	l := &Limiter{
		limit:    limit,
		window:   window,
		visitors: make(map[string]*visitor),
		stopCh:   make(chan struct{}),
	}
	go l.cleanup()
	return l
}

// Allow checks whether the given IP is within the rate limit.
func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	v, ok := l.visitors[ip]
	if !ok || now.After(v.resetAt) {
		l.visitors[ip] = &visitor{tokens: l.limit - 1, resetAt: now.Add(l.window)}
		return true
	}
	if v.tokens <= 0 {
		return false
	}
	v.tokens--
	return true
}

// Wrap returns HTTP middleware that rejects requests exceeding the rate limit
// with 429 Too Many Requests.
func (l *Limiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !l.Allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Stop shuts down the background cleanup goroutine.
func (l *Limiter) Stop() {
	close(l.stopCh)
}

// cleanup removes expired visitor entries every 2 minutes.
func (l *Limiter) cleanup() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			for ip, v := range l.visitors {
				if now.After(v.resetAt) {
					delete(l.visitors, ip)
				}
			}
			l.mu.Unlock()
		case <-l.stopCh:
			return
		}
	}
}

// extractIP returns the client IP, preferring X-Forwarded-For when present
// (common behind reverse proxies), falling back to RemoteAddr.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first comma-separated value (original client)
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return strings.TrimSpace(xff[:i])
			}
		}
		return strings.TrimSpace(xff)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
