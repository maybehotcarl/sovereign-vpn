package main

import (
	"flag"
	"log"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/config"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/server"
)

func main() {
	configPath := flag.String("config", "", "Path to config JSON file (optional, uses defaults + flags if not set)")
	listenAddr := flag.String("listen", ":8080", "Listen address")
	ethRPC := flag.String("eth-rpc", "", "Ethereum RPC endpoint")
	memesContract := flag.String("memes-contract", "", "Memes ERC-1155 contract address")
	policyContract := flag.String("policy-contract", "", "AccessPolicy contract address")
	chainID := flag.Int("chain-id", 11155111, "Ethereum chain ID (1=mainnet, 11155111=sepolia)")
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

	// Override with flags if provided
	if *listenAddr != ":8080" || cfg.ListenAddr == "" {
		cfg.ListenAddr = *listenAddr
	}
	if *ethRPC != "" {
		cfg.EthereumRPC = *ethRPC
	}
	if *memesContract != "" {
		cfg.MemesContract = *memesContract
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

	// Create and start server
	srv := server.New(cfg, checker)
	srv.SetChainID(*chainID)

	log.Printf("Sovereign VPN Gateway starting")
	log.Printf("  Ethereum RPC: %s", cfg.EthereumRPC)
	log.Printf("  AccessPolicy: %s", cfg.AccessPolicyContract)
	log.Printf("  Chain ID:     %d", *chainID)
	log.Printf("  SIWE Domain:  %s", cfg.SIWEDomain)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
