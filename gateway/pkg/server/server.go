package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/config"
	"github.com/ethereum/go-ethereum/common"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftgate"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/noderegistry"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/rep6529"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sessionmgr"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/siwe"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/wireguard"
)

// Server is the Sovereign VPN gateway.
type Server struct {
	cfg        *config.Config
	siwe       *siwe.Service
	checker    nftcheck.AccessChecker
	gate       *nftgate.Gate
	wg         *wireguard.Manager
	registry   *noderegistry.Registry
	rep        *rep6529.Checker
	userRep    *rep6529.Checker
	sessionMgr *sessionmgr.Manager
	mux        *http.ServeMux
	corsOrigin string
}

// New creates a new gateway server.
func New(cfg *config.Config, checker nftcheck.AccessChecker, wg *wireguard.Manager) *Server {
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

	// Session info (public — returns contract/pricing for frontend)
	s.mux.HandleFunc("GET /session/info", s.handleSessionInfo)

	// Node discovery endpoint (public)
	s.mux.HandleFunc("GET /nodes", s.handleListNodes)
	s.mux.HandleFunc("GET /nodes/region", s.handleListNodesByRegion)

	return s
}

// SetChainID sets the expected chain ID for SIWE verification.
func (s *Server) SetChainID(chainID int) {
	s.siwe.SetChainID(chainID)
}

// SetRegistry configures the node registry for node discovery endpoints.
func (s *Server) SetRegistry(r *noderegistry.Registry) {
	s.registry = r
}

// SetRepChecker configures the 6529 rep checker for node eligibility.
func (s *Server) SetRepChecker(r *rep6529.Checker) {
	s.rep = r
}

// SetUserRepChecker configures the 6529 rep checker for user ban checking.
func (s *Server) SetUserRepChecker(r *rep6529.Checker) {
	s.userRep = r
}

// SetSessionManager configures the on-chain session manager.
func (s *Server) SetSessionManager(m *sessionmgr.Manager) {
	s.sessionMgr = m
}

// SetCORSOrigin configures the allowed CORS origin for cross-origin requests.
func (s *Server) SetCORSOrigin(origin string) {
	s.corsOrigin = origin
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	if s.corsOrigin == "" {
		return s.mux
	}
	return s.corsMiddleware(s.mux)
}

// corsMiddleware wraps a handler with CORS headers for the configured origin.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.corsOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
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

	// Step 3b: Check user rep ban list (if enabled)
	if s.userRep != nil {
		repResult, err := s.userRep.CheckRep(r.Context(), auth.Address.Hex())
		if err != nil {
			log.Printf("Warning: user rep check failed for %s: %v (allowing access)", auth.Address.Hex(), err)
		} else if repResult.Rating < 0 {
			log.Printf("Access denied (banned): %s rep=%d in %q", auth.Address.Hex(), repResult.Rating, s.userRep.Category())
			writeJSON(w, http.StatusForbidden, map[string]string{
				"address": auth.Address.Hex(),
				"tier":    "denied",
				"error":   "wallet banned: negative reputation in VPN User category",
			})
			return
		}
	}

	// Step 4: Create a session
	session := s.gate.CreateSession(auth.Address, result.Tier)

	// Step 5: Record free session on-chain (fire-and-forget).
	// Paid sessions are opened by the user directly via the contract.
	if s.sessionMgr != nil && result.Tier == nftcheck.TierFree {
		s.sessionMgr.OpenFreeSession(auth.Address, uint64(s.cfg.CredentialTTL.Seconds()))
	}

	log.Printf("Access granted: %s tier=%s", auth.Address.Hex(), result.Tier)

	writeJSON(w, http.StatusOK, VerifyResponse{
		Address:   auth.Address.Hex(),
		Tier:      result.Tier.String(),
		ExpiresAt: session.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// =========================================================================
//                          SESSION HANDLERS
// =========================================================================

// GET /session/info — returns contract address, pricing, and node operator
// for the frontend to construct the openSession transaction.
func (s *Server) handleSessionInfo(w http.ResponseWriter, r *http.Request) {
	if s.sessionMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "session manager not configured")
		return
	}
	info, err := s.sessionMgr.GetSessionInfo(r.Context())
	if err != nil {
		log.Printf("Error getting session info: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to read session info from contract")
		return
	}
	writeJSON(w, http.StatusOK, info)
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

	// For paid tier, verify on-chain payment before provisioning
	if session.Tier == nftcheck.TierPaid && s.sessionMgr != nil {
		sessionID, err := s.sessionMgr.GetActiveSessionID(r.Context(), session.Address)
		if err != nil || sessionID == 0 {
			writeError(w, http.StatusPaymentRequired, "on-chain payment required for paid tier")
			return
		}
		onChain, err := s.sessionMgr.GetSession(r.Context(), sessionID)
		if err != nil || onChain.Payment.Sign() == 0 {
			writeError(w, http.StatusPaymentRequired, "on-chain payment not found")
			return
		}
		// Use on-chain duration for WireGuard peer TTL
		peerCfg, err := s.wg.AddPeer(req.PublicKey, time.Duration(onChain.Duration)*time.Second)
		if err != nil {
			log.Printf("Error adding WireGuard peer: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to provision VPN connection")
			return
		}
		expiresAt := time.Now().Add(time.Duration(onChain.Duration) * time.Second)
		log.Printf("VPN connected (paid): %s -> %s (duration=%ds)", session.Address.Hex(), peerCfg.ClientAddress, onChain.Duration)
		writeJSON(w, http.StatusOK, ConnectResponse{
			ServerPublicKey: peerCfg.ServerPublicKey,
			ServerEndpoint:  peerCfg.ServerEndpoint,
			ClientAddress:   peerCfg.ClientAddress,
			DNS:             peerCfg.DNS,
			AllowedIPs:      peerCfg.AllowedIPs,
			ExpiresAt:       expiresAt.UTC().Format(time.RFC3339),
			Tier:            session.Tier.String(),
		})
		return
	}

	// Provision WireGuard peer (free tier or no session manager)
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

	// Close on-chain session (fire-and-forget)
	if s.sessionMgr != nil {
		s.sessionMgr.CloseSessionFor(common.Address(parseAddress(req.SessionToken)))
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
//                          NODE DISCOVERY HANDLERS
// =========================================================================

// NodeResponse is a public-facing node representation.
type NodeResponse struct {
	Operator    string `json:"operator"`
	Endpoint    string `json:"endpoint"`
	WgPubKey    string `json:"wg_pub_key"`
	Region      string `json:"region"`
	Rep         int64  `json:"rep"`          // 6529 "VPN Operator" rep
	RepEligible bool   `json:"rep_eligible"` // whether rep >= required minimum
	Active      bool   `json:"active"`
}

// GET /nodes — list all active VPN nodes from the on-chain registry.
// Only returns nodes with sufficient 6529 "VPN Operator" rep.
func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	if s.registry == nil {
		writeError(w, http.StatusServiceUnavailable, "node registry not configured")
		return
	}

	nodes, err := s.registry.GetActiveNodes(r.Context())
	if err != nil {
		log.Printf("Error fetching active nodes: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch nodes")
		return
	}

	resp := s.enrichNodesWithRep(r.Context(), nodes)

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes":        resp,
		"count":        len(resp),
		"min_rep":      s.getMinRep(),
		"rep_category": s.getRepCategory(),
	})
}

