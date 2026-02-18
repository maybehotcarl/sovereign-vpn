package nftcheck

import (
	"context"
	"fmt"
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

// Checker queries the AccessPolicy contract to determine a wallet's VPN access tier.
type Checker struct {
	client       *ethclient.Client
	policyAddr   common.Address
	policyABI    abi.ABI
	cacheTTL     time.Duration
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

// Check queries the AccessPolicy contract for a wallet's access tier.
// Results are cached for cacheTTL duration.
func (c *Checker) Check(ctx context.Context, wallet common.Address) (CheckResult, error) {
	// Check cache first
	c.mu.RLock()
	if entry, ok := c.cache[wallet]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.result, nil
	}
	c.mu.RUnlock()

	// Call AccessPolicy.checkAccess(address) on-chain
	callData, err := c.policyABI.Pack("checkAccess", wallet)
	if err != nil {
		return CheckResult{}, fmt.Errorf("packing call data: %w", err)
	}

	msg := ethereum.CallMsg{
		To:   &c.policyAddr,
		Data: callData,
	}

	output, err := c.client.CallContract(ctx, msg, nil)
	if err != nil {
		return CheckResult{}, fmt.Errorf("calling AccessPolicy.checkAccess: %w", err)
	}

	// Decode the response: (bool access, bool free)
	results, err := c.policyABI.Unpack("checkAccess", output)
	if err != nil {
		return CheckResult{}, fmt.Errorf("unpacking response: %w", err)
	}

	if len(results) != 2 {
		return CheckResult{}, fmt.Errorf("expected 2 return values, got %d", len(results))
	}

	access, ok := results[0].(bool)
	if !ok {
		return CheckResult{}, fmt.Errorf("unexpected type for access: %T", results[0])
	}
	free, ok := results[1].(bool)
	if !ok {
		return CheckResult{}, fmt.Errorf("unexpected type for free: %T", results[1])
	}

	var tier AccessTier
	switch {
	case free:
		tier = TierFree
	case access:
		tier = TierPaid
	default:
		tier = TierDenied
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

