package delegation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// mock6529RPC creates a mock Ethereum RPC that responds to
// retrieveDelegationAddresses calls.
func mock6529RPC(vaultForDelegate map[common.Address][]common.Address) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
			ID      int             `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")

		switch req.Method {
		case "eth_call":
			// Parse call params to extract the delegate address
			var params []json.RawMessage
			json.Unmarshal(req.Params, &params)

			var callObj struct {
				Input string `json:"input"`
				Data  string `json:"data"`
				To    string `json:"to"`
			}
			if len(params) > 0 {
				json.Unmarshal(params[0], &callObj)
			}

			callData := callObj.Input
			if callData == "" {
				callData = callObj.Data
			}

			// Check which registry is being called by the "to" address
			to := common.HexToAddress(callObj.To)

			var result []byte

			if to == Registry6529 {
				// 6529 retrieveDelegationAddresses: selector(4) + delegate(32) + collection(32) + useCase(32)
				// delegate is at bytes 4-36, padded. Extract last 20 bytes of the first param.
				var delegate common.Address
				if len(callData) >= 74 { // "0x" + 8 selector + 64 param1
					delegate = common.HexToAddress("0x" + callData[len("0x")+8+24:len("0x")+8+64])
				}

				vaults := vaultForDelegate[delegate]
				// ABI encode: offset(32) + length(32) + addresses(32 each)
				result = make([]byte, 64+len(vaults)*32)
				// Offset to array data = 32
				big.NewInt(32).FillBytes(result[0:32])
				// Array length
				big.NewInt(int64(len(vaults))).FillBytes(result[32:64])
				for i, v := range vaults {
					copy(result[64+i*32+12:64+(i+1)*32], v.Bytes())
				}
			} else {
				// delegate.xyz getIncomingDelegations - return empty array for simplicity
				result = make([]byte, 64)
				big.NewInt(32).FillBytes(result[0:32])
				// length = 0
			}

			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  "0x" + hex.EncodeToString(result),
			})

		default:
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  "0x1",
			})
		}
	}))
}

func TestFind6529Vaults(t *testing.T) {
	hotWallet := common.HexToAddress("0x1111111111111111111111111111111111111111")
	coldWallet := common.HexToAddress("0x2222222222222222222222222222222222222222")
	memesAddr := common.HexToAddress("0x33fd426905f149f8376e227d0c9d3340aad17af1")

	rpc := mock6529RPC(map[common.Address][]common.Address{
		hotWallet: {coldWallet},
	})
	defer rpc.Close()

	client, err := ethclient.Dial(rpc.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	checker, err := NewChecker(Config{
		Client:        client,
		Enable6529:    true,
		MemesContract: memesAddr,
		CacheTTL:      time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Override the 6529 registry address to point to our mock
	checker.r6529Addr = Registry6529

	vaults, err := checker.FindVaults(context.Background(), hotWallet)
	if err != nil {
		t.Fatalf("FindVaults: %v", err)
	}

	if len(vaults) != 1 {
		t.Fatalf("expected 1 vault, got %d", len(vaults))
	}
	if vaults[0] != coldWallet {
		t.Errorf("expected vault %s, got %s", coldWallet.Hex(), vaults[0].Hex())
	}
}

func TestFindVaultsNoResults(t *testing.T) {
	hotWallet := common.HexToAddress("0x3333333333333333333333333333333333333333")
	memesAddr := common.HexToAddress("0x33fd426905f149f8376e227d0c9d3340aad17af1")

	rpc := mock6529RPC(map[common.Address][]common.Address{})
	defer rpc.Close()

	client, _ := ethclient.Dial(rpc.URL)
	defer client.Close()

	checker, _ := NewChecker(Config{
		Client:        client,
		Enable6529:    true,
		MemesContract: memesAddr,
		CacheTTL:      time.Minute,
	})

	vaults, err := checker.FindVaults(context.Background(), hotWallet)
	if err != nil {
		t.Fatalf("FindVaults: %v", err)
	}
	if len(vaults) != 0 {
		t.Errorf("expected 0 vaults, got %d", len(vaults))
	}
}

func TestFindVaultsCaching(t *testing.T) {
	hotWallet := common.HexToAddress("0x4444444444444444444444444444444444444444")
	coldWallet := common.HexToAddress("0x5555555555555555555555555555555555555555")

	callCount := 0
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req struct {
			ID int `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		result := make([]byte, 64+32)
		big.NewInt(32).FillBytes(result[0:32])
		big.NewInt(1).FillBytes(result[32:64])
		copy(result[64+12:96], coldWallet.Bytes())

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  "0x" + hex.EncodeToString(result),
		})
	}))
	defer rpc.Close()

	client, _ := ethclient.Dial(rpc.URL)
	defer client.Close()

	checker, _ := NewChecker(Config{
		Client:     client,
		Enable6529: true,
		CacheTTL:   time.Minute,
	})

	// First call hits RPC
	checker.FindVaults(context.Background(), hotWallet)
	firstCount := callCount

	// Second call should be cached
	checker.FindVaults(context.Background(), hotWallet)
	if callCount != firstCount {
		t.Errorf("second call should be cached, but RPC was called %d times", callCount)
	}

	// Invalidate and call again should hit RPC
	checker.Invalidate(hotWallet)
	checker.FindVaults(context.Background(), hotWallet)
	if callCount <= firstCount {
		t.Error("after invalidation, RPC should be called again")
	}
}

func TestDedupe(t *testing.T) {
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	result := dedupe([]common.Address{addr1, addr2, addr1, addr2, addr1})
	if len(result) != 2 {
		t.Errorf("expected 2 unique addresses, got %d", len(result))
	}
}

func TestDedupeEmpty(t *testing.T) {
	result := dedupe(nil)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}
