// Package nftgate provides HTTP middleware that checks Memes card ownership
// before allowing VPN handshake requests through.
//
// Designed to plug into:
//   - Sentinel dvpn-node's Gin router (Option A from handshake analysis)
//   - The standalone Sovereign VPN server (for Phase 0 without Sentinel Hub)
//
// The middleware intercepts POST / requests, extracts the client's identity,
// checks NFT ownership via the AccessPolicy contract, and either allows the
// request through or returns 403 Forbidden.
package nftgate

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/nftcheck"
)

// Session represents an authenticated VPN session.
type Session struct {
	Address   common.Address
	Tier      nftcheck.AccessTier
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Gate holds the NFT verification state and session store.
type Gate struct {
	checker    *nftcheck.Checker
	credTTL    time.Duration
	sessions   *SessionStore
}

// NewGate creates a new NFT gate.
func NewGate(checker *nftcheck.Checker, credentialTTL time.Duration) *Gate {
	return &Gate{
		checker:  checker,
		credTTL:  credentialTTL,
		sessions: NewSessionStore(),
	}
}

// CheckAccess verifies NFT ownership for a wallet address.
// Returns the access tier or an error.
func (g *Gate) CheckAccess(ctx context.Context, wallet common.Address) (nftcheck.AccessTier, error) {
	result, err := g.checker.Check(ctx, wallet)
	if err != nil {
		return nftcheck.TierDenied, err
	}
	return result.Tier, nil
}

// CreateSession creates a new authenticated session for a verified wallet.
func (g *Gate) CreateSession(wallet common.Address, tier nftcheck.AccessTier) *Session {
	now := time.Now()
	session := &Session{
		Address:   wallet,
		Tier:      tier,
		CreatedAt: now,
		ExpiresAt: now.Add(g.credTTL),
	}
	g.sessions.Set(wallet, session)
	log.Printf("[nftgate] Session created: %s tier=%s expires=%s",
		wallet.Hex(), tier, session.ExpiresAt.Format(time.RFC3339))
	return session
}

// GetSession retrieves an active session. Returns nil if expired or not found.
func (g *Gate) GetSession(wallet common.Address) *Session {
	session := g.sessions.Get(wallet)
	if session == nil {
		return nil
	}
	if time.Now().After(session.ExpiresAt) {
		g.sessions.Delete(wallet)
		return nil
	}
	return session
}

// RevokeSession removes a session (used when NFT transfer is detected).
func (g *Gate) RevokeSession(wallet common.Address) {
	g.sessions.Delete(wallet)
	log.Printf("[nftgate] Session revoked: %s", wallet.Hex())
}

// InvalidateCache removes cached NFT check results for a wallet.
func (g *Gate) InvalidateCache(wallet common.Address) {
	g.checker.Invalidate(wallet)
}

// ActiveSessionCount returns the number of active sessions.
func (g *Gate) ActiveSessionCount() int {
	return g.sessions.Len()
}

// HTTPMiddleware returns a standard net/http middleware that checks for a valid session.
// Requests without a valid session token get 401. Requests with a session for a denied
// tier get 403. Used by the standalone server.
func (g *Gate) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only gate POST requests (handshake). Let GET /health etc. through.
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		// Extract session token from header
		token := r.Header.Get("X-Session-Token")
		if token == "" {
			http.Error(w, `{"error":"missing X-Session-Token header"}`, http.StatusUnauthorized)
			return
		}

		// Parse the token as an Ethereum address (the session is keyed by wallet address)
		if !common.IsHexAddress(token) {
			http.Error(w, `{"error":"invalid session token"}`, http.StatusUnauthorized)
			return
		}

		wallet := common.HexToAddress(token)
		session := g.GetSession(wallet)
		if session == nil {
			http.Error(w, `{"error":"session expired or not found, re-authenticate via /auth/verify"}`, http.StatusUnauthorized)
			return
		}

		if session.Tier == nftcheck.TierDenied {
			http.Error(w, `{"error":"access denied, no qualifying Memes card found"}`, http.StatusForbidden)
			return
		}

		// Attach session info to request context for downstream handlers
		ctx := context.WithValue(r.Context(), sessionContextKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SessionFromContext retrieves the session from the request context.
func SessionFromContext(ctx context.Context) *Session {
	session, _ := ctx.Value(sessionContextKey).(*Session)
	return session
}

type contextKey string

const sessionContextKey contextKey = "sovereign-vpn-session"
