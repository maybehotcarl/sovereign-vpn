// Package noderegistry provides a Go client for the on-chain NodeRegistry contract.
// It enables the gateway and CLI to discover active VPN nodes.
// Reputation is checked separately via the 6529 API (rep6529 package).
package noderegistry

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Node represents a registered VPN node from the on-chain registry.
// Reputation is managed off-chain via the 6529 community rep system.
type Node struct {
	Operator      common.Address
	Endpoint      string
	WgPubKey      string
	Region        string
	StakedAmount  *big.Int
	RegisteredAt  time.Time
	LastHeartbeat time.Time
	Active        bool
	Slashed       bool
}

// Registry reads the NodeRegistry smart contract.
type Registry struct {
	client       *ethclient.Client
	contractAddr common.Address
	abi          abi.ABI
	cacheTTL     time.Duration

	mu         sync.RWMutex
	cachedList []Node
	cacheTime  time.Time
}

// ABI for the updated NodeRegistry (no reputation field in Node struct).
const nodeRegistryABIJSON = `[
	{
		"inputs": [],
		"name": "getActiveNodes",
		"outputs": [{
			"components": [
				{"name": "operator", "type": "address"},
				{"name": "endpoint", "type": "string"},
				{"name": "wgPubKey", "type": "string"},
				{"name": "region", "type": "string"},
				{"name": "stakedAmount", "type": "uint256"},
				{"name": "registeredAt", "type": "uint256"},
				{"name": "lastHeartbeat", "type": "uint256"},
				{"name": "active", "type": "bool"},
				{"name": "slashed", "type": "bool"}
			],
			"name": "",
			"type": "tuple[]"
		}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [{"name": "operator", "type": "address"}],
		"name": "getNode",
		"outputs": [{
			"components": [
				{"name": "operator", "type": "address"},
				{"name": "endpoint", "type": "string"},
				{"name": "wgPubKey", "type": "string"},
				{"name": "region", "type": "string"},
				{"name": "stakedAmount", "type": "uint256"},
				{"name": "registeredAt", "type": "uint256"},
				{"name": "lastHeartbeat", "type": "uint256"},
				{"name": "active", "type": "bool"},
				{"name": "slashed", "type": "bool"}
			],
			"name": "",
			"type": "tuple"
		}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "nodeCount",
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [{"name": "operator", "type": "address"}],
		"name": "isHeartbeatOverdue",
		"outputs": [{"name": "", "type": "bool"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [{"name": "region", "type": "string"}],
		"name": "getActiveNodesByRegion",
		"outputs": [{
			"components": [
				{"name": "operator", "type": "address"},
				{"name": "endpoint", "type": "string"},
				{"name": "wgPubKey", "type": "string"},
				{"name": "region", "type": "string"},
				{"name": "stakedAmount", "type": "uint256"},
				{"name": "registeredAt", "type": "uint256"},
				{"name": "lastHeartbeat", "type": "uint256"},
				{"name": "active", "type": "bool"},
				{"name": "slashed", "type": "bool"}
			],
			"name": "",
			"type": "tuple[]"
		}],
		"stateMutability": "view",
		"type": "function"
	}
]`

// NewRegistry creates a registry reader connected to an Ethereum RPC.
func NewRegistry(rpcURL string, contractAddress string, cacheTTL time.Duration) (*Registry, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to Ethereum RPC: %w", err)
	}

	parsed, err := abi.JSON(strings.NewReader(nodeRegistryABIJSON))
	if err != nil {
		return nil, fmt.Errorf("parsing NodeRegistry ABI: %w", err)
	}

	return &Registry{
		client:       client,
		contractAddr: common.HexToAddress(contractAddress),
		abi:          parsed,
		cacheTTL:     cacheTTL,
	}, nil
}

// GetActiveNodes returns all active nodes, with caching.
func (r *Registry) GetActiveNodes(ctx context.Context) ([]Node, error) {
	r.mu.RLock()
	if time.Since(r.cacheTime) < r.cacheTTL && r.cachedList != nil {
		list := r.cachedList
		r.mu.RUnlock()
		return list, nil
	}
	r.mu.RUnlock()

	nodes, err := r.fetchActiveNodes(ctx)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cachedList = nodes
	r.cacheTime = time.Now()
	r.mu.Unlock()

	return nodes, nil
}

// GetActiveNodesByRegion returns active nodes in a specific region.
func (r *Registry) GetActiveNodesByRegion(ctx context.Context, region string) ([]Node, error) {
	callData, err := r.abi.Pack("getActiveNodesByRegion", region)
	if err != nil {
		return nil, fmt.Errorf("packing call data: %w", err)
	}

	output, err := r.client.CallContract(ctx, ethereum.CallMsg{
		To:   &r.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling getActiveNodesByRegion: %w", err)
	}

	return r.decodeNodeArray("getActiveNodesByRegion", output)
}

// GetNode returns a specific node's data.
func (r *Registry) GetNode(ctx context.Context, operator common.Address) (*Node, error) {
	callData, err := r.abi.Pack("getNode", operator)
	if err != nil {
		return nil, fmt.Errorf("packing call data: %w", err)
	}

	output, err := r.client.CallContract(ctx, ethereum.CallMsg{
		To:   &r.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling getNode: %w", err)
	}

	results, err := r.abi.Unpack("getNode", output)
	if err != nil {
		return nil, fmt.Errorf("unpacking getNode: %w", err)
	}

	if len(results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results))
	}

	return decodeNodeStruct(results[0])
}

