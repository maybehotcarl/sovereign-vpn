package wireguard

import (
	"testing"
	"time"
)

// These tests exercise the IP pool and in-memory peer tracking.
// They do NOT require a real WireGuard interface — we override the
// wgSetPeer/wgRemovePeer calls by testing the pool and tracking directly.

func TestIPPoolAllocate(t *testing.T) {
	pool, err := newIPPool("10.8.0.0/24")
	if err != nil {
		t.Fatalf("newIPPool: %v", err)
	}

	ip, err := pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if ip != "10.8.0.2" {
		t.Errorf("first IP should be 10.8.0.2, got %s", ip)
	}

	ip2, err := pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if ip2 != "10.8.0.3" {
		t.Errorf("second IP should be 10.8.0.3, got %s", ip2)
	}
}

func TestIPPoolRelease(t *testing.T) {
	pool, err := newIPPool("10.8.0.0/24")
	if err != nil {
		t.Fatal(err)
	}

	ip1, _ := pool.Allocate()
	ip2, _ := pool.Allocate()

	pool.Release(ip1)

	// After releasing ip1, allocating should give ip2+1 first (sequential),
	// then wrap around to ip1 when that slot comes up.
	ip3, _ := pool.Allocate()
	if ip3 != "10.8.0.4" {
		t.Errorf("expected 10.8.0.4 (sequential), got %s", ip3)
	}

	_ = ip2 // used
}

func TestIPPoolExhaustion(t *testing.T) {
	pool, err := newIPPool("10.8.0.0/24")
	if err != nil {
		t.Fatal(err)
	}

	// Allocate all 253 addresses (.2 through .254)
	for i := 0; i < 253; i++ {
		_, err := pool.Allocate()
		if err != nil {
			t.Fatalf("Allocate failed at iteration %d: %v", i, err)
		}
	}

	// Next allocation should fail
	_, err = pool.Allocate()
	if err == nil {
		t.Error("expected error when pool exhausted")
	}
}

func TestIPPoolReleaseAndReallocate(t *testing.T) {
	pool, err := newIPPool("10.8.0.0/24")
	if err != nil {
		t.Fatal(err)
	}

	// Allocate all
	ips := make([]string, 253)
	for i := 0; i < 253; i++ {
		ips[i], _ = pool.Allocate()
	}

	// Release one in the middle
	pool.Release(ips[100])

	// Should be able to allocate again
	ip, err := pool.Allocate()
	if err != nil {
		t.Fatalf("should be able to allocate after release: %v", err)
	}
	if ip != ips[100] {
		t.Logf("reallocated IP: %s (released was %s)", ip, ips[100])
	}
}

func TestIPPoolDifferentSubnets(t *testing.T) {
	pool, err := newIPPool("172.16.0.0/24")
	if err != nil {
		t.Fatal(err)
	}

	ip, _ := pool.Allocate()
	if ip != "172.16.0.2" {
		t.Errorf("expected 172.16.0.2, got %s", ip)
	}
}

func TestIPPoolInvalidSubnet(t *testing.T) {
	_, err := newIPPool("not-a-cidr")
	if err == nil {
		t.Error("expected error for invalid subnet")
	}
}

func TestPeerTracking(t *testing.T) {
	// Test the peer map and count without real WG commands
	m := &Manager{
		peers: make(map[string]*Peer),
	}

	if m.PeerCount() != 0 {
		t.Errorf("expected 0 peers, got %d", m.PeerCount())
	}

	now := time.Now()
	m.peers["test-key-1"] = &Peer{
		PublicKey:  "test-key-1",
		ClientIP:   "10.8.0.2",
		AssignedAt: now,
		ExpiresAt:  now.Add(1 * time.Hour),
	}

	if m.PeerCount() != 1 {
		t.Errorf("expected 1 peer, got %d", m.PeerCount())
	}

	peer := m.GetPeer("test-key-1")
	if peer == nil {
		t.Fatal("expected peer, got nil")
	}
	if peer.ClientIP != "10.8.0.2" {
		t.Errorf("expected 10.8.0.2, got %s", peer.ClientIP)
	}

	// Non-existent peer
	if m.GetPeer("no-such-key") != nil {
		t.Error("expected nil for non-existent peer")
	}
}

func TestCleanExpired(t *testing.T) {
	pool, _ := newIPPool("10.8.0.0/24")
	m := &Manager{
		peers:  make(map[string]*Peer),
		ipPool: pool,
		cfg:    Config{Interface: "wg-test"},
	}

	now := time.Now()

	// Add an expired peer
	m.peers["expired-key"] = &Peer{
		PublicKey:  "expired-key",
		ClientIP:   "10.8.0.2",
		AssignedAt: now.Add(-2 * time.Hour),
		ExpiresAt:  now.Add(-1 * time.Hour),
	}
	pool.allocated["10.8.0.2"] = true

	// Add a valid peer
	m.peers["valid-key"] = &Peer{
		PublicKey:  "valid-key",
		ClientIP:   "10.8.0.3",
		AssignedAt: now,
		ExpiresAt:  now.Add(1 * time.Hour),
	}
	pool.allocated["10.8.0.3"] = true

	// CleanExpired will call wgRemovePeer which will fail (no real WG),
	// but it ignores the error with _ =
	removed := m.CleanExpired()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	if m.PeerCount() != 1 {
		t.Errorf("expected 1 peer remaining, got %d", m.PeerCount())
	}

	if m.GetPeer("expired-key") != nil {
		t.Error("expired peer should be removed")
	}
	if m.GetPeer("valid-key") == nil {
		t.Error("valid peer should still exist")
	}
}

func TestTruncateKey(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"abcdefghijklmnop", "abcdefgh..."},
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678..."},
		{"", ""},
	}

	for _, tt := range tests {
		got := truncateKey(tt.input)
		if got != tt.expected {
			t.Errorf("truncateKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNewManagerValidConfig(t *testing.T) {
	m, err := NewManager(Config{
		Interface:       "wg0",
		ServerPublicKey: "test-pub-key",
		ServerEndpoint:  "1.2.3.4:51820",
		Subnet:          "10.8.0.0/24",
		DNS:             "1.1.1.1",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if m.PeerCount() != 0 {
		t.Errorf("new manager should have 0 peers")
	}
}

func TestNewManagerInvalidSubnet(t *testing.T) {
	_, err := NewManager(Config{
		Subnet: "invalid",
	})
	if err == nil {
		t.Error("expected error for invalid subnet")
	}
}