// GET /nodes/region?region=us-east — list active nodes in a region.
func (s *Server) handleListNodesByRegion(w http.ResponseWriter, r *http.Request) {
	if s.registry == nil {
		writeError(w, http.StatusServiceUnavailable, "node registry not configured")
		return
	}

	region := r.URL.Query().Get("region")
	if region == "" {
		writeError(w, http.StatusBadRequest, "region query param required")
		return
	}

	nodes, err := s.registry.GetActiveNodesByRegion(r.Context(), region)
	if err != nil {
		log.Printf("Error fetching nodes for region %s: %v", region, err)
		writeError(w, http.StatusInternalServerError, "failed to fetch nodes")
		return
	}

	resp := s.enrichNodesWithRep(r.Context(), nodes)

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes":        resp,
		"count":        len(resp),
		"region":       region,
		"min_rep":      s.getMinRep(),
		"rep_category": s.getRepCategory(),
	})
}

// enrichNodesWithRep checks 6529 rep for each node and filters to eligible nodes only.
func (s *Server) enrichNodesWithRep(ctx context.Context, nodes []noderegistry.Node) []NodeResponse {
	var eligible []NodeResponse
	for _, n := range nodes {
		nr := NodeResponse{
			Operator: n.Operator.Hex(),
			Endpoint: n.Endpoint,
			WgPubKey: n.WgPubKey,
			Region:   n.Region,
			Active:   n.Active,
		}

		// Check 6529 rep if checker is configured
		if s.rep != nil {
			result, err := s.rep.CheckRep(ctx, n.Operator.Hex())
			if err != nil {
				log.Printf("Error checking 6529 rep for %s: %v", n.Operator.Hex(), err)
				// Include but mark as not eligible if check fails
				nr.Rep = 0
				nr.RepEligible = false
			} else {
				nr.Rep = result.Rating
				nr.RepEligible = result.Eligible
			}
		} else {
			// No rep checker configured — show all nodes without rep filtering
			nr.RepEligible = true
		}

		// Only include rep-eligible nodes in the response
		if nr.RepEligible {
			eligible = append(eligible, nr)
		}
	}
	return eligible
}

func (s *Server) getMinRep() int64 {
	if s.rep != nil {
		return s.rep.MinRepRequired()
	}
	return 0
}

func (s *Server) getRepCategory() string {
	if s.rep != nil {
		return s.rep.Category()
	}
	return ""
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
