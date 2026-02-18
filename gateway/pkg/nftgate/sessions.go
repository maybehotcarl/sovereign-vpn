package nftgate

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// SessionStore is a thread-safe in-memory session store.
// For Phase 0 single-instance deployment. Replace with Redis for multi-instance.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[common.Address]*Session
}

// NewSessionStore creates an empty session store with periodic cleanup.
func NewSessionStore() *SessionStore {
	ss := &SessionStore{
		sessions: make(map[common.Address]*Session),
	}
	go ss.cleanup()
	return ss
}

// Set stores or updates a session.
func (ss *SessionStore) Set(addr common.Address, session *Session) {
	ss.mu.Lock()
	ss.sessions[addr] = session
	ss.mu.Unlock()
}

// Get retrieves a session by wallet address. Returns nil if not found.
func (ss *SessionStore) Get(addr common.Address) *Session {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.sessions[addr]
}

// Delete removes a session.
func (ss *SessionStore) Delete(addr common.Address) {
	ss.mu.Lock()
	delete(ss.sessions, addr)
	ss.mu.Unlock()
}

// Len returns the number of sessions.
func (ss *SessionStore) Len() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.sessions)
}

// cleanup periodically removes expired sessions.
func (ss *SessionStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ss.mu.Lock()
		now := time.Now()
		for addr, session := range ss.sessions {
			if now.After(session.ExpiresAt) {
				delete(ss.sessions, addr)
			}
		}
		ss.mu.Unlock()
	}
}
