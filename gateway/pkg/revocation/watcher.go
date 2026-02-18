// Package revocation watches for ERC-1155 transfer events and revokes
// VPN sessions when NFTs are transferred away from authenticated wallets.
//
// Subscribes to TransferSingle and TransferBatch events on the Memes contract
// via WebSocket. When a transfer is detected, it:
//  1. Invalidates the NFT check cache for the sender
//  2. Revokes the sender's VPN session
//  3. Removes their WireGuard peer
package revocation

import (
	"context"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ERC-1155 event signatures (keccak256)
var (
	// TransferSingle(address indexed operator, address indexed from, address indexed to, uint256 id, uint256 value)
	transferSingleSig = common.HexToHash("0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62")
	// TransferBatch(address indexed operator, address indexed from, address indexed to, uint256[] ids, uint256[] values)
	transferBatchSig = common.HexToHash("0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb")
)

// SessionRevoker is called when an NFT transfer is detected.
type SessionRevoker interface {
	// InvalidateAndRevoke invalidates cache and revokes session for a wallet.
	InvalidateAndRevoke(wallet common.Address)
}

// Watcher monitors ERC-1155 transfer events for real-time session revocation.
type Watcher struct {
	client        *ethclient.Client
	memesContract common.Address
	revoker       SessionRevoker
	erc1155ABI    abi.ABI
	cancel        context.CancelFunc
}

const erc1155EventABI = `[{
	"anonymous": false,
	"inputs": [
		{"indexed": true, "name": "operator", "type": "address"},
		{"indexed": true, "name": "from", "type": "address"},
		{"indexed": true, "name": "to", "type": "address"},
		{"indexed": false, "name": "id", "type": "uint256"},
		{"indexed": false, "name": "value", "type": "uint256"}
	],
	"name": "TransferSingle",
	"type": "event"
},{
	"anonymous": false,
	"inputs": [
		{"indexed": true, "name": "operator", "type": "address"},
		{"indexed": true, "name": "from", "type": "address"},
		{"indexed": true, "name": "to", "type": "address"},
		{"indexed": false, "name": "ids", "type": "uint256[]"},
		{"indexed": false, "name": "values", "type": "uint256[]"}
	],
	"name": "TransferBatch",
	"type": "event"
}]`

// NewWatcher creates a transfer event watcher.
// The wsURL should be a WebSocket Ethereum RPC endpoint (wss://).
func NewWatcher(wsURL string, memesContract common.Address, revoker SessionRevoker) (*Watcher, error) {
	client, err := ethclient.Dial(wsURL)
	if err != nil {
		return nil, err
	}

	parsedABI, err := abi.JSON(strings.NewReader(erc1155EventABI))
	if err != nil {
		return nil, err
	}

	return &Watcher{
		client:        client,
		memesContract: memesContract,
		revoker:       revoker,
		erc1155ABI:    parsedABI,
	}, nil
}

// Start begins watching for transfer events. Blocks until context is cancelled.
// Automatically reconnects on errors.
func (w *Watcher) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := w.subscribe(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return // context cancelled
				}
				log.Printf("[revocation] Subscription error, reconnecting in 10s: %v", err)
				time.Sleep(10 * time.Second)
			}
		}
	}
}

// Stop cancels the watcher.
func (w *Watcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.client.Close()
}

func (w *Watcher) subscribe(ctx context.Context) error {
	query := ethereum.FilterQuery{
		Addresses: []common.Address{w.memesContract},
		Topics: [][]common.Hash{
			{transferSingleSig, transferBatchSig},
		},
	}

	logs := make(chan types.Log)
	sub, err := w.client.SubscribeFilterLogs(ctx, query, logs)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	log.Printf("[revocation] Watching %s for ERC-1155 transfers", w.memesContract.Hex())

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-sub.Err():
			return err
		case vLog := <-logs:
			w.handleLog(vLog)
		}
	}
}

func (w *Watcher) handleLog(vLog types.Log) {
	// ERC-1155 events have 4 topics: [sig, operator(indexed), from(indexed), to(indexed)]
	if len(vLog.Topics) < 4 {
		return
	}

	// Topics[1] = indexed "operator"
	// Topics[2] = indexed "from" address
	// Topics[3] = indexed "to" address
	from := common.BytesToAddress(vLog.Topics[2].Bytes())
	to := common.BytesToAddress(vLog.Topics[3].Bytes())

	zeroAddr := common.Address{}

	switch vLog.Topics[0] {
	case transferSingleSig:
		id := new(big.Int)
		if len(vLog.Data) >= 32 {
			id.SetBytes(vLog.Data[:32])
		}
		log.Printf("[revocation] TransferSingle: from=%s to=%s tokenId=%s",
			truncAddr(from), truncAddr(to), id.String())

	case transferBatchSig:
		log.Printf("[revocation] TransferBatch: from=%s to=%s",
			truncAddr(from), truncAddr(to))
	}

	// Revoke the sender's session (they no longer hold the NFT)
	if from != zeroAddr {
		log.Printf("[revocation] Revoking session for sender: %s", from.Hex())
		w.revoker.InvalidateAndRevoke(from)
	}

	// Also invalidate cache for the receiver (they now have new NFTs,
	// might upgrade tier)
	if to != zeroAddr {
		w.revoker.InvalidateAndRevoke(to)
	}
}

func truncAddr(addr common.Address) string {
	hex := addr.Hex()
	if len(hex) > 10 {
		return hex[:6] + "..." + hex[len(hex)-4:]
	}
	return hex
}