// NodeCount returns the total number of registered nodes.
func (r *Registry) NodeCount(ctx context.Context) (uint64, error) {
	callData, err := r.abi.Pack("nodeCount")
	if err != nil {
		return 0, fmt.Errorf("packing call data: %w", err)
	}

	output, err := r.client.CallContract(ctx, ethereum.CallMsg{
		To:   &r.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return 0, fmt.Errorf("calling nodeCount: %w", err)
	}

	results, err := r.abi.Unpack("nodeCount", output)
	if err != nil {
		return 0, fmt.Errorf("unpacking nodeCount: %w", err)
	}

	count, ok := results[0].(*big.Int)
	if !ok {
		return 0, fmt.Errorf("unexpected type for nodeCount: %T", results[0])
	}
	return count.Uint64(), nil
}

// IsHeartbeatOverdue checks if a node's heartbeat is overdue.
func (r *Registry) IsHeartbeatOverdue(ctx context.Context, operator common.Address) (bool, error) {
	callData, err := r.abi.Pack("isHeartbeatOverdue", operator)
	if err != nil {
		return false, fmt.Errorf("packing call data: %w", err)
	}

	output, err := r.client.CallContract(ctx, ethereum.CallMsg{
		To:   &r.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return false, fmt.Errorf("calling isHeartbeatOverdue: %w", err)
	}

	results, err := r.abi.Unpack("isHeartbeatOverdue", output)
	if err != nil {
		return false, fmt.Errorf("unpacking isHeartbeatOverdue: %w", err)
	}

	overdue, ok := results[0].(bool)
	if !ok {
		return false, fmt.Errorf("unexpected type: %T", results[0])
	}
	return overdue, nil
}

// InvalidateCache forces the next GetActiveNodes call to re-fetch from chain.
func (r *Registry) InvalidateCache() {
	r.mu.Lock()
	r.cachedList = nil
	r.cacheTime = time.Time{}
	r.mu.Unlock()
}

// Close shuts down the Ethereum client.
func (r *Registry) Close() {
	r.client.Close()
}

// fetchActiveNodes calls the contract and returns active nodes.
func (r *Registry) fetchActiveNodes(ctx context.Context) ([]Node, error) {
	callData, err := r.abi.Pack("getActiveNodes")
	if err != nil {
		return nil, fmt.Errorf("packing call data: %w", err)
	}

	output, err := r.client.CallContract(ctx, ethereum.CallMsg{
		To:   &r.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling getActiveNodes: %w", err)
	}

	return r.decodeNodeArray("getActiveNodes", output)
}

// decodeNodeArray unpacks a tuple[] of Node structs.
func (r *Registry) decodeNodeArray(method string, output []byte) ([]Node, error) {
	results, err := r.abi.Unpack(method, output)
	if err != nil {
		return nil, fmt.Errorf("unpacking %s: %w", method, err)
	}

	if len(results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results))
	}

	// The result is a slice of structs
	rawNodes, ok := results[0].([]struct {
		Operator      common.Address `json:"operator"`
		Endpoint      string         `json:"endpoint"`
		WgPubKey      string         `json:"wgPubKey"`
		Region        string         `json:"region"`
		StakedAmount  *big.Int       `json:"stakedAmount"`
		RegisteredAt  *big.Int       `json:"registeredAt"`
		LastHeartbeat *big.Int       `json:"lastHeartbeat"`
		Active        bool           `json:"active"`
		Slashed       bool           `json:"slashed"`
	})
	if !ok {
		return nil, fmt.Errorf("unexpected type for node array: %T", results[0])
	}

	nodes := make([]Node, len(rawNodes))
	for i, raw := range rawNodes {
		nodes[i] = Node{
			Operator:      raw.Operator,
			Endpoint:      raw.Endpoint,
			WgPubKey:      raw.WgPubKey,
			Region:        raw.Region,
			StakedAmount:  raw.StakedAmount,
			RegisteredAt:  time.Unix(raw.RegisteredAt.Int64(), 0),
			LastHeartbeat: time.Unix(raw.LastHeartbeat.Int64(), 0),
			Active:        raw.Active,
			Slashed:       raw.Slashed,
		}
	}
	return nodes, nil
}

// decodeNodeStruct unpacks a single Node struct from ABI output.
func decodeNodeStruct(raw interface{}) (*Node, error) {
	s, ok := raw.(struct {
		Operator      common.Address `json:"operator"`
		Endpoint      string         `json:"endpoint"`
		WgPubKey      string         `json:"wgPubKey"`
		Region        string         `json:"region"`
		StakedAmount  *big.Int       `json:"stakedAmount"`
		RegisteredAt  *big.Int       `json:"registeredAt"`
		LastHeartbeat *big.Int       `json:"lastHeartbeat"`
		Active        bool           `json:"active"`
		Slashed       bool           `json:"slashed"`
	})
	if !ok {
		return nil, fmt.Errorf("unexpected node struct type: %T", raw)
	}

	return &Node{
		Operator:      s.Operator,
		Endpoint:      s.Endpoint,
		WgPubKey:      s.WgPubKey,
		Region:        s.Region,
		StakedAmount:  s.StakedAmount,
		RegisteredAt:  time.Unix(s.RegisteredAt.Int64(), 0),
		LastHeartbeat: time.Unix(s.LastHeartbeat.Int64(), 0),
		Active:        s.Active,
		Slashed:       s.Slashed,
	}, nil
}
