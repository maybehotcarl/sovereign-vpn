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
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/noderegistry"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/rep6529"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/revocation"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/server"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sessionmgr"
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
	siweDomain := flag.String("siwe-domain", "", "SIWE domain (default: 6529vpn.io)")

	// Direct mode (mainnet) — check Memes ERC-1155 directly without AccessPolicy
	directMode := flag.Bool("direct-mode", false, "Check Memes ERC-1155 directly (no AccessPolicy contract needed)")
	thisCardID := flag.Int64("this-card-id", 0, "Token ID for THIS card (free tier). 0 = no free tier")
	maxTokenID := flag.Int64("max-token-id", 350, "Highest Memes token ID to check")

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

	// Node registry flags
	nodeRegistryContract := flag.String("node-registry", "", "NodeRegistry contract address")
	nodeRegistryCacheTTL := flag.Duration("node-cache-ttl", 2*time.Minute, "Node registry cache TTL")

	// 6529 Rep flags
	repMinimum := flag.Int64("rep-min", rep6529.DefaultMinRep, "Minimum 6529 rep to operate a node")
	repCategory := flag.String("rep-category", rep6529.DefaultCategory, "6529 rep category name")
	repAPIURL := flag.String("rep-api-url", rep6529.DefaultBaseURL, "6529 rep API base URL")
	repCacheTTL := flag.Duration("rep-cache-ttl", 5*time.Minute, "6529 rep cache TTL")

	// User ban check flags
	userBanCheck := flag.Bool("user-ban-check", false, "Enable user rep ban checking via 6529 rep")
	userBanCategory := flag.String("user-ban-category", "VPN User", "6529 rep category for user ban checking")

	// CORS flag
	corsOrigin := flag.String("cors-origin", "", "Allowed CORS origin (e.g. https://6529vpn.io)")

	// Heartbeat flags (for node operators running a gateway)
	heartbeatKey := flag.String("heartbeat-key", "", "Private key hex for sending heartbeat txs (node operator mode)")
	heartbeatInterval := flag.Duration("heartbeat-interval", 30*time.Minute, "Heartbeat send interval")

	// SessionManager flags
	sessionManagerContract := flag.String("session-manager", "", "SessionManager contract address (enables on-chain session tracking)")
	sessionKey := flag.String("session-key", "", "Private key hex for SessionManager txs (contract owner)")

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
	if *siweDomain != "" {
		cfg.SIWEDomain = *siweDomain
		cfg.SIWEUri = "https://" + *siweDomain
	}

	// In direct mode, AccessPolicy is not required
	if *directMode {
		if cfg.MemesContract == "" {
			log.Fatal("--memes-contract is required")
		}
		if cfg.EthereumRPC == "" {
			log.Fatal("--eth-rpc is required")
		}
	} else {
		if err := cfg.Validate(); err != nil {
			log.Fatalf("Invalid config: %v", err)
		}
	}

	// Create NFT checker (direct mode or AccessPolicy mode)
	var checker nftcheck.AccessChecker
	if *directMode {
		if cfg.MemesContract == "" {
			log.Fatal("--memes-contract is required in direct mode")
		}
		dc, err := nftcheck.NewDirectChecker(cfg.EthereumRPC, cfg.MemesContract, *thisCardID, *maxTokenID, 5*time.Minute)
		if err != nil {
			log.Fatalf("Failed to create direct NFT checker: %v", err)
		}
		defer dc.Close()
		checker = dc
		log.Printf("Direct mode: checking Memes ERC-1155 at %s (this-card=%d, max-id=%d)", cfg.MemesContract, *thisCardID, *maxTokenID)

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
			dc.SetDelegation(delChecker)
			log.Printf("Delegation enabled (delegate.xyz=%v, 6529=%v)", *enableDelegateXYZ, *enable6529)
		}
	} else {
		ac, err := nftcheck.NewChecker(cfg.EthereumRPC, cfg.AccessPolicyContract, 5*time.Minute)
		if err != nil {
			log.Fatalf("Failed to create NFT checker: %v", err)
		}
		defer ac.Close()
		checker = ac

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
			ac.SetDelegation(delChecker)
			log.Printf("Delegation enabled (delegate.xyz=%v, 6529=%v)", *enableDelegateXYZ, *enable6529)
		}
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

	if *corsOrigin != "" {
		srv.SetCORSOrigin(*corsOrigin)
		log.Printf("CORS enabled for origin: %s", *corsOrigin)
	}

	// Configure user ban check if enabled
	if *userBanCheck {
		userRepChecker := rep6529.NewChecker(rep6529.Config{
			Category: *userBanCategory,
			MinRep:   1, // placeholder; we check Rating < 0 directly
			CacheTTL: *repCacheTTL,
		})
		srv.SetUserRepChecker(userRepChecker)
		log.Printf("User ban check enabled: category=%q", *userBanCategory)
	}

	// Configure node registry if contract address is provided
	if *nodeRegistryContract != "" {
		registry, err := noderegistry.NewRegistry(cfg.EthereumRPC, *nodeRegistryContract, *nodeRegistryCacheTTL)
		if err != nil {
			log.Fatalf("Failed to create node registry: %v", err)
		}
		defer registry.Close()
		srv.SetRegistry(registry)
		log.Printf("Node registry enabled: %s", *nodeRegistryContract)

		// Configure 6529 rep checker for node eligibility
		repChecker := rep6529.NewChecker(rep6529.Config{
			BaseURL:  *repAPIURL,
			Category: *repCategory,
			MinRep:   *repMinimum,
			CacheTTL: *repCacheTTL,
		})
		srv.SetRepChecker(repChecker)
		log.Printf("6529 rep filter enabled: category=%q min=%d", *repCategory, *repMinimum)

		// Start heartbeat sender if private key is provided (node operator mode)
		if *heartbeatKey != "" {
			hb, err := noderegistry.NewHeartbeatSender(
				cfg.EthereumRPC, *nodeRegistryContract, *heartbeatKey,
				int64(*chainID), *heartbeatInterval,
			)
			if err != nil {
				log.Fatalf("Failed to create heartbeat sender: %v", err)
			}
			go hb.Start(context.Background())
			defer hb.Stop()
			log.Printf("Heartbeat sender started (interval=%s)", *heartbeatInterval)
		}
	}

	// Configure SessionManager if contract address is provided
	if *sessionManagerContract != "" {
		keyHex := *sessionKey
		if keyHex == "" {
			keyHex = *heartbeatKey // fall back to heartbeat key
		}
		if keyHex == "" {
			log.Printf("Warning: --session-manager set without --session-key; on-chain sessions disabled")
		} else {
			sm, err := sessionmgr.New(cfg.EthereumRPC, *sessionManagerContract, keyHex, int64(*chainID))
			if err != nil {
				log.Fatalf("Failed to create session manager: %v", err)
			}
			defer sm.Close()
			srv.SetSessionManager(sm)
			log.Printf("SessionManager enabled: %s", *sessionManagerContract)
		}
	}

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
	if *nodeRegistryContract != "" {
		log.Printf("  NodeRegistry:  %s", *nodeRegistryContract)
		log.Printf("  6529 Rep Min:  %d (%s)", *repMinimum, *repCategory)
	}
	if *sessionManagerContract != "" {
		log.Printf("  SessionMgr:    %s", *sessionManagerContract)
	}

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
