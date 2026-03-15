package anonauth

import (
	"sync"
	"time"
)

// NullifierStore tracks nullifiers that have been consumed by the anonymous path.
type NullifierStore struct {
	mu         sync.RWMutex
	nullifiers map[string]time.Time
}

// NewNullifierStore creates a nullifier store with periodic cleanup.
func NewNullifierStore() *NullifierStore {
	ns := &NullifierStore{
		nullifiers: make(map[string]time.Time),
	}
	go ns.cleanup()
	return ns
}

// Consume marks a nullifier as used until the provided TTL expires.
// Returns false if the nullifier is already active.
func (ns *NullifierStore) Consume(nullifier string, ttl time.Duration) bool {
	if nullifier == "" {
		return false
	}
	if ttl <= 0 {
		ttl = time.Minute
	}

	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	ns.mu.Lock()
	defer ns.mu.Unlock()
	if current, ok := ns.nullifiers[nullifier]; ok && now.Before(current) {
		return false
	}
	ns.nullifiers[nullifier] = expiresAt
	return true
}

// IsConsumed reports whether the nullifier is still active.
func (ns *NullifierStore) IsConsumed(nullifier string) bool {
	now := time.Now().UTC()

	ns.mu.RLock()
	expiresAt, ok := ns.nullifiers[nullifier]
	ns.mu.RUnlock()
	if !ok {
		return false
	}
	if now.Before(expiresAt) {
		return true
	}

	ns.mu.Lock()
	if current, ok := ns.nullifiers[nullifier]; ok && !now.Before(current) {
		delete(ns.nullifiers, nullifier)
	}
	ns.mu.Unlock()
	return false
}

// Release removes a nullifier from the active set.
func (ns *NullifierStore) Release(nullifier string) {
	if nullifier == "" {
		return
	}

	ns.mu.Lock()
	delete(ns.nullifiers, nullifier)
	ns.mu.Unlock()
}

func (ns *NullifierStore) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		ns.mu.Lock()
		for nullifier, expiresAt := range ns.nullifiers {
			if !now.Before(expiresAt) {
				delete(ns.nullifiers, nullifier)
			}
		}
		ns.mu.Unlock()
	}
}
