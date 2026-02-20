package sessionmgr

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Manager interacts with the SessionManager smart contract for on-chain session tracking.
type Manager struct {
	client       *ethclient.Client
	contractAddr common.Address
	abi          abi.ABI
	key          *ecdsa.PrivateKey // nil = read-only (no writes)
	operatorAddr common.Address    // derived from key — the "node" param
	chainID      *big.Int
	mu           sync.Mutex // protects nonce management
}

const sessionManagerABI = `[
	{
		"inputs": [
			{"name": "user", "type": "address"},
			{"name": "node", "type": "address"},
			{"name": "duration", "type": "uint256"}
		],
		"name": "openFreeSession",
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [{"name": "sessionId", "type": "uint256"}],
		"name": "closeSession",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [{"name": "user", "type": "address"}],
		"name": "getActiveSessionId",
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "view",
		"type": "function"
	}
]`

// New creates a SessionManager client. If privateKeyHex is empty, operates in read-only mode.
func New(rpcURL, contractAddr, privateKeyHex string, chainID int64) (*Manager, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to Ethereum RPC: %w", err)
	}

	parsed, err := abi.JSON(strings.NewReader(sessionManagerABI))
	if err != nil {
		return nil, fmt.Errorf("parsing SessionManager ABI: %w", err)
	}

	m := &Manager{
		client:       client,
		contractAddr: common.HexToAddress(contractAddr),
		abi:          parsed,
		chainID:      big.NewInt(chainID),
	}

	if privateKeyHex != "" {
		key, err := crypto.HexToECDSA(privateKeyHex)
		if err != nil {
			return nil, fmt.Errorf("parsing private key: %w", err)
		}
		m.key = key
		m.operatorAddr = crypto.PubkeyToAddress(key.PublicKey)
	}

	return m, nil
}

// OpenFreeSession sends an openFreeSession tx in a background goroutine (fire-and-forget).
func (m *Manager) OpenFreeSession(user common.Address, durationSecs uint64) {
	if m.key == nil {
		log.Printf("[sessionmgr] Warning: read-only mode, cannot open session")
		return
	}

	go func() {
		callData, err := m.abi.Pack("openFreeSession", user, m.operatorAddr, new(big.Int).SetUint64(durationSecs))
		if err != nil {
			log.Printf("[sessionmgr] Error packing openFreeSession: %v", err)
			return
		}

		m.sendTx(callData, "openFreeSession")
	}()
}

// CloseSessionFor queries the active session ID for a user and closes it on-chain (fire-and-forget).
func (m *Manager) CloseSessionFor(user common.Address) {
	if m.key == nil {
		log.Printf("[sessionmgr] Warning: read-only mode, cannot close session")
		return
	}

	go func() {
		ctx := context.Background()
		sessionID, err := m.GetActiveSessionID(ctx, user)
		if err != nil {
			log.Printf("[sessionmgr] Error getting active session for %s: %v", user.Hex(), err)
			return
		}
		if sessionID == 0 {
			return // no active session on-chain
		}

		callData, err := m.abi.Pack("closeSession", new(big.Int).SetUint64(sessionID))
		if err != nil {
			log.Printf("[sessionmgr] Error packing closeSession: %v", err)
			return
		}

		m.sendTx(callData, "closeSession")
	}()
}

// GetActiveSessionID returns the active on-chain session ID for a user (0 = none).
func (m *Manager) GetActiveSessionID(ctx context.Context, user common.Address) (uint64, error) {
	callData, err := m.abi.Pack("getActiveSessionId", user)
	if err != nil {
		return 0, fmt.Errorf("packing call data: %w", err)
	}

	output, err := m.client.CallContract(ctx, ethereum.CallMsg{
		To:   &m.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return 0, fmt.Errorf("calling getActiveSessionId: %w", err)
	}

	results, err := m.abi.Unpack("getActiveSessionId", output)
	if err != nil {
		return 0, fmt.Errorf("unpacking getActiveSessionId: %w", err)
	}

	id, ok := results[0].(*big.Int)
	if !ok {
		return 0, fmt.Errorf("unexpected type for session ID: %T", results[0])
	}
	return id.Uint64(), nil
}

// Close shuts down the Ethereum client.
func (m *Manager) Close() {
	m.client.Close()
}

// sendTx signs and sends a transaction to the SessionManager contract.
// Must be called from a goroutine — logs errors instead of returning them.
func (m *Manager) sendTx(callData []byte, method string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx := context.Background()
	from := crypto.PubkeyToAddress(m.key.PublicKey)

	nonce, err := m.client.PendingNonceAt(ctx, from)
	if err != nil {
		log.Printf("[sessionmgr] Error getting nonce: %v", err)
		return
	}

	gasPrice, err := m.client.SuggestGasPrice(ctx)
	if err != nil {
		log.Printf("[sessionmgr] Error getting gas price: %v", err)
		return
	}

	tx := types.NewTransaction(
		nonce,
		m.contractAddr,
		big.NewInt(0),
		150000, // gas limit
		gasPrice,
		callData,
	)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(m.chainID), m.key)
	if err != nil {
		log.Printf("[sessionmgr] Error signing tx: %v", err)
		return
	}

	err = m.client.SendTransaction(ctx, signedTx)
	if err != nil {
		log.Printf("[sessionmgr] Error sending %s tx: %v", method, err)
		return
	}

	log.Printf("[sessionmgr] %s tx sent: %s", method, signedTx.Hash().Hex())
}
