package anonauth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const defaultProofType = "vpn_access_v1"

// Challenge is returned to clients before they generate an anonymous access proof.
type Challenge struct {
	ID          string    `json:"challenge_id"`
	Nonce       string    `json:"nonce"`
	PolicyEpoch uint64    `json:"policy_epoch"`
	ProofType   string    `json:"proof_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Service manages anonymous-access challenges and nullifier tracking.
type Service struct {
	challengeTTL time.Duration
	nonceLength  int
	proofType    string
	challenges   challengeStoreBackend
	nullifiers   nullifierStoreBackend

	mu          sync.RWMutex
	policyEpoch uint64
}

// NewService creates a new anonymous auth service.
func NewService(challengeTTL time.Duration, nonceLength int, proofType string, policyEpoch uint64) *Service {
	return NewServiceWithBackends(
		challengeTTL,
		nonceLength,
		proofType,
		policyEpoch,
		newInMemoryChallengeBackend(),
		newInMemoryNullifierBackend(),
	)
}

// NewServiceWithBackends creates an anonymous auth service with pluggable shared-state backends.
func NewServiceWithBackends(
	challengeTTL time.Duration,
	nonceLength int,
	proofType string,
	policyEpoch uint64,
	challengeStore challengeStoreBackend,
	nullifierStore nullifierStoreBackend,
) *Service {
	if nonceLength <= 0 {
		nonceLength = 16
	}
	if proofType == "" {
		proofType = defaultProofType
	}
	if challengeStore == nil {
		challengeStore = newInMemoryChallengeBackend()
	}
	if nullifierStore == nil {
		nullifierStore = newInMemoryNullifierBackend()
	}

	return &Service{
		challengeTTL: challengeTTL,
		nonceLength:  nonceLength,
		proofType:    proofType,
		challenges:   challengeStore,
		nullifiers:   nullifierStore,
		policyEpoch:  policyEpoch,
	}
}

// NewChallenge creates and stores a new anonymous-access challenge.
func (s *Service) NewChallenge() (*Challenge, error) {
	id, err := randomToken(24)
	if err != nil {
		return nil, fmt.Errorf("generating challenge id: %w", err)
	}
	nonce, err := randomToken(s.nonceLength)
	if err != nil {
		return nil, fmt.Errorf("generating challenge nonce: %w", err)
	}

	now := time.Now().UTC()
	challenge := &Challenge{
		ID:          id,
		Nonce:       nonce,
		PolicyEpoch: s.PolicyEpoch(),
		ProofType:   s.proofType,
		ExpiresAt:   now.Add(s.challengeTTL),
	}
	if err := s.challenges.Set(challenge); err != nil {
		return nil, fmt.Errorf("storing challenge: %w", err)
	}
	return challenge, nil
}

// GetChallenge retrieves a stored challenge if it is still active.
func (s *Service) GetChallenge(id string) *Challenge {
	challenge, _ := s.GetChallengeWithError(id)
	return challenge
}

// GetChallengeWithError retrieves a stored challenge if it is still active.
func (s *Service) GetChallengeWithError(id string) (*Challenge, error) {
	return s.challenges.Get(id)
}

// DeleteChallenge removes a challenge after it has been used or abandoned.
func (s *Service) DeleteChallenge(id string) {
	_ = s.DeleteChallengeWithError(id)
}

// DeleteChallengeWithError removes a challenge after it has been used or abandoned.
func (s *Service) DeleteChallengeWithError(id string) error {
	return s.challenges.Delete(id)
}

// ConsumeNullifier marks a nullifier as used for the provided TTL.
func (s *Service) ConsumeNullifier(nullifier string, ttl time.Duration) bool {
	ok, _ := s.ConsumeNullifierWithError(nullifier, ttl)
	return ok
}

// ConsumeNullifierWithError marks a nullifier as used for the provided TTL.
func (s *Service) ConsumeNullifierWithError(nullifier string, ttl time.Duration) (bool, error) {
	return s.nullifiers.Consume(nullifier, ttl)
}

// IsNullifierConsumed reports whether a nullifier is still active.
func (s *Service) IsNullifierConsumed(nullifier string) bool {
	ok, _ := s.IsNullifierConsumedWithError(nullifier)
	return ok
}

// IsNullifierConsumedWithError reports whether a nullifier is still active.
func (s *Service) IsNullifierConsumedWithError(nullifier string) (bool, error) {
	return s.nullifiers.IsConsumed(nullifier)
}

// ReleaseNullifier removes a nullifier reservation.
func (s *Service) ReleaseNullifier(nullifier string) {
	_ = s.ReleaseNullifierWithError(nullifier)
}

// ReleaseNullifierWithError removes a nullifier reservation.
func (s *Service) ReleaseNullifierWithError(nullifier string) error {
	return s.nullifiers.Release(nullifier)
}

// SetPolicyEpoch updates the active policy epoch used for new challenges.
func (s *Service) SetPolicyEpoch(epoch uint64) {
	s.mu.Lock()
	s.policyEpoch = epoch
	s.mu.Unlock()
}

// PolicyEpoch returns the active policy epoch.
func (s *Service) PolicyEpoch() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policyEpoch
}

func randomToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
