package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/config"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftgate"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/noderegistry"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/payoutvault"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/ratelimit"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/rep6529"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sessionmgr"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/siwe"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/subscriptionmgr"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/wireguard"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/zkverify"
)

// Server is the Sovereign VPN gateway.
type Server struct {
	cfg         *config.Config
	siwe        *siwe.Service
	checker     nftcheck.AccessChecker
	gate        *nftgate.Gate
	wg          *wireguard.Manager
	registry    *noderegistry.Registry
	userRep     *rep6529.Checker
	sessionMgr  *sessionmgr.Manager
	subMgr      *subscriptionmgr.Manager
	zkClient    *zkverify.Client
	payoutVault *payoutvault.Client
	thisCardID  int64
	peerMu      sync.RWMutex
	peerOwners  map[string]common.Address
	mux         *http.ServeMux
	corsOrigin  string
	limiter     *ratelimit.Limiter
}

// New creates a new gateway server.
func New(cfg *config.Config, checker nftcheck.AccessChecker, wg *wireguard.Manager) *Server {
	gate := nftgate.NewGate(checker, cfg.CredentialTTL)

	var limiter *ratelimit.Limiter
	if cfg.RateLimitPerMinute > 0 {
		limiter = ratelimit.New(cfg.RateLimitPerMinute, time.Minute)
	}

	s := &Server{
		cfg:        cfg,
		siwe:       siwe.NewService(cfg.SIWEDomain, cfg.SIWEUri, cfg.ChallengeTTL, cfg.NonceLength),
		checker:    checker,
		gate:       gate,
		wg:         wg,
		peerOwners: make(map[string]common.Address),
		mux:        http.NewServeMux(),
		limiter:    limiter,
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

	// Subscription info (public — returns tiers + contract address for frontend)
	s.mux.HandleFunc("GET /subscription/tiers", s.handleSubscriptionTiers)

	// Node discovery endpoint (public)
	s.mux.HandleFunc("GET /nodes", s.handleListNodes)
	s.mux.HandleFunc("GET /nodes/region", s.handleListNodesByRegion)

	// Payout status (public — returns pending payout + 0zk address for an operator)
	s.mux.HandleFunc("GET /payout/status", s.handlePayoutStatus)

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

// SetUserRepChecker configures the 6529 rep checker for user ban checking.
func (s *Server) SetUserRepChecker(r *rep6529.Checker) {
	s.userRep = r
}

// SetSessionManager configures the on-chain session manager.
func (s *Server) SetSessionManager(m *sessionmgr.Manager) {
	s.sessionMgr = m
}

// SetSubscriptionManager configures the on-chain subscription manager.
func (s *Server) SetSubscriptionManager(m *subscriptionmgr.Manager) {
	s.subMgr = m
}

// SetCORSOrigin configures the allowed CORS origin for cross-origin requests.
func (s *Server) SetCORSOrigin(origin string) {
	s.corsOrigin = origin
}

// SetZKClient configures the ZK API client for proof verification.
func (s *Server) SetZKClient(c *zkverify.Client) {
	s.zkClient = c
}

// SetPayoutVault configures the PayoutVault client for payout status queries.
func (s *Server) SetPayoutVault(c *payoutvault.Client) {
	s.payoutVault = c
}

// SetThisCardID configures the token ID that grants free tier via ZK proof.
func (s *Server) SetThisCardID(id int64) {
	s.thisCardID = id
}

// Handler returns the HTTP handler with rate limiting and CORS applied.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	if s.corsOrigin != "" {
		h = s.corsMiddleware(h)
	}
	if s.limiter != nil {
		h = s.limiter.Wrap(h)
	}
	return h
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
		Handler:      s.Handler(),
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
	Address      string `json:"address"`
	SessionToken string `json:"session_token"`
	Tier         string `json:"tier"`
	ExpiresAt    string `json:"expires_at"`
}

// zkProofPayload is an optional ZK proof included in the verify request.
type zkProofPayload struct {
	ProofType     string   `json:"proof_type"`
	Proof         any      `json:"proof"`
	PublicSignals []string `json:"public_signals"`
}

// verifyRequest extends the SIWE signed message with an optional ZK proof.
type verifyRequest struct {
	Message   string          `json:"message"`
	Signature string          `json:"signature"`
	ZKProof   *zkProofPayload `json:"zk_proof,omitempty"`
}

