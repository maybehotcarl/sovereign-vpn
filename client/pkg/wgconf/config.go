// Package wgconf generates WireGuard configuration files.
package wgconf

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/curve25519"
)

// KeyPair holds a WireGuard private/public key pair.
type KeyPair struct {
	PrivateKey string
	PublicKey  string
}

// GenerateKeyPair generates a new WireGuard key pair.
func GenerateKeyPair() (*KeyPair, error) {
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return nil, fmt.Errorf("generating random bytes: %w", err)
	}

	// Clamp the private key per Curve25519
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	return &KeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(privateKey[:]),
		PublicKey:  base64.StdEncoding.EncodeToString(publicKey[:]),
	}, nil
}

// Config holds all values needed to write a WireGuard config file.
type Config struct {
	PrivateKey      string
	ClientAddress   string
	DNS             string
	ServerPublicKey string
	ServerEndpoint  string
	AllowedIPs      string
}

// WriteFile writes a wg-quick compatible configuration file.
func (c *Config) WriteFile(path string) error {
	content := c.String()
	return os.WriteFile(path, []byte(content), 0600)
}

// String returns the wg-quick configuration as a string.
func (c *Config) String() string {
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
DNS = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s
PersistentKeepalive = 25
`, c.PrivateKey, c.ClientAddress, c.DNS, c.ServerPublicKey, c.ServerEndpoint, c.AllowedIPs)
}
