// svpn is the Sovereign VPN client CLI.
//
// Usage:
//
//	svpn connect --gateway http://localhost:8080 --key wallet.key
//	svpn status  --gateway http://localhost:8080 --key wallet.key
//	svpn disconnect --gateway http://localhost:8080 --key wallet.key
//	svpn keygen  --out wallet.key
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/maybehotcarl/sovereign-vpn/client/pkg/api"
	"github.com/maybehotcarl/sovereign-vpn/client/pkg/wallet"
	"github.com/maybehotcarl/sovereign-vpn/client/pkg/wgconf"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "connect":
		cmdConnect(os.Args[2:])
	case "disconnect":
		cmdDisconnect(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "keygen":
		cmdKeygen(os.Args[2:])
	case "health":
		cmdHealth(os.Args[2:])
	case "nodes":
		cmdNodes(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Sovereign VPN Client

Usage:
  svpn <command> [flags]

Commands:
  connect      Authenticate and connect to VPN
  disconnect   Disconnect from VPN
  status       Check VPN connection status
  nodes        List available VPN nodes
  keygen       Generate a new Ethereum wallet
  health       Check gateway health

Flags (connect/disconnect/status):
  --gateway    Gateway URL (default: http://localhost:8080)
  --key        Path to wallet key file
  --wg-conf    Path to write WireGuard config (default: sovereign-vpn.conf)`)
}

func cmdConnect(args []string) {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	gateway := fs.String("gateway", "http://localhost:8080", "Gateway URL")
	keyFile := fs.String("key", "", "Path to wallet private key file")
	wgConfPath := fs.String("wg-conf", "sovereign-vpn.conf", "Path to write WireGuard config")
	fs.Parse(args)

	if *keyFile == "" {
		log.Fatal("--key is required (use 'svpn keygen' to create one)")
	}

	// Load wallet
	w, err := wallet.FromKeyFile(*keyFile)
	if err != nil {
		log.Fatalf("Failed to load wallet: %v", err)
	}
	log.Printf("Wallet: %s", w.AddressHex())

	client := api.NewClient(*gateway)

	// Step 1: Get challenge
	log.Println("Requesting authentication challenge...")
	challenge, err := client.GetChallenge(w.AddressHex())
	if err != nil {
		log.Fatalf("Challenge failed: %v", err)
	}

	// Step 2: Sign challenge
	log.Println("Signing challenge with wallet...")
	signature, err := w.SignMessage(challenge.Message)
	if err != nil {
		log.Fatalf("Signing failed: %v", err)
	}

	// Step 3: Verify signature + check NFT
	log.Println("Verifying signature and checking NFT access...")
	verify, err := client.Verify(challenge.Message, signature)
	if err != nil {
		log.Fatalf("Verification failed: %v", err)
	}

	log.Printf("Access tier: %s (expires %s)", verify.Tier, verify.ExpiresAt)

	if verify.Tier == "denied" {
		log.Fatal("Access denied: no qualifying Memes card found in this wallet")
	}

	// Step 4: Generate WireGuard keypair
	log.Println("Generating WireGuard keypair...")
	keys, err := wgconf.GenerateKeyPair()
	if err != nil {
		log.Fatalf("Key generation failed: %v", err)
	}

	// Step 5: Connect to VPN
	log.Println("Requesting VPN connection...")
	conn, err := client.Connect(verify.Address, keys.PublicKey)
	if err != nil {
		log.Fatalf("VPN connect failed: %v", err)
	}

	// Step 6: Write WireGuard config
	cfg := &wgconf.Config{
		PrivateKey:      keys.PrivateKey,
		ClientAddress:   conn.ClientAddress,
		DNS:             conn.DNS,
		ServerPublicKey: conn.ServerPublicKey,
		ServerEndpoint:  conn.ServerEndpoint,
		AllowedIPs:      conn.AllowedIPs,
	}

	if err := cfg.WriteFile(*wgConfPath); err != nil {
		log.Fatalf("Failed to write WireGuard config: %v", err)
	}

	fmt.Println()
	fmt.Println("=== VPN Connected ===")
	fmt.Printf("  Tier:           %s\n", conn.Tier)
	fmt.Printf("  Client IP:      %s\n", conn.ClientAddress)
	fmt.Printf("  Server:         %s\n", conn.ServerEndpoint)
	fmt.Printf("  Expires:        %s\n", conn.ExpiresAt)
	fmt.Printf("  Config written: %s\n", *wgConfPath)
	fmt.Println()
	fmt.Println("To activate the VPN tunnel, run:")
	fmt.Printf("  sudo wg-quick up ./%s\n", *wgConfPath)
	fmt.Println()
	fmt.Println("To disconnect:")
	fmt.Printf("  sudo wg-quick down ./%s\n", *wgConfPath)
}

func cmdDisconnect(args []string) {
	fs := flag.NewFlagSet("disconnect", flag.ExitOnError)
	gateway := fs.String("gateway", "http://localhost:8080", "Gateway URL")
	keyFile := fs.String("key", "", "Path to wallet private key file")
	pubKey := fs.String("wg-pubkey", "", "WireGuard public key to disconnect")
	fs.Parse(args)

	if *keyFile == "" || *pubKey == "" {
		log.Fatal("--key and --wg-pubkey are required")
	}

	w, err := wallet.FromKeyFile(*keyFile)
	if err != nil {
		log.Fatalf("Failed to load wallet: %v", err)
	}

	client := api.NewClient(*gateway)
	if err := client.Disconnect(w.AddressHex(), *pubKey); err != nil {
		log.Fatalf("Disconnect failed: %v", err)
	}

	fmt.Println("Disconnected from VPN.")
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	gateway := fs.String("gateway", "http://localhost:8080", "Gateway URL")
	keyFile := fs.String("key", "", "Path to wallet private key file")
	fs.Parse(args)

	if *keyFile == "" {
		log.Fatal("--key is required")
	}

	w, err := wallet.FromKeyFile(*keyFile)
	if err != nil {
		log.Fatalf("Failed to load wallet: %v", err)
	}

	client := api.NewClient(*gateway)
	status, err := client.Status(w.AddressHex())
	if err != nil {
		log.Fatalf("Status check failed: %v", err)
	}

	if status.Connected {
		fmt.Printf("Connected (tier=%s, expires=%s)\n", status.Tier, status.ExpiresAt)
	} else {
		fmt.Printf("Not connected: %s\n", status.Reason)
	}
}

func cmdKeygen(args []string) {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	outFile := fs.String("out", "", "Output file for private key")
	fs.Parse(args)

	w, err := wallet.Generate()
	if err != nil {
		log.Fatalf("Key generation failed: %v", err)
	}

	fmt.Printf("Address: %s\n", w.AddressHex())

	if *outFile != "" {
		if err := w.SaveKeyFile(*outFile); err != nil {
			log.Fatalf("Failed to save key: %v", err)
		}
		fmt.Printf("Private key saved to: %s\n", *outFile)
	} else {
		fmt.Printf("Private key: %s\n", w.PrivateKeyHex())
		fmt.Println("(Use --out <file> to save to a file)")
	}
}

func cmdHealth(args []string) {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	gateway := fs.String("gateway", "http://localhost:8080", "Gateway URL")
	fs.Parse(args)

	client := api.NewClient(*gateway)
	health, err := client.Health()
	if err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Println("Gateway health:")
	for k, v := range health {
		fmt.Printf("  %s: %v\n", k, v)
	}
}

func cmdNodes(args []string) {
	fs := flag.NewFlagSet("nodes", flag.ExitOnError)
	gateway := fs.String("gateway", "http://localhost:8080", "Gateway URL")
	region := fs.String("region", "", "Filter by region (e.g., us-east)")
	fs.Parse(args)

	client := api.NewClient(*gateway)

	var resp *api.NodesResponse
	var err error
	if *region != "" {
		resp, err = client.ListNodesByRegion(*region)
	} else {
		resp, err = client.ListNodes()
	}
	if err != nil {
		log.Fatalf("Failed to list nodes: %v", err)
	}

	if resp.Count == 0 {
		fmt.Println("No active nodes found.")
		return
	}

	fmt.Printf("Active nodes: %d\n\n", resp.Count)
	for i, n := range resp.Nodes {
		fmt.Printf("  [%d] %s\n", i+1, n.Endpoint)
		fmt.Printf("      Region:   %s\n", n.Region)
		fmt.Printf("      Rep:      %d (6529 VPN Operator)\n", n.Rep)
		fmt.Printf("      Operator: %s\n", n.Operator)
		fmt.Println()
	}
}