// POST /auth/verify -- verify SIWE signature + check NFT (or ZK proof) -> create session
// Request: { "message": "...", "signature": "0x...", "zk_proof": { ... } }
// Response: { "address": "0x...", "session_token": "<opaque>", "tier": "free|paid|denied", "expires_at": "..." }
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Message == "" || req.Signature == "" {
		writeError(w, http.StatusBadRequest, "message and signature are required")
		return
	}

	// Step 1: Verify SIWE signature, recover wallet address
	signed := &siwe.SignedMessage{Message: req.Message, Signature: req.Signature}
	auth, err := s.siwe.Verify(signed)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Step 2: Determine access tier — ZK proof path or on-chain path
	var result nftcheck.CheckResult

	if req.ZKProof != nil && s.zkClient != nil {
		// ZK path: forward proof to ZK API for verification
		zkResult, err := s.zkClient.VerifyProof(r.Context(), zkverify.ProofPayload{
			ProofType:     req.ZKProof.ProofType,
			Proof:         req.ZKProof.Proof,
			PublicSignals: req.ZKProof.PublicSignals,
		})
		if err != nil {
			log.Printf("ZK API error during proof verification: %v", err)
			writeError(w, http.StatusBadGateway, "ZK verification service unavailable")
			return
		}

		if !zkResult.Valid {
			log.Printf("ZK proof invalid: type=%s reason=%s", req.ZKProof.ProofType, zkResult.Reason)
			writeJSON(w, http.StatusForbidden, VerifyResponse{
				Address: auth.Address.Hex(),
				Tier:    nftcheck.TierDenied.String(),
			})
			return
		}

		// Determine tier from public signals
		result = nftcheck.CheckResult{
			Tier:      s.tierFromZKProof(req.ZKProof),
			CheckedAt: time.Now(),
		}
		log.Printf("ZK proof valid: type=%s tier=%s", req.ZKProof.ProofType, result.Tier)
	} else {
		// On-chain path: existing NFT check
		result, err = s.checker.Check(r.Context(), auth.Address)
		if err != nil {
			log.Printf("Error checking NFT access: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to check NFT access")
			return
		}
	}

	// Step 3: Deny if no access
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
			log.Printf("Warning: user rep check failed (allowing access): %v", err)
		} else if repResult.Rating < 0 {
			log.Printf("Access denied (banned): rep=%d category=%q", repResult.Rating, s.userRep.Category())
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
	if session == nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	// Step 5: Record free session on-chain (fire-and-forget).
	// Paid sessions are opened by the user directly via the contract.
	if s.sessionMgr != nil && result.Tier == nftcheck.TierFree {
		s.sessionMgr.OpenFreeSession(auth.Address, uint64(s.cfg.CredentialTTL.Seconds()))
	}

	log.Printf("Access granted: tier=%s", result.Tier)

	writeJSON(w, http.StatusOK, VerifyResponse{
		Address:      auth.Address.Hex(),
		SessionToken: session.Token,
		Tier:         result.Tier.String(),
		ExpiresAt:    session.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// tierFromZKProof determines the access tier from a validated ZK proof.
// For card_ownership: publicSignals[1] is cardId — if it matches thisCardID → free, else → paid.
// For tdh_range: publicSignals[1] is bucketMin — any valid proof → paid (or free if configured).
// Default: any valid proof → paid.
func (s *Server) tierFromZKProof(proof *zkProofPayload) nftcheck.AccessTier {
	if proof.ProofType == "card_ownership" && len(proof.PublicSignals) >= 2 {
		cardID, err := strconv.ParseInt(proof.PublicSignals[1], 10, 64)
		if err == nil && s.thisCardID > 0 && cardID == s.thisCardID {
			return nftcheck.TierFree
		}
		return nftcheck.TierPaid
	}

	// tdh_range or any other proof type: valid proof = paid access
	return nftcheck.TierPaid
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

// GET /subscription/tiers — returns subscription tier list + contract address
// for the frontend to construct subscribe transactions.
func (s *Server) handleSubscriptionTiers(w http.ResponseWriter, r *http.Request) {
	if s.subMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "subscription manager not configured")
		return
	}
	tiers, err := s.subMgr.GetTiers(r.Context())
	if err != nil {
		log.Printf("Error getting subscription tiers: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to read tiers from contract")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"contract": s.subMgr.ContractAddr(),
		"chain_id": s.subMgr.ChainID(),
		"tiers":    tiers,
	})
}

// =========================================================================
//                          VPN HANDLERS
// =========================================================================

// ConnectRequest is the body for POST /vpn/connect.
type ConnectRequest struct {
	SessionToken string `json:"session_token"` // Opaque session token from /auth/verify
	PublicKey    string `json:"public_key"`    // Client's WireGuard public key
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
// Request: { "session_token": "<opaque-token>", "public_key": "base64-wg-pubkey" }
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
	session := s.gate.GetSessionByToken(req.SessionToken)
	if session == nil {
		writeError(w, http.StatusUnauthorized, "session expired or not found, re-authenticate via /auth/verify")
		return
	}
	if !s.claimsPeer(req.PublicKey, session.Address) {
		writeError(w, http.StatusForbidden, "public key is already bound to another session")
		return
	}

	if session.Tier == nftcheck.TierDenied {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	// For paid tier, check subscription first, then fall back to 24h session
	if session.Tier == nftcheck.TierPaid {
		// Path 1: Check active subscription
		if s.subMgr != nil {
			sub, err := s.subMgr.GetSubscription(r.Context(), session.Address)
			if err == nil && sub.ExpiresAt > uint64(time.Now().Unix()) {
				remaining := time.Duration(sub.ExpiresAt-uint64(time.Now().Unix())) * time.Second
				peerCfg, err := s.wg.AddPeer(req.PublicKey, remaining)
				if err != nil {
					log.Printf("Error adding WireGuard peer: %v", err)
					writeError(w, http.StatusInternalServerError, "failed to provision VPN connection")
					return
				}
				expiresAt := time.Now().Add(remaining)
				s.setPeerOwner(req.PublicKey, session.Address)
				log.Printf("VPN connected (subscription): remaining=%s", remaining)
				writeJSON(w, http.StatusOK, ConnectResponse{
					ServerPublicKey: peerCfg.ServerPublicKey,
					ServerEndpoint:  peerCfg.ServerEndpoint,
					ClientAddress:   peerCfg.ClientAddress,
					DNS:             peerCfg.DNS,
					AllowedIPs:      peerCfg.AllowedIPs,
					ExpiresAt:       expiresAt.UTC().Format(time.RFC3339),
					Tier:            "subscription",
				})
				return
			}
		}

		// Path 2: Fall back to 24h session
		if s.sessionMgr != nil {
			sessionID, err := s.sessionMgr.GetActiveSessionID(r.Context(), session.Address)
			if err == nil && sessionID != 0 {
				onChain, err := s.sessionMgr.GetSession(r.Context(), sessionID)
				if err == nil && onChain.Payment.Sign() > 0 {
					peerCfg, err := s.wg.AddPeer(req.PublicKey, time.Duration(onChain.Duration)*time.Second)
					if err != nil {
						log.Printf("Error adding WireGuard peer: %v", err)
						writeError(w, http.StatusInternalServerError, "failed to provision VPN connection")
						return
					}
					expiresAt := time.Now().Add(time.Duration(onChain.Duration) * time.Second)
					s.setPeerOwner(req.PublicKey, session.Address)
					log.Printf("VPN connected (paid): duration=%ds", onChain.Duration)
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
			}
		}

		writeError(w, http.StatusPaymentRequired, "on-chain payment required for paid tier")
		return
	}

	// Provision WireGuard peer (free tier or no session manager)
	peerCfg, err := s.wg.AddPeer(req.PublicKey, time.Until(session.ExpiresAt))
	if err != nil {
		log.Printf("Error adding WireGuard peer: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to provision VPN connection")
		return
	}

	log.Printf("VPN connected: tier=%s", session.Tier)
	s.setPeerOwner(req.PublicKey, session.Address)

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
// Request: { "session_token": "<opaque-token>", "public_key": "base64-wg-pubkey" }
func (s *Server) handleVPNDisconnect(w http.ResponseWriter, r *http.Request) {
	var req ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SessionToken == "" || req.PublicKey == "" {
		writeError(w, http.StatusBadRequest, "session_token and public_key are required")
		return
	}

	session := s.gate.GetSessionByToken(req.SessionToken)
	if session == nil {
		writeError(w, http.StatusUnauthorized, "session expired or not found, re-authenticate via /auth/verify")
		return
	}
	if !s.peerOwnedBy(req.PublicKey, session.Address) {
		writeError(w, http.StatusForbidden, "public key is not owned by this session")
		return
	}

	if err := s.wg.RemovePeer(req.PublicKey); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.deletePeerOwner(req.PublicKey)

	// Close on-chain session (fire-and-forget) — skip for subscribers
	// (subscription stays valid; user can reconnect freely)
	addr := session.Address
	isSubscriber := false
	if s.subMgr != nil {
		active, err := s.subMgr.HasActiveSubscription(r.Context(), addr)
		if err == nil && active {
			isSubscriber = true
		}
	}
	if !isSubscriber && s.sessionMgr != nil {
		s.sessionMgr.CloseSessionFor(addr)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func bearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}

	parts := strings.Fields(auth)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return parts[1]
}

// GET /vpn/status -- check connection status
// Authorization: Bearer <opaque-token>
func (s *Server) handleVPNStatus(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusBadRequest, "Authorization Bearer token required")
		return
	}

	session := s.gate.GetSessionByToken(token)
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
	Operator       string `json:"operator"`
	Endpoint       string `json:"endpoint"`
	WgPubKey       string `json:"wg_pub_key"`
	Region         string `json:"region"`
	CardEligible   bool   `json:"card_eligible"` // whether operator holds the required card
	Active         bool   `json:"active"`
	RailgunAddress string `json:"railgun_address,omitempty"` // RAILGUN 0zk address
}

// GET /nodes — list all active VPN nodes from the on-chain registry.
// Only returns nodes whose operators hold the required card.
// TODO(prod-scale): Move to paginated/indexed node reads before large-node mainnet rollout.
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

	resp := s.enrichNodesWithCardCheck(r.Context(), nodes)

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": resp,
		"count": len(resp),
		"gate":  "card_ownership",
	})
}

