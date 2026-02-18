package server

import (
	"log"

	"github.com/ethereum/go-ethereum/common"
)

// Revoker implements revocation.SessionRevoker by invalidating NFT cache,
// revoking sessions, and removing WireGuard peers.
type Revoker struct {
	srv *Server
}

// NewRevoker creates a session revoker linked to the gateway server.
func NewRevoker(srv *Server) *Revoker {
	return &Revoker{srv: srv}
}

// InvalidateAndRevoke invalidates the NFT check cache and revokes the session.
func (r *Revoker) InvalidateAndRevoke(wallet common.Address) {
	// Invalidate NFT check cache so next check hits on-chain
	r.srv.checker.Invalidate(wallet)

	// Revoke session via the gate
	r.srv.gate.RevokeSession(wallet)

	log.Printf("[revoker] Invalidated cache and revoked session for %s", wallet.Hex())
}
