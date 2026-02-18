package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config holds all gateway configuration.
type Config struct {
	// Server settings
	ListenAddr string `json:"listen_addr"` // e.g. ":8080"

	// Ethereum RPC endpoint for NFT ownership checks
	EthereumRPC string `json:"ethereum_rpc"` // e.g. "https://eth-sepolia.g.alchemy.com/v2/YOUR_KEY"

	// Memes contract address (ERC-1155)
	MemesContract string `json:"memes_contract"`

	// AccessPolicy contract address
	AccessPolicyContract string `json:"access_policy_contract"`

	// SIWE settings
	SIWEDomain        string        `json:"siwe_domain"`         // e.g. "sovereignvpn.network"
	SIWEUri           string        `json:"siwe_uri"`            // e.g. "https://sovereignvpn.network"
	ChallengeTTL      time.Duration `json:"challenge_ttl"`       // How long a challenge is valid
	NonceLength       int           `json:"nonce_length"`        // Length of random nonce (min 8)
	CredentialTTL     time.Duration `json:"credential_ttl"`      // WireGuard credential validity

	// Rate limiting
	RateLimitPerMinute int `json:"rate_limit_per_minute"` // Per-IP rate limit
}

// DefaultConfig returns a config with sensible defaults for development.
func DefaultConfig() *Config {
	return &Config{
		ListenAddr:         ":8080",
		EthereumRPC:        "https://rpc.sepolia.org",
		MemesContract:      "",
		AccessPolicyContract: "",
		SIWEDomain:         "sovereignvpn.network",
		SIWEUri:            "https://sovereignvpn.network",
		ChallengeTTL:       5 * time.Minute,
		NonceLength:        16,
		CredentialTTL:      24 * time.Hour,
		RateLimitPerMinute: 30,
	}
}

// LoadFromFile reads config from a JSON file, applying defaults for missing fields.
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}

// Validate checks that required fields are set.
func (c *Config) Validate() error {
	if c.MemesContract == "" {
		return fmt.Errorf("memes_contract is required")
	}
	if c.AccessPolicyContract == "" {
		return fmt.Errorf("access_policy_contract is required")
	}
	if c.EthereumRPC == "" {
		return fmt.Errorf("ethereum_rpc is required")
	}
	if c.NonceLength < 8 {
		return fmt.Errorf("nonce_length must be >= 8")
	}
	return nil
}
