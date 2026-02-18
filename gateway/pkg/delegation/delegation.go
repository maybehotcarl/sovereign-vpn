// Package delegation checks delegation registries to support cold wallet users.
//
// When a user authenticates with a hot wallet, this package checks if that
// hot wallet is an authorized delegate for a cold wallet that holds Memes cards.
// Supports two delegation registries:
//
//  1. delegate.xyz v2 (https://delegate.xyz) — the universal delegation standard
//  2. 6529 Delegation (https://github.com/6529-Collections/nftdelegation) — 6529-native
//
// The checker tries both registries and returns the delegating wallet(s) found.
package delegation

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

// DelegateXYZV2 is the delegate.xyz v2 registry address (same on all chains).
// https://docs.delegate.xyz/technical-documentation/delegate-registry/contract-addresses
var DelegateXYZV2 = common.HexToAddress("0x00000000000000447e69651d841bD8D104Bed493")

// Registry6529 is the 6529 Collections delegation contract on mainnet.
// https://github.com/6529-Collections/nftdelegation
var Registry6529 = common.HexToAddress("0x2202CB9c00487e7e8EF21e6d8E914B32e709f43d")

// Config holds delegation checker configuration.
type Config struct {
	// Ethereum client (shared with nftcheck)
	Client *ethclient.Client

	// Which registries to check
	EnableDelegateXYZ bool
	Enable6529        bool

	// The Memes contract address (for contract-scoped delegation queries)
	MemesContract common.Address

	// Cache TTL for delegation lookups
	CacheTTL time.Duration
}

// Checker queries delegation registries to find cold wallets that have
// delegated to a given hot wallet.
type Checker struct {
	client        *ethclient.Client
	memesContract common.Address
	enableDXYZ    bool
	enable6529    bool
	dxyzAddr      common.Address
	r6529Addr     common.Address
	dxyzABI       abi.ABI
	r6529ABI      abi.ABI
	cacheTTL      time.Duration
	mu            sync.RWMutex
	cache         map[common.Address]cacheEntry
}

type cacheEntry struct {
	vaults    []common.Address
	expiresAt time.Time
}

// delegate.xyz v2 ABI: checkDelegateForContract(address delegate, address vault, address contract_) → bool
const delegateXYZABIJSON = `[{
	"inputs": [
		{"name": "delegate", "type": "address"},
		{"name": "vault", "type": "address"},
		{"name": "contract_", "type": "address"}
	],
	"name": "checkDelegateForContract",
	"outputs": [{"name": "", "type": "bool"}],
	"stateMutability": "view",
	"type": "function"
},{
	"inputs": [
		{"name": "delegate", "type": "address"}
	],
	"name": "getIncomingDelegations",
	"outputs": [{
		"components": [
			{"name": "type_", "type": "uint8"},
			{"name": "to", "type": "address"},
			{"name": "from", "type": "address"},
			{"name": "rights", "type": "bytes32"},
			{"name": "contract_", "type": "address"},
			{"name": "tokenId", "type": "uint256"},
			{"name": "amount", "type": "uint256"}
		],
		"name": "delegations",
		"type": "tuple[]"
	}],
	"stateMutability": "view",
	"type": "function"
},{
	"inputs": [
		{"name": "delegate", "type": "address"}
	],
	"name": "getDelegatesForAll",
	"outputs": [{"name": "", "type": "address[]"}],
	"stateMutability": "view",
	"type": "function"
}]`

// 6529 delegation ABI: retrieveDelegationAddresses
// The 6529 contract uses a different interface. The key function checks
// if a hot wallet is delegated by a cold wallet for a specific use case.
const registry6529ABIJSON = `[{
	"inputs": [
		{"name": "_delegationAddress", "type": "address"},
		{"name": "_collectionAddress", "type": "address"},
		{"name": "_useCase", "type": "uint256"}
	],
	"name": "retrieveDelegationAddresses",
	"outputs": [{"name": "", "type": "address[]"}],
	"stateMutability": "view",
	"type": "function"
}]`

// Use case 1 = general delegation in the 6529 delegation contract.
var useCase6529General = big.NewInt(1)

// NewChecker creates a delegation checker.
func NewChecker(cfg Config) (*Checker, error) {
	dxyzABI, err := abi.JSON(strings.NewReader(delegateXYZABIJSON))
	if err != nil {
		return nil, fmt.Errorf("parsing delegate.xyz ABI: %w", err)
	}

	r6529ABI, err := abi.JSON(strings.NewReader(registry6529ABIJSON))
	if err != nil {
		return nil, fmt.Errorf("parsing 6529 delegation ABI: %w", err)
	}

	c := &Checker{
		client:        cfg.Client,
		memesContract: cfg.MemesContract,
		enableDXYZ:    cfg.EnableDelegateXYZ,
		enable6529:    cfg.Enable6529,
		dxyzAddr:      DelegateXYZV2,
		r6529Addr:     Registry6529,
		dxyzABI:       dxyzABI,
		r6529ABI:      r6529ABI,
		cacheTTL:      cfg.CacheTTL,
		cache:         make(map[common.Address]cacheEntry),
	}

	if c.cacheTTL == 0 {
		c.cacheTTL = 5 * time.Minute
	}

	go c.cleanup()
	return c, nil
}