// GET /nodes/region?region=us-east — list active nodes in a region.
// TODO(prod-scale): Move to paginated/indexed node reads before large-node mainnet rollout.
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

	resp := s.enrichNodesWithCardCheck(r.Context(), nodes)

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes":  resp,
		"count":  len(resp),
		"region": region,
		"gate":   "card_ownership",
	})
}

// enrichNodesWithCardCheck checks on-chain card ownership for each node and filters out ineligible.
func (s *Server) enrichNodesWithCardCheck(ctx context.Context, nodes []noderegistry.Node) []NodeResponse {
	var eligible []NodeResponse
	for _, n := range nodes {
		nr := NodeResponse{
			Operator: n.Operator.Hex(),
			Endpoint: n.Endpoint,
			WgPubKey: n.WgPubKey,
			Region:   n.Region,
			Active:   n.Active,
		}

		// Check on-chain card ownership via NodeRegistry.isEligibleOperator
		cardOk, err := s.registry.IsEligibleOperator(ctx, n.Operator)
		if err != nil {
			log.Printf("Error checking card eligibility for %s: %v", n.Operator.Hex(), err)
			nr.CardEligible = false
		} else {
			nr.CardEligible = cardOk
		}

		// Fetch RAILGUN 0zk address if registry is available
		if s.registry != nil {
			railgunAddr, err := s.registry.GetRailgunAddress(ctx, n.Operator)
			if err == nil && railgunAddr != "" {
				nr.RailgunAddress = railgunAddr
			}
		}

		// Only include card-eligible nodes in the response
		if nr.CardEligible {
			eligible = append(eligible, nr)
		}
	}
	return eligible
}

