package main

import (
	"flag"
	"log"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/config"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/server"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/wireguard"
)

func main() {
	configPath := flag.String("config", "", "Path to config JSON file")
	listenAddr := flag.String("listen", ":8080", "Listen address")
	ethRPC := flag.String("eth-rpc", "", "Ethereum RPC endpoint")
	policyContract := flag.String("policy-contract", "", "AccessPolicy contract address")
	chainID := flag.Int("chain-id", 11155111, "Ethereum chain ID (1=mainnet, 11155111=sepolia)")

	// WireGuard flags
	wgInterface := flag.String("wg-interface", "wg0", "WireGuard interface name")
	wgPubKey := flag.String("wg-pubkey", "", "Server WireGuard public key")
	wgEndpoint := flag.String("wg-endpoint", "", "Server public endpoint (e.g. vpn.example.com:51820)")
	wgSubnet := flag.String("wg-subnet", "10.8.0.0/24", "Client IP subnet")
	wgDNS := flag.String("wg-dns", "1.1.1.1", "DNS server for clients")

	flag.Parse()

	// Load config
	var cfg *config.Config
	if *configPath != "" {
		var err error
		cfg, err = config.LoadFromFile(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
		cfg = config.DefaultConfig()
	}

	// Override with flags
	if *listenAddr != ":8080" || cfg.ListenAddr == "" {
		cfg.ListenAddr = *listenAddr
	}
	if *ethRPC != "" {
		cfg.EthereumRPC = *ethRPC
	}
	if *policyContract != "" {
		cfg.AccessPolicyContract = *policyContract
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// Create NFT checker
	checker, err := nftcheck.NewChecker(cfg.EthereumRPC, cfg.AccessPolicyContract, 5*time.Minute)
	if err != nil {
		log.Fatalf("Failed to create NFT checker: %v", err)
	}
	defer checker.Close()

	// Create WireGuard manager
	wgCfg := wireguard.Config{
		Interface:       *wgInterface,
		ServerPublicKey: *wgPubKey,
		ServerEndpoint:  *wgEndpoint,
		Subnet:          *wgSubnet,
		DNS:             *wgDNS,
	}

	wgManager, err := wireguard.NewManager(wgCfg)
	if err != nil {
		log.Fatalf("Failed to create WireGuard manager: %v", err)
	}

	// Start expired peer cleanup every minute
	wgManager.StartCleanupWorker(1 * time.Minute)

	// Create and start server
	srv := server.New(cfg, checker, wgManager)
	srv.SetChainID(*chainID)

	log.Printf("Sovereign VPN Gateway starting")
	log.Printf("  Ethereum RPC:  %s", cfg.EthereumRPC)
	log.Printf("  AccessPolicy:  %s", cfg.AccessPolicyContract)
	log.Printf("  Chain ID:      %d", *chainID)
	log.Printf("  SIWE Domain:   %s", cfg.SIWEDomain)
	log.Printf("  WG Interface:  %s", *wgInterface)
	log.Printf("  WG Endpoint:   %s", *wgEndpoint)
	log.Printf("  WG Subnet:     %s", *wgSubnet)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
