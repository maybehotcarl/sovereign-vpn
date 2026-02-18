package nftgate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/maybehotcarl/sovereign-vpn/gateway/internal/nftcheck"
)

// mockChecker implements just enough of Checker for Gate tests.
// We bypass the real Checker by pre-populating sessions directly.

func testGate() *Gate {
	return &Gate{
		checker:  nil, // not used in session-only tests
		credTTL:  1 * time.Hour,
		sessions: NewSessionStore(),
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
		sessions: NewSessionStore(),
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
	g.CreateSession(addr, nftcheck.TierFree)

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
	req.Header.Set("X-Session-Token", addr.Hex())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHTTPMiddlewareDeniedTier(t *testing.T) {
	g := testGate()
	addr := common.HexToAddress("0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	g.CreateSession(addr, nftcheck.TierDenied)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for denied tier")
	})

	handler := g.HTTPMiddleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/vpn/connect", nil)
	req.Header.Set("X-Session-Token", addr.Hex())
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
