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
	Address           common.Address
	AddressBound      bool
	NullifierHash     string
	SessionKeyHash    string
	PolicyEpoch       uint64
	GatewayInstanceID string
	GatewayPublicURL  string
	GatewayForwardURL string
	ID                string
	Token             string
	Tier              nftcheck.AccessTier
	CreatedAt         time.Time
	ExpiresAt         time.Time
}

// GatewayIdentity identifies the gateway instance that owns a live WireGuard session.
type GatewayIdentity struct {
	InstanceID string
	PublicURL  string
	ForwardURL string
}

// Gate holds the NFT verification state and session store.
type Gate struct {
	checker    nftcheck.AccessChecker
	credTTL    time.Duration
	sessions   sessionStoreBackend
	signingKey [32]byte
}

// NewGate creates a new NFT gate.
func NewGate(checker nftcheck.AccessChecker, credentialTTL time.Duration) *Gate {
	g, err := NewGateWithOptions(checker, credentialTTL, GateOptions{})
	if err != nil {
		panic(err)
	}
	return g
}

// NewGateWithOptions creates a new NFT gate with pluggable session storage and signing secret.
func NewGateWithOptions(
	checker nftcheck.AccessChecker,
	credentialTTL time.Duration,
	opts GateOptions,
) (*Gate, error) {
	key, err := deriveSigningKey(opts.SessionSigningSecret)
	if err != nil {
		return nil, fmt.Errorf("initializing session signing key: %w", err)
	}

	sessionStore := opts.SessionStore
	if sessionStore == nil {
		sessionStore = newInMemorySessionBackend()
	}

	return &Gate{
		checker:    checker,
		credTTL:    credentialTTL,
		sessions:   sessionStore,
		signingKey: key,
	}, nil
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
	session, err := g.CreateSessionWithError(wallet, tier)
	if err != nil {
		log.Printf("[nftgate] Failed to create session: %v", err)
		return nil
	}
	return session
}

// CreateSessionWithError creates a new authenticated session for a verified wallet.
func (g *Gate) CreateSessionWithError(wallet common.Address, tier nftcheck.AccessTier) (*Session, error) {
	now := time.Now()
	expiresAt := now.Add(g.credTTL)
	id, token, err := g.newSessionToken(expiresAt)
	if err != nil {
		return nil, err
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
	if err := g.sessions.Set(session); err != nil {
		return nil, err
	}
	log.Printf("[nftgate] Session created: tier=%s expires=%s",
		tier, session.ExpiresAt.Format(time.RFC3339))
	return session, nil
}

// AnonymousSessionParams describes the metadata attached to an anonymous session.
type AnonymousSessionParams struct {
	Tier           nftcheck.AccessTier
	PolicyEpoch    uint64
	NullifierHash  string
	SessionKeyHash string
	ExpiresAt      time.Time
}

// CreateAnonymousSession creates a new authenticated session without binding it to a wallet address.
func (g *Gate) CreateAnonymousSession(params AnonymousSessionParams) *Session {
	session, err := g.CreateAnonymousSessionWithError(params)
	if err != nil {
		log.Printf("[nftgate] Failed to create anonymous session: %v", err)
		return nil
	}
	return session
}

// CreateAnonymousSessionWithError creates a new authenticated session without binding it to a wallet address.
func (g *Gate) CreateAnonymousSessionWithError(params AnonymousSessionParams) (*Session, error) {
	now := time.Now()
	expiresAt := now.Add(g.credTTL)
	if !params.ExpiresAt.IsZero() && params.ExpiresAt.Before(expiresAt) {
		expiresAt = params.ExpiresAt
	}
	id, token, err := g.newSessionToken(expiresAt)
	if err != nil {
		return nil, err
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
	if err := g.sessions.Set(session); err != nil {
		return nil, err
	}
	log.Printf("[nftgate] Anonymous session created: tier=%s expires=%s",
		params.Tier, session.ExpiresAt.Format(time.RFC3339))
	return session, nil
}

// GetSession retrieves an active session. Returns nil if expired or not found.
func (g *Gate) GetSession(wallet common.Address) *Session {
	session, _ := g.GetSessionWithError(wallet)
	return session
}

// GetSessionWithError retrieves an active session. Returns nil if expired or not found.
func (g *Gate) GetSessionWithError(wallet common.Address) (*Session, error) {
	session, err := g.sessions.GetByAddress(wallet)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}
	if time.Now().After(session.ExpiresAt) {
		if err := g.sessions.DeleteByAddress(wallet); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return session, nil
}

// GetSessionByToken retrieves an active session by signed session token.
func (g *Gate) GetSessionByToken(token string) *Session {
	session, _ := g.GetSessionByTokenWithError(token)
	return session
}

// GetSessionByTokenWithError retrieves an active session by signed session token.
func (g *Gate) GetSessionByTokenWithError(token string) (*Session, error) {
	id, expiresAt, err := g.parseAndVerifyToken(token)
	if err != nil {
		return nil, nil
	}

	session, err := g.sessions.GetByID(id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}
	now := time.Now()
	if now.After(session.ExpiresAt) || now.After(expiresAt) {
		if err := g.sessions.DeleteByID(id); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return session, nil
}

// RevokeSession removes a session (used when NFT transfer is detected).
func (g *Gate) RevokeSession(wallet common.Address) {
	if err := g.RevokeSessionWithError(wallet); err != nil {
		log.Printf("[nftgate] Failed to revoke session: %v", err)
		return
	}
	log.Printf("[nftgate] Session revoked")
}

// RevokeSessionWithError removes a session (used when NFT transfer is detected).
func (g *Gate) RevokeSessionWithError(wallet common.Address) error {
	return g.sessions.DeleteByAddress(wallet)
}

// DeleteSessionByID removes a session by opaque session ID.
func (g *Gate) DeleteSessionByID(id string) {
	if err := g.DeleteSessionByIDWithError(id); err != nil {
		log.Printf("[nftgate] Failed to delete session: %v", err)
		return
	}
	log.Printf("[nftgate] Session deleted")
}

// DeleteSessionByIDWithError removes a session by opaque session ID.
func (g *Gate) DeleteSessionByIDWithError(id string) error {
	return g.sessions.DeleteByID(id)
}

// BindSessionGateway binds a session to a gateway instance on first successful connect.
// It returns the stored session and whether the binding was created during this call.
func (g *Gate) BindSessionGateway(id string, gateway GatewayIdentity) (*Session, bool, error) {
	return g.sessions.BindGateway(id, gateway)
}

// ReleaseSessionGateway clears the gateway owner for a session when the peer disconnects.
func (g *Gate) ReleaseSessionGateway(id string, gatewayInstanceID string) error {
	return g.sessions.ReleaseGateway(id, gatewayInstanceID)
}

// InvalidateCache removes cached NFT check results for a wallet.
func (g *Gate) InvalidateCache(wallet common.Address) {
	g.checker.Invalidate(wallet)
}

// ActiveSessionCount returns the number of active sessions.
func (g *Gate) ActiveSessionCount() int {
	count, _ := g.ActiveSessionCountWithError()
	return count
}

// ActiveSessionCountWithError returns the number of active sessions.
func (g *Gate) ActiveSessionCountWithError() (int, error) {
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

		session, err := g.GetSessionByTokenWithError(token)
		if err != nil {
			http.Error(w, `{"error":"session backend unavailable"}`, http.StatusServiceUnavailable)
			return
		}
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
