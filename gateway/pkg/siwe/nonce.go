package siwe

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// NonceStore tracks issued nonces and prevents replay attacks.
// In-memory for now; swap to Redis for production multi-instance deployments.
type NonceStore struct {
	mu     sync.Mutex
	nonces map[string]time.Time // nonce -> expiry time
	ttl    time.Duration
}

// NewNonceStore creates a nonce store with the given TTL for challenges.
func NewNonceStore(ttl time.Duration) *NonceStore {
	ns := &NonceStore{
		nonces: make(map[string]time.Time),
		ttl:    ttl,
	}
	go ns.cleanup()
	return ns
}

// Generate creates a new random nonce and stores it.
func (ns *NonceStore) Generate(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(bytes)

	ns.mu.Lock()
	ns.nonces[nonce] = time.Now().Add(ns.ttl)
	ns.mu.Unlock()

	return nonce, nil
}

// Consume validates a nonce and removes it (single-use).
// Returns false if the nonce doesn't exist or has expired.
func (ns *NonceStore) Consume(nonce string) bool {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	expiry, exists := ns.nonces[nonce]
	if !exists {
		return false
	}

	delete(ns.nonces, nonce)

	return time.Now().Before(expiry)
}

// cleanup periodically removes expired nonces.
func (ns *NonceStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ns.mu.Lock()
		now := time.Now()
		for nonce, expiry := range ns.nonces {
			if now.After(expiry) {
				delete(ns.nonces, nonce)
			}
		}
		ns.mu.Unlock()
	}
}
