package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/config"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/nftgate"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/siwe"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/wireguard"
)

// Server is the Sovereign VPN gateway.
type Server struct {
	cfg     *config.Config
	siwe    *siwe.Service
	checker *nftcheck.Checker
	gate    *nftgate.Gate
	wg      *wireguard.Manager
	mux     *http.ServeMux
}

// New creates a new gateway server.
func New(cfg *config.Config, checker *nftcheck.Checker, wg *wireguard.Manager) *Server {
	gate := nftgate.NewGate(checker, cfg.CredentialTTL)

	s := &Server{
		cfg:     cfg,
		siwe:    siwe.NewService(cfg.SIWEDomain, cfg.SIWEUri, cfg.ChallengeTTL, cfg.NonceLength),
		checker: checker,
		gate:    gate,
		wg:      wg,
		mux:     http.NewServeMux(),
	}

	// Public endpoints (no session required)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /auth/challenge", s.handleChallenge)
	s.mux.HandleFunc("POST /auth/verify", s.handleVerify)

	// VPN endpoints (session required via NFT gate)
	s.mux.HandleFunc("POST /vpn/connect", s.handleVPNConnect)
	s.mux.HandleFunc("POST /vpn/disconnect", s.handleVPNDisconnect)
	s.mux.HandleFunc("GET /vpn/status", s.handleVPNStatus)

	return s
}

// SetChainID sets the expected chain ID for SIWE verification.
func (s *Server) SetChainID(chainID int) {
	s.siwe.SetChainID(chainID)
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("Gateway listening on %s", s.cfg.ListenAddr)
	return srv.ListenAndServe()
}

// =========================================================================
//                          AUTH HANDLERS
// =========================================================================

// GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"time":            time.Now().UTC(),
		"active_sessions": s.gate.ActiveSessionCount(),
		"active_peers":    s.wg.PeerCount(),
	})
}

// ChallengeResponse is returned by POST /auth/challenge.
type ChallengeResponse struct {
	Message string `json:"message"`
	Nonce   string `json:"nonce"`
}

// POST /auth/challenge
// Request: { "address": "0x..." }
// Response: { "message": "...", "nonce": "..." }
func (s *Server) handleChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Address == "" {
		writeError(w, http.StatusBadRequest, "address is required")
		return
	}

	challenge, err := s.siwe.NewChallenge(s.cfg.NonceLength)
	if err != nil {
		log.Printf("Error generating challenge: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to generate challenge")
		return
	}

	message := siwe.FormatMessage(challenge, req.Address)

	writeJSON(w, http.StatusOK, ChallengeResponse{
		Message: message,
		Nonce:   challenge.Nonce,
	})
}

// VerifyResponse is returned by POST /auth/verify.
type VerifyResponse struct {
	Address   string `json:"address"`
	Tier      string `json:"tier"`
	ExpiresAt string `json:"expires_at"`
}

