package subscriptionmgr

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Manager interacts with the SubscriptionManager smart contract (read-only).
// Users call subscribe() directly from the frontend — the gateway only reads state.
type Manager struct {
	client       *ethclient.Client
	contractAddr common.Address
	abi          abi.ABI
	chainID      *big.Int
}

// OnChainSubscription represents a subscription read from the smart contract.
type OnChainSubscription struct {
	User      common.Address
	Node      common.Address
	Payment   *big.Int
	StartedAt uint64
	ExpiresAt uint64
	Tier      uint8
}

// TierInfo holds tier configuration for the frontend.
type TierInfo struct {
	ID       uint8  `json:"id"`
	Price    string `json:"price_wei"`
	Duration uint64 `json:"duration_seconds"`
	Active   bool   `json:"active"`
}

const subscriptionManagerABI = `[
	{
		"inputs": [{"name": "user", "type": "address"}],
		"name": "hasActiveSubscription",
		"outputs": [{"name": "", "type": "bool"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [{"name": "user", "type": "address"}],
		"name": "getSubscription",
		"outputs": [
			{
				"components": [
					{"name": "user", "type": "address"},
					{"name": "node", "type": "address"},
					{"name": "payment", "type": "uint256"},
					{"name": "startedAt", "type": "uint256"},
					{"name": "expiresAt", "type": "uint256"},
					{"name": "tier", "type": "uint8"}
				],
				"name": "",
				"type": "tuple"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [{"name": "user", "type": "address"}],
		"name": "remainingTime",
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "getActiveTierIds",
		"outputs": [{"name": "", "type": "uint8[]"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [{"name": "", "type": "uint8"}],
		"name": "tiers",
		"outputs": [
			{"name": "price", "type": "uint256"},
			{"name": "duration", "type": "uint256"},
			{"name": "active", "type": "bool"}
		],
		"stateMutability": "view",
		"type": "function"
	}
]`

// New creates a read-only SubscriptionManager client.
func New(rpcURL, contractAddr string, chainID int64) (*Manager, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to Ethereum RPC: %w", err)
	}

	parsed, err := abi.JSON(strings.NewReader(subscriptionManagerABI))
	if err != nil {
		return nil, fmt.Errorf("parsing SubscriptionManager ABI: %w", err)
	}

	return &Manager{
		client:       client,
		contractAddr: common.HexToAddress(contractAddr),
		abi:          parsed,
		chainID:      big.NewInt(chainID),
	}, nil
}

