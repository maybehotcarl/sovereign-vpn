package anonauth

import (
	"sync"
	"time"
)

// ChallengeStore keeps short-lived anonymous challenges in memory.
type ChallengeStore struct {
	mu         sync.RWMutex
	challenges map[string]*Challenge
}

// NewChallengeStore creates an empty challenge store with periodic cleanup.
func NewChallengeStore() *ChallengeStore {
	cs := &ChallengeStore{
		challenges: make(map[string]*Challenge),
	}
	go cs.cleanup()
	return cs
}

// Set stores a challenge.
func (cs *ChallengeStore) Set(challenge *Challenge) {
	cs.mu.Lock()
	cs.challenges[challenge.ID] = challenge
	cs.mu.Unlock()
}

// Get retrieves a challenge if it exists and has not expired.
func (cs *ChallengeStore) Get(id string) *Challenge {
	cs.mu.RLock()
	challenge := cs.challenges[id]
	cs.mu.RUnlock()
	if challenge == nil {
		return nil
	}

	if time.Now().UTC().After(challenge.ExpiresAt) {
		cs.Delete(id)
		return nil
	}
	return challenge
}

// Delete removes a challenge by ID.
func (cs *ChallengeStore) Delete(id string) {
	cs.mu.Lock()
	delete(cs.challenges, id)
	cs.mu.Unlock()
}

func (cs *ChallengeStore) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		cs.mu.Lock()
		for id, challenge := range cs.challenges {
			if now.After(challenge.ExpiresAt) {
				delete(cs.challenges, id)
			}
		}
		cs.mu.Unlock()
	}
}