// POST /auth/verify -- verify SIWE signature + check NFT -> create session
// Request: { "message": "...", "signature": "0x..." }
// Response: { "address": "0x...", "tier": "free|paid|denied", "expires_at": "..." }
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	var signed siwe.SignedMessage
	if err := json.NewDecoder(r.Body).Decode(&signed); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if signed.Message == "" || signed.Signature == "" {
		writeError(w, http.StatusBadRequest, "message and signature are required")
		return
	}

	// Step 1: Verify SIWE signature, recover wallet address
	auth, err := s.siwe.Verify(&signed)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Step 2: Check NFT access tier
	result, err := s.checker.Check(r.Context(), auth.Address)
	if err != nil {
		log.Printf("Error checking NFT access for %s: %v", auth.Address.Hex(), err)
		writeError(w, http.StatusInternalServerError, "failed to check NFT access")
		return
	}

	// Step 3: Deny if no Memes card
	if result.Tier == nftcheck.TierDenied {
		writeJSON(w, http.StatusForbidden, VerifyResponse{
			Address: auth.Address.Hex(),
			Tier:    result.Tier.String(),
		})
		return
	}

	// Step 4: Create a session
	session := s.gate.CreateSession(auth.Address, result.Tier)

	log.Printf("Access granted: %s tier=%s", auth.Address.Hex(), result.Tier)

	writeJSON(w, http.StatusOK, VerifyResponse{
		Address:   auth.Address.Hex(),
		Tier:      result.Tier.String(),
		ExpiresAt: session.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// =========================================================================
//                          VPN HANDLERS
// =========================================================================

// ConnectRequest is the body for POST /vpn/connect.
type ConnectRequest struct {
	SessionToken string `json:"session_token"` // Wallet address from /auth/verify
	PublicKey    string `json:"public_key"`     // Client's WireGuard public key
}

// ConnectResponse is returned by POST /vpn/connect.
type ConnectResponse struct {
	ServerPublicKey string `json:"server_public_key"`
	ServerEndpoint  string `json:"server_endpoint"`
	ClientAddress   string `json:"client_address"`
	DNS             string `json:"dns"`
	AllowedIPs      string `json:"allowed_ips"`
	ExpiresAt       string `json:"expires_at"`
	Tier            string `json:"tier"`
}

// POST /vpn/connect -- provision a WireGuard peer for an authenticated session
// Request: { "session_token": "0x...", "public_key": "base64-wg-pubkey" }
// Response: WireGuard configuration
func (s *Server) handleVPNConnect(w http.ResponseWriter, r *http.Request) {
	var req ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SessionToken == "" || req.PublicKey == "" {
		writeError(w, http.StatusBadRequest, "session_token and public_key are required")
		return
	}

	// Validate session
	session := s.gate.GetSession(parseAddress(req.SessionToken))
	if session == nil {
		writeError(w, http.StatusUnauthorized, "session expired or not found, re-authenticate via /auth/verify")
		return
	}

	if session.Tier == nftcheck.TierDenied {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	// Provision WireGuard peer
	peerCfg, err := s.wg.AddPeer(req.PublicKey, time.Until(session.ExpiresAt))
	if err != nil {
		log.Printf("Error adding WireGuard peer: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to provision VPN connection")
		return
	}

	log.Printf("VPN connected: %s -> %s", session.Address.Hex(), peerCfg.ClientAddress)

	writeJSON(w, http.StatusOK, ConnectResponse{
		ServerPublicKey: peerCfg.ServerPublicKey,
		ServerEndpoint:  peerCfg.ServerEndpoint,
		ClientAddress:   peerCfg.ClientAddress,
		DNS:             peerCfg.DNS,
		AllowedIPs:      peerCfg.AllowedIPs,
		ExpiresAt:       session.ExpiresAt.UTC().Format(time.RFC3339),
		Tier:            session.Tier.String(),
	})
}

// POST /vpn/disconnect -- remove a WireGuard peer
// Request: { "session_token": "0x...", "public_key": "base64-wg-pubkey" }
func (s *Server) handleVPNDisconnect(w http.ResponseWriter, r *http.Request) {
	var req ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.wg.RemovePeer(req.PublicKey); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// GET /vpn/status -- check connection status
// Query param: ?session_token=0x...
func (s *Server) handleVPNStatus(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("session_token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "session_token query param required")
		return
	}

	session := s.gate.GetSession(parseAddress(token))
	if session == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"connected": false,
			"reason":    "no active session",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"connected":  true,
		"tier":       session.Tier.String(),
		"expires_at": session.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// =========================================================================
//                          HELPERS
// =========================================================================

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parseAddress(s string) (addr [20]byte) {
	// Simple hex address parsing
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}
	if len(s) != 40 {
		return addr
	}
	for i := 0; i < 20; i++ {
		addr[i] = hexByte(s[i*2], s[i*2+1])
	}
	return addr
}

func hexByte(hi, lo byte) byte {
	return hexNibble(hi)<<4 | hexNibble(lo)
}

func hexNibble(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10
	default:
		return 0
	}
}