// HasActiveSubscription checks if a user has an active subscription on-chain.
func (m *Manager) HasActiveSubscription(ctx context.Context, user common.Address) (bool, error) {
	callData, err := m.abi.Pack("hasActiveSubscription", user)
	if err != nil {
		return false, fmt.Errorf("packing call data: %w", err)
	}

	output, err := m.client.CallContract(ctx, ethereum.CallMsg{
		To:   &m.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return false, fmt.Errorf("calling hasActiveSubscription: %w", err)
	}

	results, err := m.abi.Unpack("hasActiveSubscription", output)
	if err != nil {
		return false, fmt.Errorf("unpacking hasActiveSubscription: %w", err)
	}

	active, ok := results[0].(bool)
	if !ok {
		return false, fmt.Errorf("unexpected type for bool: %T", results[0])
	}
	return active, nil
}

// GetSubscription reads a user's subscription details from the on-chain contract.
func (m *Manager) GetSubscription(ctx context.Context, user common.Address) (*OnChainSubscription, error) {
	callData, err := m.abi.Pack("getSubscription", user)
	if err != nil {
		return nil, fmt.Errorf("packing getSubscription: %w", err)
	}

	output, err := m.client.CallContract(ctx, ethereum.CallMsg{
		To:   &m.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling getSubscription: %w", err)
	}

	results, err := m.abi.Unpack("getSubscription", output)
	if err != nil {
		return nil, fmt.Errorf("unpacking getSubscription: %w", err)
	}

	s, ok := results[0].(struct {
		User      common.Address `json:"user"`
		Node      common.Address `json:"node"`
		Payment   *big.Int       `json:"payment"`
		StartedAt *big.Int       `json:"startedAt"`
		ExpiresAt *big.Int       `json:"expiresAt"`
		Tier      uint8          `json:"tier"`
	})
	if !ok {
		return nil, fmt.Errorf("unexpected type for subscription tuple: %T", results[0])
	}

	return &OnChainSubscription{
		User:      s.User,
		Node:      s.Node,
		Payment:   s.Payment,
		StartedAt: s.StartedAt.Uint64(),
		ExpiresAt: s.ExpiresAt.Uint64(),
		Tier:      s.Tier,
	}, nil
}

// RemainingTime returns the remaining subscription time in seconds (0 if expired).
func (m *Manager) RemainingTime(ctx context.Context, user common.Address) (uint64, error) {
	callData, err := m.abi.Pack("remainingTime", user)
	if err != nil {
		return 0, fmt.Errorf("packing remainingTime: %w", err)
	}

	output, err := m.client.CallContract(ctx, ethereum.CallMsg{
		To:   &m.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return 0, fmt.Errorf("calling remainingTime: %w", err)
	}

	results, err := m.abi.Unpack("remainingTime", output)
	if err != nil {
		return 0, fmt.Errorf("unpacking remainingTime: %w", err)
	}

	remaining, ok := results[0].(*big.Int)
	if !ok {
		return 0, fmt.Errorf("unexpected type for remaining time: %T", results[0])
	}
	return remaining.Uint64(), nil
}

// GetTiers fetches all active tier configurations from the contract.
func (m *Manager) GetTiers(ctx context.Context) ([]TierInfo, error) {
	// Step 1: get active tier IDs
	idsData, err := m.abi.Pack("getActiveTierIds")
	if err != nil {
		return nil, fmt.Errorf("packing getActiveTierIds: %w", err)
	}

	idsOut, err := m.client.CallContract(ctx, ethereum.CallMsg{
		To:   &m.contractAddr,
		Data: idsData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling getActiveTierIds: %w", err)
	}

	idsResults, err := m.abi.Unpack("getActiveTierIds", idsOut)
	if err != nil {
		return nil, fmt.Errorf("unpacking getActiveTierIds: %w", err)
	}

	tierIds, ok := idsResults[0].([]uint8)
	if !ok {
		return nil, fmt.Errorf("unexpected type for tier IDs: %T", idsResults[0])
	}

	// Step 2: fetch each tier config
	var result []TierInfo
	for _, id := range tierIds {
		tierData, err := m.abi.Pack("tiers", id)
		if err != nil {
			return nil, fmt.Errorf("packing tiers(%d): %w", id, err)
		}

		tierOut, err := m.client.CallContract(ctx, ethereum.CallMsg{
			To:   &m.contractAddr,
			Data: tierData,
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("calling tiers(%d): %w", id, err)
		}

		tierResults, err := m.abi.Unpack("tiers", tierOut)
		if err != nil {
			return nil, fmt.Errorf("unpacking tiers(%d): %w", id, err)
		}

		price, _ := tierResults[0].(*big.Int)
		duration, _ := tierResults[1].(*big.Int)
		active, _ := tierResults[2].(bool)

		result = append(result, TierInfo{
			ID:       id,
			Price:    price.String(),
			Duration: duration.Uint64(),
			Active:   active,
		})
	}

	return result, nil
}

// ContractAddr returns the contract address as a hex string.
func (m *Manager) ContractAddr() string {
	return m.contractAddr.Hex()
}

// ChainID returns the chain ID.
func (m *Manager) ChainID() int64 {
	return m.chainID.Int64()
}

// Close shuts down the Ethereum client.
func (m *Manager) Close() {
	m.client.Close()
}
