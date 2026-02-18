package siwe

import (
	"crypto/ecdsa"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// Helper: sign a message with a private key using ERC-191 personal_sign.
func personalSign(key *ecdsa.PrivateKey, message string) (string, error) {
	hash := signHash([]byte(message))
	sig, err := crypto.Sign(hash, key)
	if err != nil {
		return "", err
	}
	// Convert recovery ID from 0/1 to 27/28 (MetaMask format)
	sig[64] += 27
	return hexutil.Encode(sig), nil
}

func TestNewChallenge(t *testing.T) {
	svc := NewService("test.example.com", "https://test.example.com", 5*time.Minute, 16)

	challenge, err := svc.NewChallenge(16)
	if err != nil {
		t.Fatalf("NewChallenge failed: %v", err)
	}

	if challenge.Domain != "test.example.com" {
		t.Errorf("expected domain test.example.com, got %s", challenge.Domain)
	}
	if challenge.Version != "1" {
		t.Errorf("expected version 1, got %s", challenge.Version)
	}
	if len(challenge.Nonce) != 32 { // 16 bytes -> 32 hex chars
		t.Errorf("expected nonce length 32, got %d", len(challenge.Nonce))
	}
}

func TestFormatMessage(t *testing.T) {
	challenge := &Challenge{
		Domain:    "test.example.com",
		URI:       "https://test.example.com",
		Version:   "1",
		ChainID:   1,
		Nonce:     "abc123",
		IssuedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Statement: "Sign in to Sovereign VPN with your Ethereum account.",
	}

	msg := FormatMessage(challenge, "0x1234567890abcdef1234567890abcdef12345678")

	if msg == "" {
		t.Fatal("FormatMessage returned empty string")
	}

	// Check it contains expected elements
	expected := []string{
		"test.example.com wants you to sign in with your Ethereum account:",
		"0x1234567890abcdef1234567890abcdef12345678",
		"URI: https://test.example.com",
		"Version: 1",
		"Chain ID: 1",
		"Nonce: abc123",
		"Issued At: 2026-01-01T00:00:00Z",
		"Sign in to Sovereign VPN",
	}
	for _, exp := range expected {
		if !contains(msg, exp) {
			t.Errorf("message missing expected text: %q\n\nFull message:\n%s", exp, msg)
		}
	}
}

func TestVerifyValidSignature(t *testing.T) {
	svc := NewService("test.example.com", "https://test.example.com", 5*time.Minute, 16)

	// Generate a test key
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey)

	// Get a challenge
	challenge, err := svc.NewChallenge(16)
	if err != nil {
		t.Fatalf("NewChallenge failed: %v", err)
	}

	// Format the message
	message := FormatMessage(challenge, address.Hex())

	// Sign it
	sig, err := personalSign(key, message)
	if err != nil {
		t.Fatalf("personalSign failed: %v", err)
	}

	// Verify
	auth, err := svc.Verify(&SignedMessage{
		Message:   message,
		Signature: sig,
	})
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if auth.Address != address {
		t.Errorf("expected address %s, got %s", address.Hex(), auth.Address.Hex())
	}
}

func TestVerifyRejectsReplayedNonce(t *testing.T) {
	svc := NewService("test.example.com", "https://test.example.com", 5*time.Minute, 16)

	key, _ := crypto.GenerateKey()
	address := crypto.PubkeyToAddress(key.PublicKey)

	challenge, _ := svc.NewChallenge(16)
	message := FormatMessage(challenge, address.Hex())
	sig, _ := personalSign(key, message)

	signed := &SignedMessage{Message: message, Signature: sig}

	// First verify should succeed
	_, err := svc.Verify(signed)
	if err != nil {
		t.Fatalf("First verify should succeed: %v", err)
	}

	// Second verify with same nonce should fail
	_, err = svc.Verify(signed)
	if err == nil {
		t.Fatal("Second verify should fail (nonce replay)")
	}
}

func TestVerifyRejectsWrongDomain(t *testing.T) {
	svc := NewService("real.example.com", "https://real.example.com", 5*time.Minute, 16)

	key, _ := crypto.GenerateKey()
	address := crypto.PubkeyToAddress(key.PublicKey)

	challenge, _ := svc.NewChallenge(16)

	// Tamper with the domain in the message
	challenge.Domain = "evil.example.com"
	message := FormatMessage(challenge, address.Hex())
	sig, _ := personalSign(key, message)

	_, err := svc.Verify(&SignedMessage{Message: message, Signature: sig})
	if err == nil {
		t.Fatal("Should reject wrong domain")
	}
}

func TestVerifyRejectsWrongSigner(t *testing.T) {
	svc := NewService("test.example.com", "https://test.example.com", 5*time.Minute, 16)

	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()
	address1 := crypto.PubkeyToAddress(key1.PublicKey)

	challenge, _ := svc.NewChallenge(16)
	message := FormatMessage(challenge, address1.Hex())

	// Sign with key2 (wrong key)
	sig, _ := personalSign(key2, message)

	_, err := svc.Verify(&SignedMessage{Message: message, Signature: sig})
	if err == nil {
		t.Fatal("Should reject signature from wrong key")
	}
}

func TestNonceStoreConsume(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)

	nonce, _ := store.Generate(16)

	// First consume should succeed
	if !store.Consume(nonce) {
		t.Fatal("First consume should succeed")
	}

	// Second consume should fail
	if store.Consume(nonce) {
		t.Fatal("Second consume should fail")
	}
}

func TestNonceStoreExpiry(t *testing.T) {
	store := NewNonceStore(1 * time.Millisecond)

	nonce, _ := store.Generate(16)

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	if store.Consume(nonce) {
		t.Fatal("Should reject expired nonce")
	}
}

func TestNonceStoreRejectsUnknown(t *testing.T) {
	store := NewNonceStore(5 * time.Minute)

	if store.Consume("nonexistent") {
		t.Fatal("Should reject unknown nonce")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
