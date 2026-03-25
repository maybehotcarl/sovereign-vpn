package sharedredis

import (
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultPrefix = "sovereign-vpn"

// Store wraps a Redis client with a stable key prefix for gateway shared state.
type Store struct {
	client *redis.Client
	prefix string
}

// New creates a Redis-backed shared state store from a redis:// or rediss:// URL.
func New(url string, prefix string) (*Store, error) {
	opts, err := redis.ParseURL(strings.TrimSpace(url))
	if err != nil {
		return nil, err
	}

	normalizedPrefix := strings.Trim(strings.TrimSpace(prefix), ":")
	if normalizedPrefix == "" {
		normalizedPrefix = defaultPrefix
	}

	return &Store{
		client: redis.NewClient(opts),
		prefix: normalizedPrefix,
	}, nil
}

// Client returns the underlying Redis client.
func (s *Store) Client() *redis.Client {
	return s.client
}

// Close closes the underlying Redis client.
func (s *Store) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

// Key builds a prefixed Redis key.
func (s *Store) Key(parts ...string) string {
	segments := []string{s.prefix}
	for _, part := range parts {
		trimmed := strings.Trim(strings.TrimSpace(part), ":")
		if trimmed == "" {
			continue
		}
		segments = append(segments, trimmed)
	}
	return strings.Join(segments, ":")
}

// TTLUntil computes a positive TTL for Redis expiry.
func TTLUntil(expiresAt time.Time) time.Duration {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return time.Second
	}
	return ttl
}
