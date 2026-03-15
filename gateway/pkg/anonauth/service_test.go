package anonauth

import (
	"testing"
	"time"
)

func TestNewChallengeIncludesPolicyEpochAndProofType(t *testing.T) {
	svc := NewService(time.Minute, 8, "vpn_access_v1", 42)

	challenge, err := svc.NewChallenge()
	if err != nil {
		t.Fatalf("NewChallenge: %v", err)
	}
	if challenge.ID == "" {
		t.Fatal("expected challenge id")
	}
	if challenge.Nonce == "" {
		t.Fatal("expected challenge nonce")
	}
	if challenge.PolicyEpoch != 42 {
		t.Fatalf("PolicyEpoch = %d, want 42", challenge.PolicyEpoch)
	}
	if challenge.ProofType != "vpn_access_v1" {
		t.Fatalf("ProofType = %q, want vpn_access_v1", challenge.ProofType)
	}
	if got := svc.GetChallenge(challenge.ID); got == nil {
		t.Fatal("expected stored challenge")
	}
}

func TestNullifierStoreConsume(t *testing.T) {
	store := NewNullifierStore()

	if ok := store.Consume("nul_1", time.Minute); !ok {
		t.Fatal("expected first consume to succeed")
	}
	if ok := store.Consume("nul_1", time.Minute); ok {
		t.Fatal("expected duplicate consume to fail")
	}
	if !store.IsConsumed("nul_1") {
		t.Fatal("expected nullifier to be active")
	}
}

func TestNullifierStoreExpires(t *testing.T) {
	store := NewNullifierStore()
	if ok := store.Consume("nul_2", 10*time.Millisecond); !ok {
		t.Fatal("expected consume to succeed")
	}

	time.Sleep(20 * time.Millisecond)

	if store.IsConsumed("nul_2") {
		t.Fatal("expected nullifier to expire")
	}
	if ok := store.Consume("nul_2", time.Minute); !ok {
		t.Fatal("expected consume after expiry to succeed")
	}
}
