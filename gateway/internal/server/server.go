package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/config"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/siwe"
)

// Server is the SIWE authentication gateway.
type Server struct {
	cfg     *config.Config
	siwe    *siwe.Service
	checker *nftcheck.Checker
	mux     *http.ServeMux
}

// New creates a new gateway server.
func New(cfg *config.Config, checker *nftcheck.Checker) *Server {
	s := &Server{
		cfg:     cfg,
		siwe:    siwe.NewService(cfg.SIWEDomain, cfg.SIWEUri, cfg.ChallengeTTL, cfg.NonceLength),
		checker: checker,
		mux:     http.NewServeMux(),
	}

	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /auth/challenge", s.handleChallenge)
	s.mux.HandleFunc("POST /auth/verify", s.handleVerify)

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

// --- Handlers ---

// GET /health -- simple health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

// ChallengeResponse is returned by POST /auth/challenge.
type ChallengeResponse struct {
	Message string `json:"message"` // The EIP-4361 message the client should sign
	Nonce   string `json:"nonce"`   // The nonce (for reference)
}

// POST /auth/challenge -- generate a SIWE challenge
//
// Request body: { "address": "0x..." }
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

	// Format the EIP-4361 message for the client to sign
	message := siwe.FormatMessage(challenge, req.Address)

	writeJSON(w, http.StatusOK, ChallengeResponse{
		Message: message,
		Nonce:   challenge.Nonce,
	})
}

// VerifyResponse is returned by POST /auth/verify.
type VerifyResponse struct {
	Address    string `json:"address"`     // Verified wallet address
	Tier       string `json:"tier"`        // "free", "paid", or "denied"
	Credential string `json:"credential"`  // Placeholder for WireGuard credential (Sprint 4)
	ExpiresAt  string `json:"expires_at"`  // Credential expiry timestamp
}

// POST /auth/verify -- verify a signed SIWE message and check NFT access
//
// Request body: { "message": "...", "signature": "0x..." }
// Response: { "address": "0x...", "tier": "free|paid|denied", "credential": "...", "expires_at": "..." }
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

	// Step 1: Verify SIWE signature and recover wallet address
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

	// Step 3: Deny if no access
	if result.Tier == nftcheck.TierDenied {
		writeJSON(w, http.StatusForbidden, VerifyResponse{
			Address: auth.Address.Hex(),
			Tier:    result.Tier.String(),
		})
		return
	}

	// Step 4: Issue credential (placeholder -- real WireGuard config in Sprint 4)
	expiresAt := time.Now().Add(s.cfg.CredentialTTL)

	log.Printf("Access granted: %s tier=%s", auth.Address.Hex(), result.Tier)

	writeJSON(w, http.StatusOK, VerifyResponse{
		Address:    auth.Address.Hex(),
		Tier:       result.Tier.String(),
		Credential: "wireguard-config-placeholder", // Sprint 4: actual WG config
		ExpiresAt:  expiresAt.UTC().Format(time.RFC3339),
	})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
