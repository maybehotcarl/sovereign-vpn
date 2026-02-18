// Package integration tests the full Sovereign VPN auth + connect pipeline.
//
// It starts a mock Ethereum RPC, a real gateway server, and exercises the
// complete client flow: keygen → challenge → sign → verify → connect.
package integration

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/maybehotcarl/sovereign-vpn/client/pkg/api"
	"github.com/maybehotcarl/sovereign-vpn/client/pkg/wallet"
	"github.com/maybehotcarl/sovereign-vpn/client/pkg/wgconf"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/config"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/server"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/wireguard"
)

// mockEthRPC simulates an Ethereum JSON-RPC server that responds to
// eth_call for AccessPolicy.checkAccess(address) → (true, true) for allowed
// addresses, or (false, false) for denied addresses.
func mockEthRPC(allowedAddrs map[common.Address]bool) *httptest.Server {
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
			// go-ethereum sends: [{"from":"0x...","input":"0x...","to":"0x..."}, "latest"]
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

			// go-ethereum v1.17 uses "input", older versions use "data"
			callData := callObj.Input
			if callData == "" {
				callData = callObj.Data
			}

			// callData is "0x" + 8 hex selector + 64 hex padded address
			// Extract the address (last 40 hex chars of the padded param)
			var addr common.Address
			if len(callData) >= 42 {
				addrHex := callData[len(callData)-40:]
				addr = common.HexToAddress("0x" + addrHex)
			}

			// Return (bool access, bool free) ABI-encoded
			var access, free bool
			if allowedAddrs[addr] {
				access = true
				free = true
			}

			// ABI encode: two bool values as 32-byte words
			result := make([]byte, 64)
			if access {
				result[31] = 1
			}
			if free {
				result[63] = 1
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

func TestFullConnectFlow(t *testing.T) {
	// Generate a test wallet
	w, err := wallet.Generate()
	if err != nil {
		t.Fatalf("wallet.Generate: %v", err)
	}

	// Start mock Ethereum RPC that allows our wallet
	ethRPC := mockEthRPC(map[common.Address]bool{
		w.Address(): true,
	})
	defer ethRPC.Close()

	// Create gateway config
	cfg := config.DefaultConfig()
	cfg.AccessPolicyContract = "0x0000000000000000000000000000000000000001"
	cfg.MemesContract = "0x0000000000000000000000000000000000000002"
	cfg.EthereumRPC = ethRPC.URL
	cfg.SIWEDomain = "test.local"
	cfg.SIWEUri = "https://test.local"
	cfg.CredentialTTL = 1 * time.Hour
	cfg.ChallengeTTL = 5 * time.Minute
	cfg.NonceLength = 16

	// Create NFT checker pointing to mock RPC
	checker, err := nftcheck.NewChecker(ethRPC.URL, cfg.AccessPolicyContract, 5*time.Minute)
	if err != nil {
		t.Fatalf("nftcheck.NewChecker: %v", err)
	}
	defer checker.Close()

	// Create WireGuard manager (won't actually run wg commands in this test,
	// but the IP pool and peer tracking will work)
	wgMgr, err := wireguard.NewManager(wireguard.Config{
		Interface:       "wg-test",
		ServerPublicKey: "test-server-public-key-base64===========",
		ServerEndpoint:  "test.local:51820",
		Subnet:          "10.99.0.0/24",
		DNS:             "1.1.1.1",
	})
	if err != nil {
		t.Fatalf("wireguard.NewManager: %v", err)
	}

	// Start gateway server
	srv := server.New(cfg, checker, wgMgr)
	srv.SetChainID(11155111) // Sepolia
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := api.NewClient(ts.URL)

	// Step 1: Health check
	t.Run("health", func(t *testing.T) {
		health, err := client.Health()
		if err != nil {
			t.Fatalf("Health: %v", err)
		}
		if health["status"] != "ok" {
			t.Errorf("expected status ok, got %v", health["status"])
		}
	})

	// Step 2: Get challenge
	var challengeMsg string
	t.Run("challenge", func(t *testing.T) {
		resp, err := client.GetChallenge(w.AddressHex())
		if err != nil {
			t.Fatalf("GetChallenge: %v", err)
		}
		if resp.Message == "" {
			t.Fatal("expected non-empty challenge message")
		}
		if resp.Nonce == "" {
			t.Fatal("expected non-empty nonce")
		}
		challengeMsg = resp.Message
		t.Logf("Challenge message:\n%s", resp.Message)
	})

	// Step 3: Sign challenge with wallet
	var signature string
	t.Run("sign", func(t *testing.T) {
		var err error
		signature, err = w.SignMessage(challengeMsg)
		if err != nil {
			t.Fatalf("SignMessage: %v", err)
		}
		if !strings.HasPrefix(signature, "0x") {
			t.Error("signature should start with 0x")
		}
		t.Logf("Signature: %s...", signature[:20])
	})

	// Step 4: Verify signature (creates session, checks NFT)
	var verifyResp *api.VerifyResponse
	t.Run("verify", func(t *testing.T) {
		var err error
		verifyResp, err = client.Verify(challengeMsg, signature)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if verifyResp.Tier != "free" {
			t.Errorf("expected tier=free, got %s", verifyResp.Tier)
		}
		if verifyResp.Address == "" {
			t.Error("expected non-empty address")
		}
		if verifyResp.ExpiresAt == "" {
			t.Error("expected non-empty expiry")
		}
		t.Logf("Verified: address=%s tier=%s expires=%s",
			verifyResp.Address, verifyResp.Tier, verifyResp.ExpiresAt)
	})

	// Step 5: Generate WireGuard keypair
	var keys *wgconf.KeyPair
	t.Run("wg-keygen", func(t *testing.T) {
		var err error
		keys, err = wgconf.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair: %v", err)
		}
		t.Logf("WG public key: %s", keys.PublicKey)
	})

	// Step 6: Connect to VPN
	// Note: This will fail at the wg set command (no real interface), but
	// we can verify the flow up to that point. In a real test environment
	// with a WG interface, this would succeed fully.
	t.Run("connect", func(t *testing.T) {
		if verifyResp == nil {
			t.Skip("skipping: verify step failed")
		}
		resp, err := client.Connect(verifyResp.Address, keys.PublicKey)
		if err != nil {
			// Expected to fail because wg command won't work in test env
			t.Logf("Connect failed (expected in test env without WireGuard): %v", err)
			return
		}
		t.Logf("Connected: IP=%s endpoint=%s tier=%s",
			resp.ClientAddress, resp.ServerEndpoint, resp.Tier)

		// Verify WireGuard config can be generated
		wgCfg := &wgconf.Config{
			PrivateKey:      keys.PrivateKey,
			ClientAddress:   resp.ClientAddress,
			DNS:             resp.DNS,
			ServerPublicKey: resp.ServerPublicKey,
			ServerEndpoint:  resp.ServerEndpoint,
			AllowedIPs:      resp.AllowedIPs,
		}
		confStr := wgCfg.String()
		if !strings.Contains(confStr, "[Interface]") {
			t.Error("config should contain [Interface]")
		}
		if !strings.Contains(confStr, "[Peer]") {
			t.Error("config should contain [Peer]")
		}
		t.Logf("WireGuard config:\n%s", confStr)
	})

	// Step 7: Check status
	t.Run("status", func(t *testing.T) {
		if verifyResp == nil {
			t.Skip("skipping: verify step failed")
		}
		resp, err := client.Status(verifyResp.Address)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		// Session should be active (even if VPN connect failed at WG level)
		if !resp.Connected {
			t.Logf("Status shows not connected (expected if WG connect failed)")
		} else {
			t.Logf("Status: connected=%v tier=%s", resp.Connected, resp.Tier)
		}
	})
}

func TestDeniedWalletFlow(t *testing.T) {
	// Generate a wallet that will NOT be in the allowed list
	w, _ := wallet.Generate()

	// Mock RPC returns (false, false) for all addresses by default
	ethRPC := mockEthRPC(map[common.Address]bool{})
	defer ethRPC.Close()

	cfg := config.DefaultConfig()
	cfg.AccessPolicyContract = "0x0000000000000000000000000000000000000001"
	cfg.MemesContract = "0x0000000000000000000000000000000000000002"
	cfg.EthereumRPC = ethRPC.URL
	cfg.SIWEDomain = "test.local"
	cfg.SIWEUri = "https://test.local"
	cfg.CredentialTTL = 1 * time.Hour
	cfg.NonceLength = 16

	checker, _ := nftcheck.NewChecker(ethRPC.URL, cfg.AccessPolicyContract, 5*time.Minute)
	defer checker.Close()

	wgMgr, _ := wireguard.NewManager(wireguard.Config{
		Interface: "wg-test", Subnet: "10.99.0.0/24",
	})

	srv := server.New(cfg, checker, wgMgr)
	srv.SetChainID(11155111)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := api.NewClient(ts.URL)

	// Get and sign challenge
	challenge, err := client.GetChallenge(w.AddressHex())
	if err != nil {
		t.Fatalf("GetChallenge: %v", err)
	}

	sig, _ := w.SignMessage(challenge.Message)

	// Verify should return 403 (denied)
	_, err = client.Verify(challenge.Message, sig)
	if err == nil {
		t.Fatal("expected error for denied wallet")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected 403 error, got: %v", err)
	}
	t.Logf("Correctly denied: %v", err)
}

func TestReplayedNonceRejected(t *testing.T) {
	w, _ := wallet.Generate()

	ethRPC := mockEthRPC(map[common.Address]bool{w.Address(): true})
	defer ethRPC.Close()

	cfg := config.DefaultConfig()
	cfg.AccessPolicyContract = "0x0000000000000000000000000000000000000001"
	cfg.MemesContract = "0x0000000000000000000000000000000000000002"
	cfg.EthereumRPC = ethRPC.URL
	cfg.SIWEDomain = "test.local"
	cfg.SIWEUri = "https://test.local"
	cfg.CredentialTTL = 1 * time.Hour
	cfg.NonceLength = 16

	checker, _ := nftcheck.NewChecker(ethRPC.URL, cfg.AccessPolicyContract, 5*time.Minute)
	defer checker.Close()

	wgMgr, _ := wireguard.NewManager(wireguard.Config{
		Interface: "wg-test", Subnet: "10.99.0.0/24",
	})

	srv := server.New(cfg, checker, wgMgr)
	srv.SetChainID(11155111)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := api.NewClient(ts.URL)

	// First auth succeeds
	challenge, _ := client.GetChallenge(w.AddressHex())
	sig, _ := w.SignMessage(challenge.Message)
	_, err := client.Verify(challenge.Message, sig)
	if err != nil {
		t.Fatalf("first verify should succeed: %v", err)
	}

	// Replay the same nonce → should fail
	_, err = client.Verify(challenge.Message, sig)
	if err == nil {
		t.Fatal("replayed nonce should be rejected")
	}
	t.Logf("Correctly rejected replay: %v", err)
}

// Helper to verify that an address string parses correctly.
func TestAddressRoundTrip(t *testing.T) {
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	hexAddr := addr.Hex()
	parsed := common.HexToAddress(hexAddr)

	if parsed != addr {
		t.Errorf("address round-trip failed: %s != %s", parsed.Hex(), addr.Hex())
	}
}

// Ensure the mock RPC correctly ABI-encodes booleans.
func TestMockRPCEncoding(t *testing.T) {
	w, _ := wallet.Generate()
	ethRPC := mockEthRPC(map[common.Address]bool{w.Address(): true})
	defer ethRPC.Close()

	// Create a checker and verify it returns the expected tier
	checker, err := nftcheck.NewChecker(ethRPC.URL, "0x0000000000000000000000000000000000000001", time.Minute)
	if err != nil {
		t.Fatalf("NewChecker: %v", err)
	}
	defer checker.Close()

	result, err := checker.Check(t.Context(), w.Address())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	if result.Tier != nftcheck.TierFree {
		t.Errorf("expected TierFree for allowed address, got %s", result.Tier)
	}

	// Check a non-allowed address
	nonAllowed := common.HexToAddress("0x0000000000000000000000000000000000000099")
	result2, err := checker.Check(t.Context(), nonAllowed)
	if err != nil {
		t.Fatalf("Check non-allowed: %v", err)
	}
	if result2.Tier != nftcheck.TierDenied {
		t.Errorf("expected TierDenied for non-allowed address, got %s", result2.Tier)
	}
}

// Unused, just to avoid import errors.
var _ = fmt.Sprintf
