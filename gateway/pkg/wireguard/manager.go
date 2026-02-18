// Package wireguard manages WireGuard peers for the Sovereign VPN.
// This is the Phase 0 standalone implementation. It shells out to `wg` and `ip`
// commands to manage peers on a pre-configured WireGuard interface.
//
// In Phase 1+, this may be replaced by Sentinel's service layer, but the
// interface stays the same.
package wireguard

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// PeerConfig is the WireGuard configuration returned to the client.
type PeerConfig struct {
	ServerPublicKey string `json:"server_public_key"`
	ServerEndpoint  string `json:"server_endpoint"`  // e.g. "1.2.3.4:51820"
	ClientAddress   string `json:"client_address"`   // e.g. "10.8.0.2/24"
	DNS             string `json:"dns"`              // e.g. "1.1.1.1"
	AllowedIPs      string `json:"allowed_ips"`      // e.g. "0.0.0.0/0, ::/0"
}

// Peer tracks an active WireGuard peer.
type Peer struct {
	PublicKey     string
	ClientIP      string
	AssignedAt    time.Time
	ExpiresAt     time.Time
	BytesReceived uint64
	BytesSent     uint64
}

// Config holds WireGuard manager configuration.
type Config struct {
	Interface       string // WireGuard interface name (e.g. "wg0")
	ServerPublicKey string // Server's WG public key
	ServerEndpoint  string // Public endpoint (e.g. "vpn.example.com:51820")
	Subnet          string // Client IP subnet (e.g. "10.8.0.0/24")
	DNS             string // DNS server for clients
}

// Manager handles WireGuard peer lifecycle.
type Manager struct {
	cfg   Config
	mu    sync.Mutex
	peers map[string]*Peer // keyed by client public key
	ipPool *ipPool
}

// NewManager creates a WireGuard peer manager.
func NewManager(cfg Config) (*Manager, error) {
	pool, err := newIPPool(cfg.Subnet)
	if err != nil {
		return nil, fmt.Errorf("initializing IP pool: %w", err)
	}

	return &Manager{
		cfg:    cfg,
		peers:  make(map[string]*Peer),
		ipPool: pool,
	}, nil
}

// AddPeer registers a new WireGuard peer and returns the client configuration.
func (m *Manager) AddPeer(clientPubKey string, ttl time.Duration) (*PeerConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Allocate a client IP
	clientIP, err := m.ipPool.Allocate()
	if err != nil {
		return nil, fmt.Errorf("no available IPs: %w", err)
	}

	// Add peer to WireGuard interface
	if err := m.wgSetPeer(clientPubKey, clientIP); err != nil {
		m.ipPool.Release(clientIP)
		return nil, fmt.Errorf("adding WireGuard peer: %w", err)
	}

	now := time.Now()
	m.peers[clientPubKey] = &Peer{
		PublicKey:  clientPubKey,
		ClientIP:   clientIP,
		AssignedAt: now,
		ExpiresAt:  now.Add(ttl),
	}

	log.Printf("[wireguard] Peer added: %s -> %s (expires %s)",
		truncateKey(clientPubKey), clientIP, now.Add(ttl).Format(time.RFC3339))

	return &PeerConfig{
		ServerPublicKey: m.cfg.ServerPublicKey,
		ServerEndpoint:  m.cfg.ServerEndpoint,
		ClientAddress:   clientIP + "/24",
		DNS:             m.cfg.DNS,
		AllowedIPs:      "0.0.0.0/0, ::/0",
	}, nil
}

// RemovePeer removes a WireGuard peer.
func (m *Manager) RemovePeer(clientPubKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	peer, exists := m.peers[clientPubKey]
	if !exists {
		return fmt.Errorf("peer not found: %s", truncateKey(clientPubKey))
	}

	if err := m.wgRemovePeer(clientPubKey); err != nil {
		return fmt.Errorf("removing WireGuard peer: %w", err)
	}

	m.ipPool.Release(peer.ClientIP)
	delete(m.peers, clientPubKey)

	log.Printf("[wireguard] Peer removed: %s", truncateKey(clientPubKey))
	return nil
}

// CleanExpired removes all peers whose credentials have expired.
func (m *Manager) CleanExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removed := 0
	for pubKey, peer := range m.peers {
		if now.After(peer.ExpiresAt) {
			_ = m.wgRemovePeer(pubKey)
			m.ipPool.Release(peer.ClientIP)
			delete(m.peers, pubKey)
			removed++
			log.Printf("[wireguard] Expired peer removed: %s", truncateKey(pubKey))
		}
	}
	return removed
}

// PeerCount returns the number of active peers.
func (m *Manager) PeerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.peers)
}

// GetPeer returns peer info by public key.
func (m *Manager) GetPeer(clientPubKey string) *Peer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peers[clientPubKey]
}

// StartCleanupWorker starts a background goroutine that removes expired peers.
func (m *Manager) StartCleanupWorker(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if n := m.CleanExpired(); n > 0 {
				log.Printf("[wireguard] Cleaned %d expired peers", n)
			}
		}
	}()
}

// --- WireGuard commands ---

func (m *Manager) wgSetPeer(pubKey, clientIP string) error {
	// wg set wg0 peer <pubkey> allowed-ips <clientIP>/32
	cmd := exec.Command("wg", "set", m.cfg.Interface,
		"peer", pubKey,
		"allowed-ips", clientIP+"/32",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (m *Manager) wgRemovePeer(pubKey string) error {
	// wg set wg0 peer <pubkey> remove
	cmd := exec.Command("wg", "set", m.cfg.Interface,
		"peer", pubKey, "remove",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// GenerateKeyPair generates a WireGuard keypair (for testing).
func GenerateKeyPair() (privateKey, publicKey string, err error) {
	// Generate 32 random bytes for private key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", "", err
	}
	// Clamp the private key per Curve25519
	key[0] &= 248
	key[31] &= 127
	key[31] |= 64
	privateKey = base64.StdEncoding.EncodeToString(key)

	// Derive public key using wg pubkey
	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = strings.NewReader(privateKey)
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("deriving public key: %w", err)
	}
	publicKey = strings.TrimSpace(string(out))
	return privateKey, publicKey, nil
}

func truncateKey(key string) string {
	if len(key) > 8 {
		return key[:8] + "..."
	}
	return key
}

// --- IP Pool ---

type ipPool struct {
	mu        sync.Mutex
	baseIP    net.IP
	allocated map[string]bool
	nextOctet int // Last octet to try next (2-254)
}

func newIPPool(subnet string) (*ipPool, error) {
	ip, _, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("parsing subnet %q: %w", subnet, err)
	}

	return &ipPool{
		baseIP:    ip.To4(),
		allocated: make(map[string]bool),
		nextOctet: 2, // .1 is the server, start clients at .2
	}, nil
}

func (p *ipPool) Allocate() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try 253 addresses (.2 through .254)
	for i := 0; i < 253; i++ {
		octet := ((p.nextOctet - 2 + i) % 253) + 2
		ip := fmt.Sprintf("%d.%d.%d.%d", p.baseIP[0], p.baseIP[1], p.baseIP[2], octet)
		if !p.allocated[ip] {
			p.allocated[ip] = true
			p.nextOctet = octet + 1
			return ip, nil
		}
	}

	return "", fmt.Errorf("IP pool exhausted")
}

func (p *ipPool) Release(ip string) {
	p.mu.Lock()
	delete(p.allocated, ip)
	p.mu.Unlock()
}
