package nftcheck

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// AccessTier represents the user's VPN access level.
type AccessTier int

const (
	TierDenied AccessTier = iota
	TierPaid
	TierFree
)

func (t AccessTier) String() string {
	switch t {
	case TierFree:
		return "free"
	case TierPaid:
		return "paid"
	default:
		return "denied"
	}
}

// CheckResult holds the result of an NFT access check.
type CheckResult struct {
	Tier      AccessTier
	CheckedAt time.Time
}

// cacheEntry holds a cached check result.
type cacheEntry struct {
	result    CheckResult
	expiresAt time.Time
}

// DelegationFinder looks up cold wallets that have delegated to a hot wallet.
type DelegationFinder interface {
	FindVaults(ctx context.Context, hotWallet common.Address) ([]common.Address, error)
}

// Checker queries the AccessPolicy contract to determine a wallet's VPN access tier.
type Checker struct {
	client       *ethclient.Client
	policyAddr   common.Address
	policyABI    abi.ABI
	cacheTTL     time.Duration
	delegation   DelegationFinder // optional, nil if delegation not configured
	mu           sync.RWMutex
	cache        map[common.Address]cacheEntry
}

// AccessPolicy.checkAccess(address) returns (bool access, bool free)
const accessPolicyABIJSON = `[{
	"inputs": [{"name": "user", "type": "address"}],
	"name": "checkAccess",
	"outputs": [
		{"name": "access", "type": "bool"},
		{"name": "free", "type": "bool"}
	],
	"stateMutability": "view",
	"type": "function"
}]`

// NewChecker creates an NFT checker connected to an Ethereum RPC endpoint.
func NewChecker(rpcURL string, policyAddress string, cacheTTL time.Duration) (*Checker, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to Ethereum RPC: %w", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(accessPolicyABIJSON))
	if err != nil {
		return nil, fmt.Errorf("parsing ABI: %w", err)
	}

	c := &Checker{
		client:     client,
		policyAddr: common.HexToAddress(policyAddress),
		policyABI:  parsedABI,
		cacheTTL:   cacheTTL,
		cache:      make(map[common.Address]cacheEntry),
	}

	go c.cleanup()
	return c, nil
}

// SetDelegation configures a delegation finder for cold wallet lookups.
func (c *Checker) SetDelegation(d DelegationFinder) {
	c.delegation = d
}

// Check queries the AccessPolicy contract for a wallet's access tier.
// If delegation is configured and the direct check returns denied,
// it also checks cold wallets that have delegated to this wallet.
// Results are cached for cacheTTL duration.
func (c *Checker) Check(ctx context.Context, wallet common.Address) (CheckResult, error) {
	// Check cache first
	c.mu.RLock()
	if entry, ok := c.cache[wallet]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.result, nil
	}
	c.mu.RUnlock()

	// Direct on-chain check
	tier, err := c.checkOnChain(ctx, wallet)
	if err != nil {
		return CheckResult{}, err
	}

	// If direct check denied and delegation is configured, check vault wallets
	if tier == TierDenied && c.delegation != nil {
		vaults, err := c.delegation.FindVaults(ctx, wallet)
		if err != nil {
			log.Printf("[nftcheck] delegation lookup failed for %s: %v", wallet.Hex(), err)
		}
		for _, vault := range vaults {
			vaultTier, err := c.checkOnChain(ctx, vault)
			if err != nil {
				log.Printf("[nftcheck] vault check failed for %s: %v", vault.Hex(), err)
				continue
			}
			if vaultTier > tier {
				tier = vaultTier
				log.Printf("[nftcheck] delegation: %s delegates from %s (tier=%s)",
					wallet.Hex(), vault.Hex(), tier)
			}
			if tier == TierFree {
				break // best possible tier
			}
		}
	}

	result := CheckResult{
		Tier:      tier,
		CheckedAt: time.Now(),
	}

	// Cache the result
	c.mu.Lock()
	c.cache[wallet] = cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.cacheTTL),
	}
	c.mu.Unlock()

	return result, nil
}

// checkOnChain calls AccessPolicy.checkAccess(address) and returns the tier.
func (c *Checker) checkOnChain(ctx context.Context, wallet common.Address) (AccessTier, error) {
	callData, err := c.policyABI.Pack("checkAccess", wallet)
	if err != nil {
		return TierDenied, fmt.Errorf("packing call data: %w", err)
	}

	output, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &c.policyAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return TierDenied, fmt.Errorf("calling AccessPolicy.checkAccess: %w", err)
	}

	results, err := c.policyABI.Unpack("checkAccess", output)
	if err != nil {
		return TierDenied, fmt.Errorf("unpacking response: %w", err)
	}

	if len(results) != 2 {
		return TierDenied, fmt.Errorf("expected 2 return values, got %d", len(results))
	}

	access, ok := results[0].(bool)
	if !ok {
		return TierDenied, fmt.Errorf("unexpected type for access: %T", results[0])
	}
	free, ok := results[1].(bool)
	if !ok {
		return TierDenied, fmt.Errorf("unexpected type for free: %T", results[1])
	}

	switch {
	case free:
		return TierFree, nil
	case access:
		return TierPaid, nil
	default:
		return TierDenied, nil
	}
}

// Invalidate removes a cached result for a wallet (used when transfer events are detected).
func (c *Checker) Invalidate(wallet common.Address) {
	c.mu.Lock()
	delete(c.cache, wallet)
	c.mu.Unlock()
}

// CacheSize returns the number of cached entries (for monitoring).
func (c *Checker) CacheSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Close shuts down the Ethereum client connection.
func (c *Checker) Close() {
	c.client.Close()
}

// cleanup periodically removes expired cache entries.
func (c *Checker) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for addr, entry := range c.cache {
			if now.After(entry.expiresAt) {
				delete(c.cache, addr)
			}
		}
		c.mu.Unlock()
	}
}

