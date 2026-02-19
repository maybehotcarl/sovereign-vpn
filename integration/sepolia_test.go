// Package integration — Sepolia live integration tests.
//
// These tests run against real Sepolia contracts. They require:
//   - SEPOLIA_RPC environment variable (Ethereum Sepolia RPC URL)
//   - PRIVATE_KEY environment variable (deployer wallet private key, no 0x prefix)
//
// The deployer wallet must hold TestMemes tokens (minted during deployment).
//
// Run with: go test -v -run TestSepolia -count=1 ./...
package integration

import (
	"context"
	"net/http/httptest"
	"os"
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

const (
	// Deployed Sepolia contracts
	sepoliaAccessPolicy = "0xF1AfCFD8eF6a869987D50e173e22F6fc99431712"
	sepoliaTestMemes    = "0x98C361b7C385b9589E60B36B880501D66123B294"
	sepoliaChainID      = 11155111
)

func skipIfNoSepolia(t *testing.T) (rpcURL, privKey string) {
	t.Helper()
	rpcURL = os.Getenv("SEPOLIA_RPC")
	privKey = os.Getenv("PRIVATE_KEY")
	if rpcURL == "" || privKey == "" {
		t.Skip("Skipping Sepolia test: SEPOLIA_RPC and PRIVATE_KEY env vars required")
	}
	return rpcURL, privKey
}

// TestSepoliaCheckAccess verifies the NFT checker can call the real
// AccessPolicy contract on Sepolia and get the correct tier for the deployer.
func TestSepoliaCheckAccess(t *testing.T) {
	rpcURL, privKey := skipIfNoSepolia(t)

	// Derive address from private key
	key, err := crypto.HexToECDSA(privKey)
	if err != nil {
		t.Fatalf("invalid PRIVATE_KEY: %v", err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	t.Logf("Deployer address: %s", addr.Hex())

	checker, err := nftcheck.NewChecker(rpcURL, sepoliaAccessPolicy, 5*time.Minute)
	if err != nil {
		t.Fatalf("NewChecker: %v", err)
	}
	defer checker.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := checker.Check(ctx, addr)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	t.Logf("Tier: %s (checked at %s)", result.Tier, result.CheckedAt)

	if result.Tier != nftcheck.TierFree {
		t.Errorf("expected TierFree for deployer, got %s", result.Tier)
	}
}

// TestSepoliaUnknownWalletDenied checks that a random wallet with no Memes
// tokens is denied access on the real Sepolia contract.
func TestSepoliaUnknownWalletDenied(t *testing.T) {
	rpcURL, _ := skipIfNoSepolia(t)

	checker, err := nftcheck.NewChecker(rpcURL, sepoliaAccessPolicy, 5*time.Minute)
	if err != nil {
		t.Fatalf("NewChecker: %v", err)
	}
	defer checker.Close()

	// Random address — almost certainly has no Memes tokens
	randAddr := common.HexToAddress("0x0000000000000000000000000000000000dead01")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := checker.Check(ctx, randAddr)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	t.Logf("Random wallet tier: %s", result.Tier)

	if result.Tier != nftcheck.TierDenied {
		t.Errorf("expected TierDenied for random wallet, got %s", result.Tier)
	}
}

// TestSepoliaFullAuthFlow exercises the complete pipeline against live Sepolia:
// keygen → challenge → sign → verify (with real on-chain NFT check) → connect
func TestSepoliaFullAuthFlow(t *testing.T) {
	rpcURL, privKey := skipIfNoSepolia(t)

	// Load deployer wallet
	w, err := wallet.FromHex(privKey)
	if err != nil {
		t.Fatalf("wallet.FromHex: %v", err)
	}
	t.Logf("Testing with wallet: %s", w.AddressHex())

	// Create gateway config pointing to real Sepolia
	cfg := config.DefaultConfig()
	cfg.AccessPolicyContract = sepoliaAccessPolicy
	cfg.MemesContract = sepoliaTestMemes
	cfg.EthereumRPC = rpcURL
	cfg.SIWEDomain = "test.local"
	cfg.SIWEUri = "https://test.local"
	cfg.CredentialTTL = 1 * time.Hour
	cfg.ChallengeTTL = 5 * time.Minute
	cfg.NonceLength = 16

	// Create NFT checker pointed at REAL Sepolia contracts
	checker, err := nftcheck.NewChecker(rpcURL, sepoliaAccessPolicy, 5*time.Minute)
	if err != nil {
		t.Fatalf("nftcheck.NewChecker: %v", err)
	}
	defer checker.Close()

	// WireGuard manager (test mode — no real wg interface)
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
	srv.SetChainID(sepoliaChainID)
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
		t.Logf("Gateway healthy: %v", health)
	})

	// Step 2: Get SIWE challenge
	var challengeMsg string
	t.Run("challenge", func(t *testing.T) {
		resp, err := client.GetChallenge(w.AddressHex())
		if err != nil {
			t.Fatalf("GetChallenge: %v", err)
		}
		if resp.Message == "" || resp.Nonce == "" {
			t.Fatal("expected non-empty challenge")
		}
		challengeMsg = resp.Message
		t.Logf("Got challenge with nonce: %s", resp.Nonce)
	})

	// Step 3: Sign with deployer wallet (ERC-191)
	var signature string
	t.Run("sign", func(t *testing.T) {
		if challengeMsg == "" {
			t.Skip("no challenge to sign")
		}
		var err error
		signature, err = w.SignMessage(challengeMsg)
		if err != nil {
			t.Fatalf("SignMessage: %v", err)
		}
		if !strings.HasPrefix(signature, "0x") {
			t.Error("signature should start with 0x")
		}
		t.Logf("Signed: %s...", signature[:20])
	})

	// Step 4: Verify signature + on-chain NFT check (REAL Sepolia call)
	var verifyResp *api.VerifyResponse
	t.Run("verify", func(t *testing.T) {
		if challengeMsg == "" || signature == "" {
			t.Skip("no challenge/signature")
		}
		var err error
		verifyResp, err = client.Verify(challengeMsg, signature)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if verifyResp.Tier != "free" {
			t.Errorf("expected tier=free (deployer has THIS card), got %s", verifyResp.Tier)
		}
		if verifyResp.Address == "" {
			t.Error("expected non-empty address")
		}
		t.Logf("Verified on Sepolia: address=%s tier=%s expires=%s",
			verifyResp.Address, verifyResp.Tier, verifyResp.ExpiresAt)
	})

	// Step 5: Generate WireGuard keys and connect
	t.Run("connect", func(t *testing.T) {
		if verifyResp == nil {
			t.Skip("verify failed, skipping connect")
		}

		keys, err := wgconf.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair: %v", err)
		}

		resp, err := client.Connect(verifyResp.Address, keys.PublicKey)
		if err != nil {
			// May fail at wg command level (no real interface) — that's OK
			t.Logf("Connect failed (expected without real WG): %v", err)
			return
		}

		t.Logf("Connected: IP=%s endpoint=%s tier=%s",
			resp.ClientAddress, resp.ServerEndpoint, resp.Tier)

		// Verify we can generate a WireGuard config from the response
		wgCfg := &wgconf.Config{
			PrivateKey:      keys.PrivateKey,
			ClientAddress:   resp.ClientAddress,
			DNS:             resp.DNS,
			ServerPublicKey: resp.ServerPublicKey,
			ServerEndpoint:  resp.ServerEndpoint,
			AllowedIPs:      resp.AllowedIPs,
		}
		confStr := wgCfg.String()
		if !strings.Contains(confStr, "[Interface]") || !strings.Contains(confStr, "[Peer]") {
			t.Error("generated config missing required sections")
		}
		t.Logf("WireGuard config:\n%s", confStr)
	})

	// Step 6: Check session status
	t.Run("status", func(t *testing.T) {
		if verifyResp == nil {
			t.Skip("verify failed, skipping status")
		}
		resp, err := client.Status(verifyResp.Address)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if !resp.Connected {
			t.Logf("Status: not connected (expected if WG connect failed)")
		} else {
			t.Logf("Status: connected=%v tier=%s expires=%s",
				resp.Connected, resp.Tier, resp.ExpiresAt)
		}
	})
}

