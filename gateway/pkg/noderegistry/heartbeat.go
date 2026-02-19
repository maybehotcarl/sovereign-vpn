package noderegistry

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// HeartbeatSender sends periodic heartbeat transactions to the NodeRegistry contract.
type HeartbeatSender struct {
	client       *ethclient.Client
	contractAddr common.Address
	abi          abi.ABI
	key          *ecdsa.PrivateKey
	chainID      *big.Int
	interval     time.Duration
	stopCh       chan struct{}
}

const heartbeatABI = `[{
	"inputs": [],
	"name": "heartbeat",
	"outputs": [],
	"stateMutability": "nonpayable",
	"type": "function"
}]`

// NewHeartbeatSender creates a heartbeat sender.
func NewHeartbeatSender(rpcURL, contractAddress, privateKeyHex string, chainID int64, interval time.Duration) (*HeartbeatSender, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to Ethereum RPC: %w", err)
	}

	key, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	parsed, err := abi.JSON(strings.NewReader(heartbeatABI))
	if err != nil {
		return nil, fmt.Errorf("parsing ABI: %w", err)
	}

	return &HeartbeatSender{
		client:       client,
		contractAddr: common.HexToAddress(contractAddress),
		abi:          parsed,
		key:          key,
		chainID:      big.NewInt(chainID),
		interval:     interval,
		stopCh:       make(chan struct{}),
	}, nil
}

// Start begins the heartbeat loop. Blocks until Stop is called.
func (h *HeartbeatSender) Start(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	log.Printf("[heartbeat] Starting heartbeat sender (interval=%s)", h.interval)

	// Send initial heartbeat
	h.sendHeartbeat(ctx)

	for {
		select {
		case <-ticker.C:
			h.sendHeartbeat(ctx)
		case <-h.stopCh:
			log.Println("[heartbeat] Stopped")
			return
		case <-ctx.Done():
			log.Println("[heartbeat] Context cancelled")
			return
		}
	}
}

// Stop stops the heartbeat loop.
func (h *HeartbeatSender) Stop() {
	close(h.stopCh)
	h.client.Close()
}

func (h *HeartbeatSender) sendHeartbeat(ctx context.Context) {
	callData, err := h.abi.Pack("heartbeat")
	if err != nil {
		log.Printf("[heartbeat] Error packing call: %v", err)
		return
	}

	from := crypto.PubkeyToAddress(h.key.PublicKey)

	nonce, err := h.client.PendingNonceAt(ctx, from)
	if err != nil {
		log.Printf("[heartbeat] Error getting nonce: %v", err)
		return
	}

	gasPrice, err := h.client.SuggestGasPrice(ctx)
	if err != nil {
		log.Printf("[heartbeat] Error getting gas price: %v", err)
		return
	}

	tx := types.NewTransaction(
		nonce,
		h.contractAddr,
		big.NewInt(0),
		100000, // gas limit
		gasPrice,
		callData,
	)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(h.chainID), h.key)
	if err != nil {
		log.Printf("[heartbeat] Error signing tx: %v", err)
		return
	}

	err = h.client.SendTransaction(ctx, signedTx)
	if err != nil {
		log.Printf("[heartbeat] Error sending tx: %v", err)
		return
	}

	log.Printf("[heartbeat] Sent heartbeat tx: %s", signedTx.Hash().Hex())
}
