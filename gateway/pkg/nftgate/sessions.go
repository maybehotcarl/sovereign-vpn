package nftgate

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// SessionStore is a thread-safe in-memory session store.
// For Phase 0 single-instance deployment. Replace with Redis for multi-instance.
type SessionStore struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	addressToID map[common.Address]string
}

// NewSessionStore creates an empty session store with periodic cleanup.
func NewSessionStore() *SessionStore {
	ss := &SessionStore{
		sessions:    make(map[string]*Session),
		addressToID: make(map[common.Address]string),
	}
	go ss.cleanup()
	return ss
}

// Set stores or updates a session.
func (ss *SessionStore) Set(session *Session) {
	ss.mu.Lock()
	if session.AddressBound {
		if oldID, ok := ss.addressToID[session.Address]; ok && oldID != session.ID {
			delete(ss.sessions, oldID)
		}
		ss.addressToID[session.Address] = session.ID
	}
	ss.sessions[session.ID] = session
	ss.mu.Unlock()
}

// GetByID retrieves a session by session ID.
func (ss *SessionStore) GetByID(id string) *Session {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.sessions[id]
}

// GetByAddress retrieves a session by wallet address. Returns nil if not found.
func (ss *SessionStore) GetByAddress(addr common.Address) *Session {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	id, ok := ss.addressToID[addr]
	if !ok {
		return nil
	}
	return ss.sessions[id]
}

// DeleteByID removes a session by ID.
func (ss *SessionStore) DeleteByID(id string) {
	ss.mu.Lock()
	session, ok := ss.sessions[id]
	if ok && session.AddressBound {
		delete(ss.addressToID, session.Address)
	}
	delete(ss.sessions, id)
	ss.mu.Unlock()
}

// DeleteByAddress removes a session by address.
func (ss *SessionStore) DeleteByAddress(addr common.Address) {
	ss.mu.Lock()
	if id, ok := ss.addressToID[addr]; ok {
		delete(ss.sessions, id)
		delete(ss.addressToID, addr)
	}
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
		for id, session := range ss.sessions {
			if now.After(session.ExpiresAt) {
				if session.AddressBound {
					delete(ss.addressToID, session.Address)
				}
				delete(ss.sessions, id)
			}
		}
		ss.mu.Unlock()
	}
}
