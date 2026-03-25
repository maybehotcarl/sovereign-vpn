package nftgate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftcheck"
)

// mockChecker implements just enough of Checker for Gate tests.
// We bypass the real Checker by pre-populating sessions directly.

func testGate() *Gate {
	return &Gate{
		checker:  nil, // not used in session-only tests
		credTTL:  1 * time.Hour,
		sessions: newInMemorySessionBackend(),
	}
}

func TestCreateAndGetSession(t *testing.T) {
	g := testGate()
	addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	session := g.CreateSession(addr, nftcheck.TierFree)
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.Tier != nftcheck.TierFree {
		t.Errorf("expected tier Free, got %s", session.Tier)
	}
	if session.Address != addr {
		t.Errorf("expected address %s, got %s", addr.Hex(), session.Address.Hex())
	}

	got := g.GetSession(addr)
	if got == nil {
		t.Fatal("expected to retrieve session, got nil")
	}
	if got.Tier != nftcheck.TierFree {
		t.Errorf("expected tier Free, got %s", got.Tier)
	}
}

func TestGetSessionExpired(t *testing.T) {
	g := &Gate{
		credTTL:  1 * time.Millisecond,
		sessions: newInMemorySessionBackend(),
	}
	addr := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	g.CreateSession(addr, nftcheck.TierPaid)
	time.Sleep(5 * time.Millisecond)

	got := g.GetSession(addr)
	if got != nil {
		t.Error("expected nil for expired session")
	}
}

func TestGetSessionNotFound(t *testing.T) {
	g := testGate()
	addr := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	got := g.GetSession(addr)
	if got != nil {
		t.Error("expected nil for non-existent session")
	}
}

func TestRevokeSession(t *testing.T) {
	g := testGate()
	addr := common.HexToAddress("0xcccccccccccccccccccccccccccccccccccccccc")

	g.CreateSession(addr, nftcheck.TierFree)
	g.RevokeSession(addr)

	got := g.GetSession(addr)
	if got != nil {
		t.Error("expected nil after revocation")
	}
}

func TestActiveSessionCount(t *testing.T) {
	g := testGate()

	if g.ActiveSessionCount() != 0 {
		t.Errorf("expected 0 sessions, got %d", g.ActiveSessionCount())
	}

	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	g.CreateSession(addr1, nftcheck.TierFree)
	g.CreateSession(addr2, nftcheck.TierPaid)

	if g.ActiveSessionCount() != 2 {
		t.Errorf("expected 2 sessions, got %d", g.ActiveSessionCount())
	}
}

func TestCreateAnonymousSession(t *testing.T) {
	g := testGate()
	expiresAt := time.Now().Add(5 * time.Minute).UTC().Round(time.Second)

	session := g.CreateAnonymousSession(AnonymousSessionParams{
		Tier:           nftcheck.TierFree,
		PolicyEpoch:    9,
		NullifierHash:  "nul_abc",
		SessionKeyHash: "sess_def",
		ExpiresAt:      expiresAt,
	})
	if session == nil {
		t.Fatal("expected anonymous session, got nil")
	}
	if session.AddressBound {
		t.Fatal("expected anonymous session to be addressless")
	}
	if session.PolicyEpoch != 9 {
		t.Fatalf("PolicyEpoch = %d, want 9", session.PolicyEpoch)
	}
	if session.NullifierHash != "nul_abc" {
		t.Fatalf("NullifierHash = %q, want nul_abc", session.NullifierHash)
	}
	if session.ExpiresAt.After(expiresAt.Add(2 * time.Second)) {
		t.Fatalf("ExpiresAt = %s, expected near %s", session.ExpiresAt, expiresAt)
	}

	got := g.GetSessionByToken(session.Token)
	if got == nil {
		t.Fatal("expected anonymous session by token")
	}
	if got.AddressBound {
		t.Fatal("expected stored anonymous session to remain addressless")
	}
}

