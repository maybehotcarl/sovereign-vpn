package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/config"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/delegation"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/revocation"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/server"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/wireguard"
)

func main() {
	configPath := flag.String("config", "", "Path to config JSON file")
	listenAddr := flag.String("listen", ":8080", "Listen address")
	ethRPC := flag.String("eth-rpc", "", "Ethereum RPC endpoint")
	ethWS := flag.String("eth-ws", "", "Ethereum WebSocket endpoint for event monitoring")
	policyContract := flag.String("policy-contract", "", "AccessPolicy contract address")
	memesContract := flag.String("memes-contract", "", "Memes ERC-1155 contract address")
	chainID := flag.Int("chain-id", 11155111, "Ethereum chain ID (1=mainnet, 11155111=sepolia)")

	// WireGuard flags
	wgInterface := flag.String("wg-interface", "wg0", "WireGuard interface name")
	wgPubKey := flag.String("wg-pubkey", "", "Server WireGuard public key")
	wgEndpoint := flag.String("wg-endpoint", "", "Server public endpoint (e.g. vpn.example.com:51820)")
	wgSubnet := flag.String("wg-subnet", "10.8.0.0/24", "Client IP subnet")
	wgDNS := flag.String("wg-dns", "1.1.1.1", "DNS server for clients")

	// Delegation flags
	enableDelegation := flag.Bool("delegation", false, "Enable delegation registry lookups")
	enableDelegateXYZ := flag.Bool("delegate-xyz", true, "Check delegate.xyz v2 registry")
	enable6529 := flag.Bool("delegation-6529", true, "Check 6529 delegation registry")

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
	if *memesContract != "" {
		cfg.MemesContract = *memesContract
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

	// Configure delegation if enabled
	if *enableDelegation {
		ethClient, err := ethclient.Dial(cfg.EthereumRPC)
		if err != nil {
			log.Fatalf("Failed to connect to Ethereum for delegation: %v", err)
		}
		defer ethClient.Close()

		delChecker, err := delegation.NewChecker(delegation.Config{
			Client:            ethClient,
			EnableDelegateXYZ: *enableDelegateXYZ,
			Enable6529:        *enable6529,
			MemesContract:     common.HexToAddress(cfg.MemesContract),
			CacheTTL:          5 * time.Minute,
		})
		if err != nil {
			log.Fatalf("Failed to create delegation checker: %v", err)
		}
		checker.SetDelegation(delChecker)
		log.Printf("Delegation enabled (delegate.xyz=%v, 6529=%v)", *enableDelegateXYZ, *enable6529)
	}

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

	// Start transfer event watcher if WebSocket endpoint is configured
	if *ethWS != "" && cfg.MemesContract != "" {
		revoker := server.NewRevoker(srv)
		watcher, err := revocation.NewWatcher(*ethWS, common.HexToAddress(cfg.MemesContract), revoker)
		if err != nil {
			log.Printf("Warning: failed to start transfer watcher: %v", err)
		} else {
			go watcher.Start(context.Background())
			defer watcher.Stop()
			log.Printf("Transfer event watcher started on %s", cfg.MemesContract)
		}
	}

	log.Printf("Sovereign VPN Gateway starting")
	log.Printf("  Ethereum RPC:  %s", cfg.EthereumRPC)
	log.Printf("  AccessPolicy:  %s", cfg.AccessPolicyContract)
	log.Printf("  Memes:         %s", cfg.MemesContract)
	log.Printf("  Chain ID:      %d", *chainID)
	log.Printf("  SIWE Domain:   %s", cfg.SIWEDomain)
	log.Printf("  WG Interface:  %s", *wgInterface)
	log.Printf("  WG Endpoint:   %s", *wgEndpoint)
	log.Printf("  WG Subnet:     %s", *wgSubnet)
	log.Printf("  Delegation:    %v", *enableDelegation)

	// Graceful shutdown
	httpSrv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Gateway listening on %s", cfg.ListenAddr)
		errCh <- httpSrv.ListenAndServe()
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down...", sig)
	case err := <-errCh:
		log.Fatalf("Server error: %v", err)
	}

	// Graceful shutdown with 30s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
	log.Println("Gateway stopped")
}