// TestSepoliaDeniedWallet verifies that a wallet without Memes tokens
// is properly denied by the gateway when checking against real Sepolia contracts.
func TestSepoliaDeniedWallet(t *testing.T) {
	rpcURL, _ := skipIfNoSepolia(t)

	// Generate a fresh wallet (no tokens on Sepolia)
	freshWallet, err := wallet.Generate()
	if err != nil {
		t.Fatalf("wallet.Generate: %v", err)
	}
	t.Logf("Fresh wallet (no tokens): %s", freshWallet.AddressHex())

	cfg := config.DefaultConfig()
	cfg.AccessPolicyContract = sepoliaAccessPolicy
	cfg.MemesContract = sepoliaTestMemes
	cfg.EthereumRPC = rpcURL
	cfg.SIWEDomain = "test.local"
	cfg.SIWEUri = "https://test.local"
	cfg.CredentialTTL = 1 * time.Hour
	cfg.NonceLength = 16

	checker, err := nftcheck.NewChecker(rpcURL, sepoliaAccessPolicy, 5*time.Minute)
	if err != nil {
		t.Fatalf("nftcheck.NewChecker: %v", err)
	}
	defer checker.Close()

	wgMgr, _ := wireguard.NewManager(wireguard.Config{
		Interface: "wg-test", Subnet: "10.99.0.0/24",
	})

	srv := server.New(cfg, checker, wgMgr)
	srv.SetChainID(sepoliaChainID)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := api.NewClient(ts.URL)

	// Auth flow
	challenge, err := client.GetChallenge(freshWallet.AddressHex())
	if err != nil {
		t.Fatalf("GetChallenge: %v", err)
	}

	sig, err := freshWallet.SignMessage(challenge.Message)
	if err != nil {
		t.Fatalf("SignMessage: %v", err)
	}

	// Verify should return 403 — no Memes tokens
	_, err = client.Verify(challenge.Message, sig)
	if err == nil {
		t.Fatal("expected error for wallet without Memes tokens")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected 403 error, got: %v", err)
	}
	t.Logf("Correctly denied wallet without tokens: %v", err)
}