func TestBindAndReleaseSessionGateway(t *testing.T) {
	g := testGate()
	addr := common.HexToAddress("0xffffffffffffffffffffffffffffffffffffffff")

	session := g.CreateSession(addr, nftcheck.TierFree)
	if session == nil {
		t.Fatal("expected session")
	}

	stored, newlyBound, err := g.BindSessionGateway(session.ID, GatewayIdentity{
		InstanceID: "gw-a",
		PublicURL:  "https://gw-a.example.com",
		ForwardURL: "http://gw-a.internal:8080",
	})
	if err != nil {
		t.Fatalf("BindSessionGateway: %v", err)
	}
	if !newlyBound {
		t.Fatal("expected first bind to mark session as newly bound")
	}
	if stored == nil {
		t.Fatal("expected stored session after first bind")
	}
	if stored.GatewayInstanceID != "gw-a" {
		t.Fatalf("stored gateway instance = %q, want gw-a", stored.GatewayInstanceID)
	}
	if stored.GatewayPublicURL != "https://gw-a.example.com" {
		t.Fatalf("stored gateway public URL = %q", stored.GatewayPublicURL)
	}
	if stored.GatewayForwardURL != "http://gw-a.internal:8080" {
		t.Fatalf("stored gateway forward URL = %q", stored.GatewayForwardURL)
	}

	stored, newlyBound, err = g.BindSessionGateway(session.ID, GatewayIdentity{
		InstanceID: "gw-a",
		PublicURL:  "https://gw-a.example.com",
		ForwardURL: "http://gw-a.internal:8080",
	})
	if err != nil {
		t.Fatalf("BindSessionGateway repeat: %v", err)
	}
	if newlyBound {
		t.Fatal("expected repeat bind on same gateway to report already bound")
	}

	stored, newlyBound, err = g.BindSessionGateway(session.ID, GatewayIdentity{
		InstanceID: "gw-b",
		PublicURL:  "https://gw-b.example.com",
		ForwardURL: "http://gw-b.internal:8080",
	})
	if err != nil {
		t.Fatalf("BindSessionGateway other gateway: %v", err)
	}
	if newlyBound {
		t.Fatal("expected bind on another gateway to be rejected")
	}
	if stored == nil {
		t.Fatal("expected stored session after conflicting bind")
	}
	if stored.GatewayInstanceID != "gw-a" {
		t.Fatalf("stored gateway instance after conflict = %q, want gw-a", stored.GatewayInstanceID)
	}

	if err := g.ReleaseSessionGateway(session.ID, "gw-b"); err != nil {
		t.Fatalf("ReleaseSessionGateway wrong gateway: %v", err)
	}
	stored = g.GetSessionByToken(session.Token)
	if stored == nil {
		t.Fatal("expected stored session after wrong release")
	}
	if stored.GatewayInstanceID != "gw-a" {
		t.Fatalf("gateway instance after wrong release = %q, want gw-a", stored.GatewayInstanceID)
	}

	if err := g.ReleaseSessionGateway(session.ID, "gw-a"); err != nil {
		t.Fatalf("ReleaseSessionGateway: %v", err)
	}
	stored = g.GetSessionByToken(session.Token)
	if stored == nil {
		t.Fatal("expected stored session after release")
	}
	if stored.GatewayInstanceID != "" || stored.GatewayPublicURL != "" || stored.GatewayForwardURL != "" {
		t.Fatalf(
			"expected gateway binding to be cleared, got id=%q public=%q forward=%q",
			stored.GatewayInstanceID,
			stored.GatewayPublicURL,
			stored.GatewayForwardURL,
		)
	}
}

func TestHTTPMiddlewareAllowsGET(t *testing.T) {
	g := testGate()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := g.HTTPMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET should pass through middleware, got %d", rec.Code)
	}
}

func TestHTTPMiddlewareMissingToken(t *testing.T) {
	g := testGate()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	handler := g.HTTPMiddleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/vpn/connect", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHTTPMiddlewareInvalidToken(t *testing.T) {
	g := testGate()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	handler := g.HTTPMiddleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/vpn/connect", nil)
	req.Header.Set("X-Session-Token", "not-an-address")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHTTPMiddlewareValidSession(t *testing.T) {
	g := testGate()
	addr := common.HexToAddress("0xdddddddddddddddddddddddddddddddddddddd")
	session := g.CreateSession(addr, nftcheck.TierFree)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := SessionFromContext(r.Context())
		if session == nil {
			t.Error("expected session in context")
			return
		}
		if session.Tier != nftcheck.TierFree {
			t.Errorf("expected Free tier, got %s", session.Tier)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := g.HTTPMiddleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/vpn/connect", nil)
	req.Header.Set("X-Session-Token", session.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHTTPMiddlewareDeniedTier(t *testing.T) {
	g := testGate()
	addr := common.HexToAddress("0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	session := g.CreateSession(addr, nftcheck.TierDenied)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for denied tier")
	})

	handler := g.HTTPMiddleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/vpn/connect", nil)
	req.Header.Set("X-Session-Token", session.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestSessionFromContextNil(t *testing.T) {
	session := SessionFromContext(context.Background())
	if session != nil {
		t.Error("expected nil from empty context")
	}
}