// FindVaults returns all cold wallet addresses that have delegated to the
// given hot wallet. Returns an empty slice if no delegations are found.
func (c *Checker) FindVaults(ctx context.Context, hotWallet common.Address) ([]common.Address, error) {
	// Check cache first
	c.mu.RLock()
	if entry, ok := c.cache[hotWallet]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.vaults, nil
	}
	c.mu.RUnlock()

	var allVaults []common.Address

	if c.enable6529 {
		vaults, err := c.find6529Vaults(ctx, hotWallet)
		if err != nil {
			log.Printf("[delegation] 6529 registry check failed for %s: %v", hotWallet.Hex(), err)
		} else {
			allVaults = append(allVaults, vaults...)
		}
	}

	if c.enableDXYZ {
		vaults, err := c.findDelegateXYZVaults(ctx, hotWallet)
		if err != nil {
			log.Printf("[delegation] delegate.xyz check failed for %s: %v", hotWallet.Hex(), err)
		} else {
			allVaults = append(allVaults, vaults...)
		}
	}

	// Deduplicate
	allVaults = dedupe(allVaults)

	// Cache the result
	c.mu.Lock()
	c.cache[hotWallet] = cacheEntry{
		vaults:    allVaults,
		expiresAt: time.Now().Add(c.cacheTTL),
	}
	c.mu.Unlock()

	return allVaults, nil
}

// find6529Vaults queries the 6529 delegation contract.
func (c *Checker) find6529Vaults(ctx context.Context, hotWallet common.Address) ([]common.Address, error) {
	// retrieveDelegationAddresses(hotWallet, memesContract, useCase=1)
	callData, err := c.r6529ABI.Pack("retrieveDelegationAddresses",
		hotWallet, c.memesContract, useCase6529General)
	if err != nil {
		return nil, fmt.Errorf("packing 6529 call: %w", err)
	}

	output, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &c.r6529Addr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling 6529 registry: %w", err)
	}

	results, err := c.r6529ABI.Unpack("retrieveDelegationAddresses", output)
	if err != nil {
		return nil, fmt.Errorf("unpacking 6529 response: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	addrs, ok := results[0].([]common.Address)
	if !ok {
		return nil, fmt.Errorf("unexpected type from 6529 registry: %T", results[0])
	}

	return addrs, nil
}

// findDelegateXYZVaults queries the delegate.xyz v2 registry for incoming delegations.
func (c *Checker) findDelegateXYZVaults(ctx context.Context, hotWallet common.Address) ([]common.Address, error) {
	// getIncomingDelegations(hotWallet) returns Delegation[] structs
	callData, err := c.dxyzABI.Pack("getIncomingDelegations", hotWallet)
	if err != nil {
		return nil, fmt.Errorf("packing delegate.xyz call: %w", err)
	}

	output, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &c.dxyzAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling delegate.xyz: %w", err)
	}

	results, err := c.dxyzABI.Unpack("getIncomingDelegations", output)
	if err != nil {
		return nil, fmt.Errorf("unpacking delegate.xyz response: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	// The result is a slice of structs. Each struct has a "from" field (the vault).
	// The ABI decoder returns []struct{...} as an interface.
	type delegation struct {
		Type_    uint8
		To       common.Address
		From     common.Address
		Rights   [32]byte
		Contract common.Address
		TokenId  *big.Int
		Amount   *big.Int
	}

	delegations, ok := results[0].([]struct {
		Type_    uint8          `json:"type_"`
		To       common.Address `json:"to"`
		From     common.Address `json:"from"`
		Rights   [32]byte       `json:"rights"`
		Contract common.Address `json:"contract_"`
		TokenId  *big.Int       `json:"tokenId"`
		Amount   *big.Int       `json:"amount"`
	})
	if !ok {
		return nil, fmt.Errorf("unexpected delegation type: %T", results[0])
	}

	var vaults []common.Address
	emptyAddr := common.Address{}
	for _, d := range delegations {
		// Filter: only delegations for the Memes contract or for all contracts (type 1 = ALL, type 2 = CONTRACT)
		if d.Type_ == 1 || // ALL delegation
			(d.Type_ == 2 && d.Contract == c.memesContract) { // CONTRACT-scoped
			if d.From != emptyAddr {
				vaults = append(vaults, d.From)
			}
		}
	}

	return vaults, nil
}

// Invalidate removes cached delegation data for a hot wallet.
func (c *Checker) Invalidate(hotWallet common.Address) {
	c.mu.Lock()
	delete(c.cache, hotWallet)
	c.mu.Unlock()
}

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

func dedupe(addrs []common.Address) []common.Address {
	seen := make(map[common.Address]bool, len(addrs))
	result := make([]common.Address, 0, len(addrs))
	for _, a := range addrs {
		if !seen[a] {
			seen[a] = true
			result = append(result, a)
		}
	}
	return result
}
