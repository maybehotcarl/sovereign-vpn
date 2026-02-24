// Package payoutvault provides a read-only Go client for the on-chain PayoutVault contract.
// It enables the gateway to query pending and processed payouts for node operators.
package payoutvault

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

// Client reads the PayoutVault smart contract.
type Client struct {
	client       *ethclient.Client
	contractAddr common.Address
	abi          abi.ABI
}

// ABI for the PayoutVault contract (read-only view functions).
const payoutVaultABIJSON = `[
	{
		"inputs": [{"name": "operator", "type": "address"}],
		"name": "pendingPayouts",
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [{"name": "operator", "type": "address"}],
		"name": "processedPayouts",
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "totalPending",
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "paused",
		"outputs": [{"name": "", "type": "bool"}],
		"stateMutability": "view",
		"type": "function"
	}
]`

// NewClient creates a PayoutVault reader connected to an Ethereum RPC.
func NewClient(rpcURL string, contractAddress string) (*Client, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to Ethereum RPC: %w", err)
	}

	parsed, err := abi.JSON(strings.NewReader(payoutVaultABIJSON))
	if err != nil {
		return nil, fmt.Errorf("parsing PayoutVault ABI: %w", err)
	}

	return &Client{
		client:       client,
		contractAddr: common.HexToAddress(contractAddress),
		abi:          parsed,
	}, nil
}

// GetPendingPayout returns the pending (unclaimed) payout for an operator.
func (c *Client) GetPendingPayout(ctx context.Context, operator common.Address) (*big.Int, error) {
	callData, err := c.abi.Pack("pendingPayouts", operator)
	if err != nil {
		return nil, fmt.Errorf("packing call data: %w", err)
	}

	output, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &c.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling pendingPayouts: %w", err)
	}

	results, err := c.abi.Unpack("pendingPayouts", output)
	if err != nil {
		return nil, fmt.Errorf("unpacking pendingPayouts: %w", err)
	}

	amount, ok := results[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unexpected type for pendingPayouts: %T", results[0])
	}
	return amount, nil
}

// GetProcessedPayout returns the total already-processed payout for an operator.
func (c *Client) GetProcessedPayout(ctx context.Context, operator common.Address) (*big.Int, error) {
	callData, err := c.abi.Pack("processedPayouts", operator)
	if err != nil {
		return nil, fmt.Errorf("packing call data: %w", err)
	}

	output, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &c.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling processedPayouts: %w", err)
	}

	results, err := c.abi.Unpack("processedPayouts", output)
	if err != nil {
		return nil, fmt.Errorf("unpacking processedPayouts: %w", err)
	}

	amount, ok := results[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unexpected type for processedPayouts: %T", results[0])
	}
	return amount, nil
}

// GetTotalPending returns the total pending payouts across all operators.
func (c *Client) GetTotalPending(ctx context.Context) (*big.Int, error) {
	callData, err := c.abi.Pack("totalPending")
	if err != nil {
		return nil, fmt.Errorf("packing call data: %w", err)
	}

	output, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &c.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("calling totalPending: %w", err)
	}

	results, err := c.abi.Unpack("totalPending", output)
	if err != nil {
		return nil, fmt.Errorf("unpacking totalPending: %w", err)
	}

	amount, ok := results[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unexpected type for totalPending: %T", results[0])
	}
	return amount, nil
}

// IsPaused returns whether the PayoutVault contract is currently paused.
func (c *Client) IsPaused(ctx context.Context) (bool, error) {
	callData, err := c.abi.Pack("paused")
	if err != nil {
		return false, fmt.Errorf("packing call data: %w", err)
	}

	output, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &c.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return false, fmt.Errorf("calling paused: %w", err)
	}

	results, err := c.abi.Unpack("paused", output)
	if err != nil {
		return false, fmt.Errorf("unpacking paused: %w", err)
	}

	paused, ok := results[0].(bool)
	if !ok {
		return false, fmt.Errorf("unexpected type for paused: %T", results[0])
	}
	return paused, nil
}

// ContractAddr returns the hex address of the PayoutVault contract.
func (c *Client) ContractAddr() string {
	return c.contractAddr.Hex()
}

// Close shuts down the Ethereum client.
func (c *Client) Close() {
	c.client.Close()
}
