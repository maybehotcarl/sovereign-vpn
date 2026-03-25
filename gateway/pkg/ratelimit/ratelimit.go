package ratelimit

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sharedredis"
)

type backend interface {
	Allow(ctx context.Context, key string) (bool, time.Duration, error)
	Close()
}

// Limiter tracks per-IP request counts over a rolling window.
type Limiter struct {
	limit   int
	window  time.Duration
	backend backend
}

type memoryBackend struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	visitors map[string]*visitor
	stopCh   chan struct{}
}

type visitor struct {
	tokens  int
	resetAt time.Time
}

const redisScript = `
local current = redis.call("INCR", KEYS[1])
if current == 1 then redis.call("PEXPIRE", KEYS[1], ARGV[1]) end
local ttl = redis.call("PTTL", KEYS[1])
return {current, ttl}
`

type redisBackend struct {
	store  *sharedredis.Store
	limit  int
	window time.Duration
}

// New creates a per-IP rate limiter.
// limit is the maximum number of requests allowed per window.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		window:  window,
		backend: newMemoryBackend(limit, window),
	}
}

// NewRedis creates a Redis-backed per-IP rate limiter suitable for multi-node deployments.
func NewRedis(store *sharedredis.Store, limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		window:  window,
		backend: &redisBackend{store: store, limit: limit, window: window},
	}
}

func newMemoryBackend(limit int, window time.Duration) *memoryBackend {
	l := &memoryBackend{
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
	allowed, _, err := l.AllowWithContext(context.Background(), ip)
	return err == nil && allowed
}

// AllowWithContext checks whether the given IP is within the rate limit and returns retry timing.
func (l *Limiter) AllowWithContext(ctx context.Context, ip string) (bool, time.Duration, error) {
	return l.backend.Allow(ctx, ip)
}

// Wrap returns HTTP middleware that rejects requests exceeding the rate limit
// with 429 Too Many Requests.
func (l *Limiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		ip := extractIP(r)
		allowed, retryAfter, err := l.AllowWithContext(r.Context(), ip)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"rate limit backend unavailable"}`))
			return
		}
		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(retryAfter.Seconds())))))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Stop shuts down the background cleanup goroutine or closes the backend.
func (l *Limiter) Stop() {
	if l == nil || l.backend == nil {
		return
	}
	l.backend.Close()
}

func (l *memoryBackend) Allow(_ context.Context, ip string) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	v, ok := l.visitors[ip]
	if !ok || now.After(v.resetAt) {
		l.visitors[ip] = &visitor{tokens: l.limit - 1, resetAt: now.Add(l.window)}
		return true, l.window, nil
	}
	if v.tokens <= 0 {
		return false, time.Until(v.resetAt), nil
	}
	v.tokens--
	return true, time.Until(v.resetAt), nil
}

func (l *memoryBackend) Close() {
	close(l.stopCh)
}

// cleanup removes expired visitor entries every 2 minutes.
func (l *memoryBackend) cleanup() {
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

func (l *redisBackend) Allow(ctx context.Context, ip string) (bool, time.Duration, error) {
	key := l.store.Key("ratelimit", ip)
	result, err := l.store.Client().Eval(
		ctx,
		redisScript,
		[]string{key},
		l.window.Milliseconds(),
	).Result()
	if err != nil {
		return false, 0, err
	}

	values, ok := result.([]any)
	if !ok || len(values) < 2 {
		return false, 0, nil
	}

	current := toInt64(values[0])
	ttlMs := toInt64(values[1])
	if ttlMs <= 0 {
		ttlMs = l.window.Milliseconds()
	}

	return current <= int64(l.limit), time.Duration(ttlMs) * time.Millisecond, nil
}

func (l *redisBackend) Close() {}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case uint64:
		return int64(v)
	default:
		return 0
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