// =========================================================================
//                          PAYOUT HANDLERS
// =========================================================================

// GET /payout/status?operator=0x... — returns pending payout + 0zk address for an operator.
func (s *Server) handlePayoutStatus(w http.ResponseWriter, r *http.Request) {
	operatorHex := r.URL.Query().Get("operator")
	if operatorHex == "" {
		writeError(w, http.StatusBadRequest, "operator query param required")
		return
	}

	operator := common.HexToAddress(operatorHex)
	resp := map[string]any{
		"operator": operator.Hex(),
	}

	// Fetch pending payout from vault
	if s.payoutVault != nil {
		pending, err := s.payoutVault.GetPendingPayout(r.Context(), operator)
		if err != nil {
			log.Printf("Error fetching pending payout for %s: %v", operatorHex, err)
		} else {
			resp["pending_payout_wei"] = pending.String()
		}

		processed, err := s.payoutVault.GetProcessedPayout(r.Context(), operator)
		if err != nil {
			log.Printf("Error fetching processed payout for %s: %v", operatorHex, err)
		} else {
			resp["processed_payout_wei"] = processed.String()
		}
	} else {
		resp["pending_payout_wei"] = "0"
		resp["processed_payout_wei"] = "0"
	}

	// Fetch RAILGUN address from registry
	if s.registry != nil {
		railgunAddr, err := s.registry.GetRailgunAddress(r.Context(), operator)
		if err != nil {
			log.Printf("Error fetching railgun address for %s: %v", operatorHex, err)
		} else {
			resp["railgun_address"] = railgunAddr
		}
	}

	writeJSON(w, http.StatusOK, resp)
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

func (s *Server) claimsPeer(pubKey string, owner common.Address) bool {
	s.peerMu.Lock()
	defer s.peerMu.Unlock()
	if existing, ok := s.peerOwners[pubKey]; ok && existing != owner {
		return false
	}
	return true
}

func (s *Server) setPeerOwner(pubKey string, owner common.Address) {
	s.peerMu.Lock()
	s.peerOwners[pubKey] = owner
	s.peerMu.Unlock()
}

func (s *Server) peerOwnedBy(pubKey string, owner common.Address) bool {
	s.peerMu.RLock()
	defer s.peerMu.RUnlock()
	existing, ok := s.peerOwners[pubKey]
	return ok && existing == owner
}

func (s *Server) deletePeerOwner(pubKey string) {
	s.peerMu.Lock()
	delete(s.peerOwners, pubKey)
	s.peerMu.Unlock()
}

func parseAddress(s string) (addr [20]byte) {
	if !common.IsHexAddress(s) {
		return addr
	}
	parsed := common.HexToAddress(s)
	copy(addr[:], parsed.Bytes())
	return addr
}
