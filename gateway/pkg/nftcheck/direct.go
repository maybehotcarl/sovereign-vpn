package nftcheck

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// DirectChecker queries an ERC-1155 contract's balanceOfBatch directly,
// without needing a deployed AccessPolicy contract. This is the preferred
// mode for mainnet where we check against the real Memes contract.
type DirectChecker struct {
	client      *ethclient.Client
	memesAddr   common.Address
	erc1155ABI  abi.ABI
	thisCardID  int64    // token ID that grants free tier
	maxTokenID  int64    // highest token ID to check
	cacheTTL    time.Duration
	delegation  DelegationFinder

	mu    sync.RWMutex
	cache map[common.Address]cacheEntry
}

// ERC-1155 balanceOfBatch: check multiple token IDs for one address in a single call
const erc1155ABIJSON = `[
	{
		"inputs": [
			{"name": "accounts", "type": "address[]"},
			{"name": "ids", "type": "uint256[]"}
		],
		"name": "balanceOfBatch",
		"outputs": [{"name": "", "type": "uint256[]"}],
		"stateMutability": "view",
		"type": "function"
	}
]`

// NewDirectChecker creates a checker that queries the Memes ERC-1155 contract directly.
//   - memesContract: the ERC-1155 contract address (mainnet: 0x33FD426905F149f8376e227d0C9D3340AaD17aF1)
//   - thisCardID: token ID that grants free tier (0 = no free tier)
//   - maxTokenID: highest token ID to check (e.g. 300 for ~300 Memes cards)
func NewDirectChecker(rpcURL, memesContract string, thisCardID, maxTokenID int64, cacheTTL time.Duration) (*DirectChecker, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to Ethereum RPC: %w", err)
	}

	parsed, err := abi.JSON(strings.NewReader(erc1155ABIJSON))
	if err != nil {
		return nil, fmt.Errorf("parsing ERC-1155 ABI: %w", err)
	}

	c := &DirectChecker{
		client:     client,
		memesAddr:  common.HexToAddress(memesContract),
		erc1155ABI: parsed,
		thisCardID: thisCardID,
		maxTokenID: maxTokenID,
		cacheTTL:   cacheTTL,
		cache:      make(map[common.Address]cacheEntry),
	}

	go c.cleanup()
	return c, nil
}

// SetDelegation configures a delegation finder for cold wallet lookups.
func (c *DirectChecker) SetDelegation(d DelegationFinder) {
	c.delegation = d
}

// Check queries the Memes contract for a wallet's access tier.
func (c *DirectChecker) Check(ctx context.Context, wallet common.Address) (CheckResult, error) {
	// Check cache
	c.mu.RLock()
	if entry, ok := c.cache[wallet]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.result, nil
	}
	c.mu.RUnlock()

	tier, err := c.checkDirect(ctx, wallet)
	if err != nil {
		return CheckResult{}, err
	}

	// If denied and delegation configured, check vaults
	if tier == TierDenied && c.delegation != nil {
		vaults, err := c.delegation.FindVaults(ctx, wallet)
		if err != nil {
			log.Printf("[nftcheck-direct] delegation lookup failed for %s: %v", wallet.Hex(), err)
		}
		for _, vault := range vaults {
			vaultTier, err := c.checkDirect(ctx, vault)
			if err != nil {
				log.Printf("[nftcheck-direct] vault check failed for %s: %v", vault.Hex(), err)
				continue
			}
			if vaultTier > tier {
				tier = vaultTier
				log.Printf("[nftcheck-direct] delegation: %s delegates from %s (tier=%s)",
					wallet.Hex(), vault.Hex(), tier)
			}
			if tier == TierFree {
				break
			}
		}
	}

	result := CheckResult{Tier: tier, CheckedAt: time.Now()}

	c.mu.Lock()
	c.cache[wallet] = cacheEntry{result: result, expiresAt: time.Now().Add(c.cacheTTL)}
	c.mu.Unlock()

	return result, nil
}

// checkDirect calls balanceOfBatch to check token ownership.
// We batch check in groups of 50 to stay within gas limits.
func (c *DirectChecker) checkDirect(ctx context.Context, wallet common.Address) (AccessTier, error) {
	hasThisCard := false
	hasAnyCard := false

	batchSize := int64(50)
	for start := int64(1); start <= c.maxTokenID; start += batchSize {
		end := start + batchSize - 1
		if end > c.maxTokenID {
			end = c.maxTokenID
		}

		count := end - start + 1
		accounts := make([]common.Address, count)
		ids := make([]*big.Int, count)
		for i := int64(0); i < count; i++ {
			accounts[i] = wallet
			ids[i] = big.NewInt(start + i)
		}

		callData, err := c.erc1155ABI.Pack("balanceOfBatch", accounts, ids)
		if err != nil {
			return TierDenied, fmt.Errorf("packing balanceOfBatch: %w", err)
		}

		output, err := c.client.CallContract(ctx, ethereum.CallMsg{
			To:   &c.memesAddr,
			Data: callData,
		}, nil)
		if err != nil {
			return TierDenied, fmt.Errorf("calling balanceOfBatch: %w", err)
		}

		results, err := c.erc1155ABI.Unpack("balanceOfBatch", output)
		if err != nil {
			return TierDenied, fmt.Errorf("unpacking balanceOfBatch: %w", err)
		}

		balances, ok := results[0].([]*big.Int)
		if !ok {
			return TierDenied, fmt.Errorf("unexpected type for balances: %T", results[0])
		}

		for i, bal := range balances {
			if bal.Sign() > 0 {
				hasAnyCard = true
				tokenID := start + int64(i)
				if tokenID == c.thisCardID {
					hasThisCard = true
				}
			}
		}

		// Early exit if we already found the best tier
		if hasThisCard {
			return TierFree, nil
		}
	}

	if hasAnyCard {
		return TierPaid, nil
	}
	return TierDenied, nil
}

// Invalidate removes a cached result for a wallet.
func (c *DirectChecker) Invalidate(wallet common.Address) {
	c.mu.Lock()
	delete(c.cache, wallet)
	c.mu.Unlock()
}

// CacheSize returns the number of cached entries.
func (c *DirectChecker) CacheSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Close shuts down the Ethereum client connection.
func (c *DirectChecker) Close() {
	c.client.Close()
}

func (c *DirectChecker) cleanup() {
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
