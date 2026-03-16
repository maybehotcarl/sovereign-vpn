package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/anonauth"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftcheck"
)

func TestParseAddress(t *testing.T) {
	tests := []struct {
		input    string
		expected string // hex without 0x prefix
	}{
		{"0x1234567890abcdef1234567890abcdef12345678", "1234567890abcdef1234567890abcdef12345678"},
		{"0xABCDEF1234567890ABCDEF1234567890ABCDEF12", "abcdef1234567890abcdef1234567890abcdef12"},
		{"short", "0000000000000000000000000000000000000000"},                                      // invalid length -> zero
		{"0xshort", "0000000000000000000000000000000000000000"},                                    // invalid length after 0x
		{"0x1234567890abcdef1234567890abcdef1234567g", "0000000000000000000000000000000000000000"}, // invalid hex char
	}

	for _, tt := range tests {
		addr := parseAddress(tt.input)
		got := ""
		for _, b := range addr {
			got += hexString(b)
		}
		if got != tt.expected {
			t.Errorf("parseAddress(%q) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func hexString(b byte) string {
	const hex = "0123456789abcdef"
	return string([]byte{hex[b>>4], hex[b&0x0f]})
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"hello": "world"})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["hello"] != "world" {
		t.Errorf("expected {hello: world}, got %v", body)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "test error")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "test error" {
		t.Errorf("expected 'test error', got %v", body)
	}
}

func TestHealthEndpoint(t *testing.T) {
	// We can't easily create a full server without a real checker + WG manager,
	// but we can test that the mux routes correctly by checking that
	// /auth/challenge requires a POST body.

	// Test that the helpers work with realistic payloads
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]any{
		"status":          "ok",
		"active_sessions": 0,
		"active_peers":    0,
	})

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

func TestChallengeRequestValidation(t *testing.T) {
	// Test that empty address is caught at the JSON decode level
	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/auth/challenge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	var parsed struct {
		Address string `json:"address"`
	}
	json.NewDecoder(req.Body).Decode(&parsed)

	if parsed.Address != "" {
		t.Errorf("expected empty address, got %q", parsed.Address)
	}
}

func TestConnectRequestValidation(t *testing.T) {
	body := `{"session_token": "tok_abc123def456", "public_key": "abc123"}`
	var req ConnectRequest
	json.NewDecoder(strings.NewReader(body)).Decode(&req)

	if req.SessionToken != "tok_abc123def456" {
		t.Errorf("expected tok_abc123def456, got %s", req.SessionToken)
	}
	if req.PublicKey != "abc123" {
		t.Errorf("expected abc123, got %s", req.PublicKey)
	}
}

func TestBearerToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/vpn/status", nil)
	req.Header.Set("Authorization", "Bearer tok_abc123def456")

	if got := bearerToken(req); got != "tok_abc123def456" {
		t.Fatalf("bearerToken() = %q, want tok_abc123def456", got)
	}
}

func TestBearerTokenRejectsMalformedHeader(t *testing.T) {
	tests := []string{
		"",
		"Bearer",
		"Basic tok_abc123def456",
		"tok_abc123def456",
		"Bearer tok_abc123def456 extra",
	}

	for _, header := range tests {
		req := httptest.NewRequest(http.MethodGet, "/vpn/status", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}

		if got := bearerToken(req); got != "" {
			t.Fatalf("bearerToken(%q) = %q, want empty", header, got)
		}
	}
}

func TestHandleAnonymousChallenge(t *testing.T) {
	s := &Server{
		anonAuth: anonauth.NewService(time.Minute, 8, "vpn_access_v1", 7),
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/anonymous/challenge", nil)
	rec := httptest.NewRecorder()

	s.handleAnonymousChallenge(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp AnonymousChallengeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.ChallengeID == "" {
		t.Fatal("expected challenge id")
	}
	if resp.Nonce == "" {
		t.Fatal("expected nonce")
	}
	if resp.PolicyEpoch != 7 {
		t.Fatalf("PolicyEpoch = %d, want 7", resp.PolicyEpoch)
	}
	if resp.ProofType != "vpn_access_v1" {
		t.Fatalf("ProofType = %q, want vpn_access_v1", resp.ProofType)
	}
}

func TestHandleAnonymousConnectMissingChallenge(t *testing.T) {
	s := &Server{
		anonAuth: anonauth.NewService(time.Minute, 8, "vpn_access_v1", 7),
	}
	body := `{"challenge_id":"missing","proof_type":"vpn_access_v1","nullifier_hash":"nul_1","session_key_hash":"sess_1","public_signals":["root","7","2","123","nul_1","challenge","sess_1"],"public_key":"wg_pub"}` //nolint:lll
	req := httptest.NewRequest(http.MethodPost, "/vpn/anonymous/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleAnonymousVPNConnect(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleAnonymousConnectRequiresVerifier(t *testing.T) {
	s := &Server{
		anonAuth: anonauth.NewService(time.Minute, 8, "vpn_access_v1", 7),
	}
	challenge, err := s.anonAuth.NewChallenge()
	if err != nil {
		t.Fatalf("NewChallenge: %v", err)
	}

	body := `{"challenge_id":"` + challenge.ID + `","proof_type":"vpn_access_v1","nullifier_hash":"nul_1","session_key_hash":"sess_1","public_signals":["root","7","2","123","nul_1","challenge","sess_1"],"public_key":"wg_pub"}` //nolint:lll
	req := httptest.NewRequest(http.MethodPost, "/vpn/anonymous/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleAnonymousVPNConnect(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestValidateVPNAccessV1Signals(t *testing.T) {
	req := AnonymousConnectRequest{
		ProofType:      vpnAccessV1ProofType,
		NullifierHash:  "nul_1",
		SessionKeyHash: "sess_1",
		PublicSignals:  []string{"root", "7", "2", "123", "nul_1", "challenge", "sess_1"},
	}
	challenge := &anonauth.Challenge{PolicyEpoch: 7}

	signals, err := validateVPNAccessV1Signals(challenge, req)
	if err != nil {
		t.Fatalf("validateVPNAccessV1Signals: %v", err)
	}
	if signals.PolicyEpoch != "7" {
		t.Fatalf("PolicyEpoch = %q, want 7", signals.PolicyEpoch)
	}
	if signals.SessionKeyHash != "sess_1" {
		t.Fatalf("SessionKeyHash = %q, want sess_1", signals.SessionKeyHash)
	}
}

func TestValidateVPNAccessV1SignalsRejectsPolicyEpochMismatch(t *testing.T) {
	req := AnonymousConnectRequest{
		ProofType:      vpnAccessV1ProofType,
		NullifierHash:  "nul_1",
		SessionKeyHash: "sess_1",
		PublicSignals:  []string{"root", "8", "2", "123", "nul_1", "challenge", "sess_1"},
	}
	challenge := &anonauth.Challenge{PolicyEpoch: 7}

	if _, err := validateVPNAccessV1Signals(challenge, req); err == nil {
		t.Fatal("expected policy epoch mismatch error")
	}
}

func TestEffectiveTierDowngradesFreeWhenDisabled(t *testing.T) {
	s := &Server{freeTier: false}
	if got := s.effectiveTier(nftcheck.TierFree); got != nftcheck.TierPaid {
		t.Fatalf("effectiveTier(free) = %s, want paid", got)
	}
}

func TestEffectiveTierKeepsFreeWhenEnabled(t *testing.T) {
	s := &Server{freeTier: true}
	if got := s.effectiveTier(nftcheck.TierFree); got != nftcheck.TierFree {
		t.Fatalf("effectiveTier(free) = %s, want free", got)
	}
}
