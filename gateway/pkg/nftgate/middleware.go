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
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftcheck"
)

// Session represents an authenticated VPN session.
type Session struct {
	Address        common.Address
	AddressBound   bool
	NullifierHash  string
	SessionKeyHash string
	PolicyEpoch    uint64
	ID             string
	Token          string
	Tier           nftcheck.AccessTier
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

// Gate holds the NFT verification state and session store.
type Gate struct {
	checker    nftcheck.AccessChecker
	credTTL    time.Duration
	sessions   *SessionStore
	signingKey [32]byte
}

// NewGate creates a new NFT gate.
func NewGate(checker nftcheck.AccessChecker, credentialTTL time.Duration) *Gate {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic("failed to initialize session signing key")
	}

	return &Gate{
		checker:    checker,
		credTTL:    credentialTTL,
		sessions:   NewSessionStore(),
		signingKey: key,
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
	expiresAt := now.Add(g.credTTL)
	id, token, err := g.newSessionToken(expiresAt)
	if err != nil {
		log.Printf("[nftgate] Failed to issue session token: %v", err)
		return nil
	}

	session := &Session{
		Address:      wallet,
		AddressBound: true,
		ID:           id,
		Token:        token,
		Tier:         tier,
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
	}
	g.sessions.Set(session)
	log.Printf("[nftgate] Session created: tier=%s expires=%s",
		tier, session.ExpiresAt.Format(time.RFC3339))
	return session
}

// AnonymousSessionParams describes the metadata attached to an anonymous session.
type AnonymousSessionParams struct {
	Tier           nftcheck.AccessTier
	PolicyEpoch    uint64
	NullifierHash  string
	SessionKeyHash string
}

// CreateAnonymousSession creates a new authenticated session without binding it to a wallet address.
func (g *Gate) CreateAnonymousSession(params AnonymousSessionParams) *Session {
	now := time.Now()
	expiresAt := now.Add(g.credTTL)
	id, token, err := g.newSessionToken(expiresAt)
	if err != nil {
		log.Printf("[nftgate] Failed to issue anonymous session token: %v", err)
		return nil
	}

	session := &Session{
		AddressBound:   false,
		NullifierHash:  params.NullifierHash,
		SessionKeyHash: params.SessionKeyHash,
		PolicyEpoch:    params.PolicyEpoch,
		ID:             id,
		Token:          token,
		Tier:           params.Tier,
		CreatedAt:      now,
		ExpiresAt:      expiresAt,
	}
	g.sessions.Set(session)
	log.Printf("[nftgate] Anonymous session created: tier=%s expires=%s",
		params.Tier, session.ExpiresAt.Format(time.RFC3339))
	return session
}

// GetSession retrieves an active session. Returns nil if expired or not found.
func (g *Gate) GetSession(wallet common.Address) *Session {
	session := g.sessions.GetByAddress(wallet)
	if session == nil {
		return nil
	}
	if time.Now().After(session.ExpiresAt) {
		g.sessions.DeleteByAddress(wallet)
		return nil
	}
	return session
}

// GetSessionByToken retrieves an active session by signed session token.
func (g *Gate) GetSessionByToken(token string) *Session {
	id, expiresAt, err := g.parseAndVerifyToken(token)
	if err != nil {
		return nil
	}

	session := g.sessions.GetByID(id)
	if session == nil {
		return nil
	}
	now := time.Now()
	if now.After(session.ExpiresAt) || now.After(expiresAt) {
		g.sessions.DeleteByID(id)
		return nil
	}
	return session
}

// RevokeSession removes a session (used when NFT transfer is detected).
func (g *Gate) RevokeSession(wallet common.Address) {
	g.sessions.DeleteByAddress(wallet)
	log.Printf("[nftgate] Session revoked")
}

// DeleteSessionByID removes a session by opaque session ID.
func (g *Gate) DeleteSessionByID(id string) {
	g.sessions.DeleteByID(id)
	log.Printf("[nftgate] Session deleted")
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

		session := g.GetSessionByToken(token)
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

func (g *Gate) newSessionToken(expiresAt time.Time) (string, string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	exp := strconv.FormatInt(expiresAt.Unix(), 10)
	payload := "v1." + id + "." + exp
	sig := g.signPayload(payload)
	return id, payload + "." + sig, nil
}

func (g *Gate) parseAndVerifyToken(token string) (string, time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 4 {
		return "", time.Time{}, fmt.Errorf("invalid session token format")
	}
	if parts[0] != "v1" {
		return "", time.Time{}, fmt.Errorf("unsupported session token version")
	}

	payload := strings.Join(parts[:3], ".")
	wantSig := g.signPayload(payload)
	if !hmac.Equal([]byte(wantSig), []byte(parts[3])) {
		return "", time.Time{}, fmt.Errorf("invalid session token signature")
	}

	expUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid session token expiry")
	}

	return parts[1], time.Unix(expUnix, 0).UTC(), nil
}

func (g *Gate) signPayload(payload string) string {
	mac := hmac.New(sha256.New, g.signingKey[:])
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
