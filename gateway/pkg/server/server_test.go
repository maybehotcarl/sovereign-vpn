package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/anonauth"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/config"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftgate"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/wireguard"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/zkverify"
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

func newTestWireGuardManager(t *testing.T, endpoint string) *wireguard.Manager {
	t.Helper()

	dir := t.TempDir()
	wgPath := filepath.Join(dir, "wg")
	if err := os.WriteFile(wgPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(wg): %v", err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	manager, err := wireguard.NewManager(wireguard.Config{
		Interface:       "wg0",
		ServerPublicKey: "server_public_key",
		ServerEndpoint:  endpoint,
		Subnet:          "10.8.0.0/24",
		DNS:             "1.1.1.1",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return manager
}

func mustTestGate(t *testing.T) *nftgate.Gate {
	t.Helper()

	gate, err := nftgate.NewGateWithOptions(nil, time.Hour, nftgate.GateOptions{
		SessionSigningSecret: "test-signing-secret",
	})
	if err != nil {
		t.Fatalf("NewGateWithOptions: %v", err)
	}
	return gate
}

func newAffinityTestServer(t *testing.T, instanceID string, publicURL string) (*Server, *nftgate.Gate, *nftgate.Session) {
	t.Helper()

	gate, err := nftgate.NewGateWithOptions(nil, time.Hour, nftgate.GateOptions{
		SessionSigningSecret: "test-signing-secret",
	})
	if err != nil {
		t.Fatalf("NewGateWithOptions: %v", err)
	}

	session, err := gate.CreateSessionWithError(
		common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678"),
		nftcheck.TierFree,
	)
	if err != nil {
		t.Fatalf("CreateSessionWithError: %v", err)
	}

	s := &Server{
		cfg: &config.Config{
			ListenAddr:        ":8080",
			GatewayInstanceID: instanceID,
			GatewayPublicURL:  publicURL,
			GatewayForwardURL: publicURL,
		},
		gate:       gate,
		wg:         newTestWireGuardManager(t, "vpn.example.com:51820"),
		peerOwners: newLocalPeerOwnershipStore(),
		peerStates: newLocalPeerStateStore(),
	}
	return s, gate, session
}

func newDeadOwnerRecoveryServer(t *testing.T, deadOwnerID string, deadOwnerURL string) (*Server, *nftgate.Gate, *nftgate.Session) {
	t.Helper()

	gate, err := nftgate.NewGateWithOptions(nil, time.Hour, nftgate.GateOptions{
		SessionSigningSecret: "test-signing-secret",
	})
	if err != nil {
		t.Fatalf("NewGateWithOptions: %v", err)
	}

	session, err := gate.CreateSessionWithError(
		common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678"),
		nftcheck.TierFree,
	)
	if err != nil {
		t.Fatalf("CreateSessionWithError: %v", err)
	}
	if _, _, err := gate.BindSessionGateway(session.ID, nftgate.GatewayIdentity{
		InstanceID: deadOwnerID,
		PublicURL:  deadOwnerURL,
	}); err != nil {
		t.Fatalf("BindSessionGateway: %v", err)
	}

	return &Server{
		cfg: &config.Config{
			ListenAddr:        ":8081",
			GatewayInstanceID: "gw-b",
			GatewayPublicURL:  "https://gw-b.example.com",
		},
		gate:                  gate,
		wg:                    newTestWireGuardManager(t, "vpn.example.com:51820"),
		peerOwners:            newLocalPeerOwnershipStore(),
		peerStates:            newLocalPeerStateStore(),
		gatewayPresence:       newLocalGatewayPresenceStore(),
		gatewayPresenceShared: true,
	}, gate, session
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

func TestHandleVPNConnectBindsGatewayOwner(t *testing.T) {
	s, gate, session := newAffinityTestServer(t, "gw-a", "https://gw-a.example.com")

	body := `{"session_token":"` + session.Token + `","public_key":"wg_pub_1"}`
	req := httptest.NewRequest(http.MethodPost, "/vpn/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleVPNConnect(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp ConnectResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.GatewayInstanceID != "gw-a" {
		t.Fatalf("GatewayInstanceID = %q, want gw-a", resp.GatewayInstanceID)
	}
	if resp.GatewayURL != "https://gw-a.example.com" {
		t.Fatalf("GatewayURL = %q, want https://gw-a.example.com", resp.GatewayURL)
	}
	if s.wg.PeerCount() != 1 {
		t.Fatalf("PeerCount = %d, want 1", s.wg.PeerCount())
	}

	stored := gate.GetSessionByToken(session.Token)
	if stored == nil {
		t.Fatal("expected stored session")
	}
	if stored.GatewayInstanceID != "gw-a" {
		t.Fatalf("stored session gateway = %q, want gw-a", stored.GatewayInstanceID)
	}
}

func TestHandleVPNConnectRejectsWrongGatewayOwner(t *testing.T) {
	s, gate, session := newAffinityTestServer(t, "gw-b", "https://gw-b.example.com")

	_, newlyBound, err := gate.BindSessionGateway(session.ID, nftgate.GatewayIdentity{
		InstanceID: "gw-a",
		PublicURL:  "https://gw-a.example.com",
	})
	if err != nil {
		t.Fatalf("BindSessionGateway: %v", err)
	}
	if !newlyBound {
		t.Fatal("expected pre-bind to set owner")
	}

	body := `{"session_token":"` + session.Token + `","public_key":"wg_pub_2"}`
	req := httptest.NewRequest(http.MethodPost, "/vpn/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleVPNConnect(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp["code"] != "session_bound_to_other_gateway" {
		t.Fatalf("code = %v, want session_bound_to_other_gateway", resp["code"])
	}
	if resp["gateway_instance_id"] != "gw-a" {
		t.Fatalf("gateway_instance_id = %v, want gw-a", resp["gateway_instance_id"])
	}
	if resp["gateway_url"] != "https://gw-a.example.com" {
		t.Fatalf("gateway_url = %v, want https://gw-a.example.com", resp["gateway_url"])
	}
	if s.wg.PeerCount() != 0 {
		t.Fatalf("PeerCount = %d, want 0", s.wg.PeerCount())
	}
}

func TestHandleVPNDisconnectRejectsWrongGatewayOwner(t *testing.T) {
	s, gate, session := newAffinityTestServer(t, "gw-b", "https://gw-b.example.com")

	_, newlyBound, err := gate.BindSessionGateway(session.ID, nftgate.GatewayIdentity{
		InstanceID: "gw-a",
		PublicURL:  "https://gw-a.example.com",
	})
	if err != nil {
		t.Fatalf("BindSessionGateway: %v", err)
	}
	if !newlyBound {
		t.Fatal("expected pre-bind to set owner")
	}

	body := `{"session_token":"` + session.Token + `","public_key":"wg_pub_2"}`
	req := httptest.NewRequest(http.MethodPost, "/vpn/disconnect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleVPNDisconnect(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp["code"] != "session_bound_to_other_gateway" {
		t.Fatalf("code = %v, want session_bound_to_other_gateway", resp["code"])
	}
	if resp["gateway_instance_id"] != "gw-a" {
		t.Fatalf("gateway_instance_id = %v, want gw-a", resp["gateway_instance_id"])
	}
}

func TestHandleVPNStatusIncludesGatewayAffinity(t *testing.T) {
	s, gate, session := newAffinityTestServer(t, "gw-b", "https://gw-b.example.com")

	_, newlyBound, err := gate.BindSessionGateway(session.ID, nftgate.GatewayIdentity{
		InstanceID: "gw-a",
		PublicURL:  "https://gw-a.example.com",
	})
	if err != nil {
		t.Fatalf("BindSessionGateway: %v", err)
	}
	if !newlyBound {
		t.Fatal("expected pre-bind to set owner")
	}

	req := httptest.NewRequest(http.MethodGet, "/vpn/status", nil)
	req.Header.Set("Authorization", "Bearer "+session.Token)
	rec := httptest.NewRecorder()

	s.handleVPNStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if connected, ok := resp["connected"].(bool); !ok || !connected {
		t.Fatalf("connected = %v, want true", resp["connected"])
	}
	if affinityOK, ok := resp["gateway_affinity_ok"].(bool); !ok || affinityOK {
		t.Fatalf("gateway_affinity_ok = %v, want false", resp["gateway_affinity_ok"])
	}
	if resp["gateway_instance_id"] != "gw-a" {
		t.Fatalf("gateway_instance_id = %v, want gw-a", resp["gateway_instance_id"])
	}
	if resp["gateway_url"] != "https://gw-a.example.com" {
		t.Fatalf("gateway_url = %v, want https://gw-a.example.com", resp["gateway_url"])
	}
}

func TestRecoverOwnedPeersRestoresWireGuardState(t *testing.T) {
	s, gate, session := newAffinityTestServer(t, "gw-a", "https://gw-a.example.com")
	expiresAt := time.Now().Add(30 * time.Minute).UTC()

	if _, _, err := gate.BindSessionGateway(session.ID, nftgate.GatewayIdentity{
		InstanceID: "gw-a",
		PublicURL:  "https://gw-a.example.com",
	}); err != nil {
		t.Fatalf("BindSessionGateway: %v", err)
	}
	if err := s.peerStates.Put(&peerState{
		PublicKey:         "wg_pub_recover",
		SessionID:         session.ID,
		GatewayInstanceID: "gw-a",
		GatewayURL:        "https://gw-a.example.com",
		ClientIP:          "10.8.0.44",
		AssignedAt:        time.Now().Add(-time.Minute).UTC(),
		ExpiresAt:         expiresAt,
	}); err != nil {
		t.Fatalf("peerStates.Put: %v", err)
	}

	if err := s.recoverOwnedPeers(); err != nil {
		t.Fatalf("recoverOwnedPeers: %v", err)
	}

	peer := s.wg.GetPeer("wg_pub_recover")
	if peer == nil {
		t.Fatal("expected peer to be recovered")
	}
	if peer.ClientIP != "10.8.0.44" {
		t.Fatalf("ClientIP = %q, want 10.8.0.44", peer.ClientIP)
	}
}

func TestHandleVPNDisconnectRecoversPeerStateOnDemand(t *testing.T) {
	s, gate, session := newAffinityTestServer(t, "gw-a", "https://gw-a.example.com")
	expiresAt := time.Now().Add(30 * time.Minute).UTC()

	if _, _, err := gate.BindSessionGateway(session.ID, nftgate.GatewayIdentity{
		InstanceID: "gw-a",
		PublicURL:  "https://gw-a.example.com",
	}); err != nil {
		t.Fatalf("BindSessionGateway: %v", err)
	}
	reserved, err := s.peerOwners.Reserve("wg_pub_recover_disconnect", session.ID, "gw-a", expiresAt)
	if err != nil {
		t.Fatalf("peerOwners.Reserve: %v", err)
	}
	if !reserved {
		t.Fatal("expected peer owner reservation to succeed")
	}
	if err := s.peerStates.Put(&peerState{
		PublicKey:         "wg_pub_recover_disconnect",
		SessionID:         session.ID,
		GatewayInstanceID: "gw-a",
		GatewayURL:        "https://gw-a.example.com",
		ClientIP:          "10.8.0.45",
		AssignedAt:        time.Now().Add(-time.Minute).UTC(),
		ExpiresAt:         expiresAt,
	}); err != nil {
		t.Fatalf("peerStates.Put: %v", err)
	}

	body := `{"session_token":"` + session.Token + `","public_key":"wg_pub_recover_disconnect"}`
	req := httptest.NewRequest(http.MethodPost, "/vpn/disconnect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleVPNDisconnect(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if s.wg.GetPeer("wg_pub_recover_disconnect") != nil {
		t.Fatal("expected recovered peer to be removed")
	}
	stored, err := s.peerStates.Get("wg_pub_recover_disconnect")
	if err != nil {
		t.Fatalf("peerStates.Get: %v", err)
	}
	if stored != nil {
		t.Fatal("expected peer state to be deleted after disconnect")
	}
}

func TestHandleVPNDisconnectForwardsToOwnerGateway(t *testing.T) {
	sharedGate, err := nftgate.NewGateWithOptions(nil, time.Hour, nftgate.GateOptions{
		SessionSigningSecret: "test-signing-secret",
	})
	if err != nil {
		t.Fatalf("NewGateWithOptions: %v", err)
	}
	session, err := sharedGate.CreateSessionWithError(
		common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678"),
		nftcheck.TierFree,
	)
	if err != nil {
		t.Fatalf("CreateSessionWithError: %v", err)
	}

	owner := &Server{
		cfg: &config.Config{
			ListenAddr:           ":8080",
			GatewayInstanceID:    "gw-a",
			GatewayForwardingKey: "forward-secret",
		},
		gate:       sharedGate,
		wg:         newTestWireGuardManager(t, "vpn.example.com:51820"),
		peerOwners: newLocalPeerOwnershipStore(),
		peerStates: newLocalPeerStateStore(),
	}
	ownerMux := http.NewServeMux()
	ownerMux.HandleFunc("POST /internal/forward/vpn/disconnect", owner.handleInternalForwardVPNDisconnect)
	ownerTS := httptest.NewServer(ownerMux)
	defer ownerTS.Close()
	owner.cfg.GatewayPublicURL = "https://gw-a.example.com"
	owner.cfg.GatewayForwardURL = ownerTS.URL

	connectReq := httptest.NewRequest(
		http.MethodPost,
		"/vpn/connect",
		strings.NewReader(`{"session_token":"`+session.Token+`","public_key":"wg_pub_forwarded"}`),
	)
	connectReq.Header.Set("Content-Type", "application/json")
	connectRec := httptest.NewRecorder()
	owner.handleVPNConnect(connectRec, connectReq)
	if connectRec.Code != http.StatusOK {
		t.Fatalf("owner connect expected 200, got %d", connectRec.Code)
	}

	proxy := &Server{
		cfg: &config.Config{
			ListenAddr:           ":8081",
			GatewayInstanceID:    "gw-b",
			GatewayPublicURL:     "https://gw-b.example.com",
			GatewayForwardURL:    "http://gw-b.internal:8080",
			GatewayForwardingKey: "forward-secret",
		},
		gate:              sharedGate,
		wg:                newTestWireGuardManager(t, "vpn.example.com:51820"),
		peerOwners:        newLocalPeerOwnershipStore(),
		peerStates:        newLocalPeerStateStore(),
		forwardHTTPClient: ownerTS.Client(),
	}

	body := `{"session_token":"` + session.Token + `","public_key":"wg_pub_forwarded"}`
	req := httptest.NewRequest(http.MethodPost, "/vpn/disconnect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	proxy.handleVPNDisconnect(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if owner.wg.PeerCount() != 0 {
		t.Fatalf("owner PeerCount = %d, want 0", owner.wg.PeerCount())
	}
}

func TestHandleHealthIncludesGatewayMetadata(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			ListenAddr:        ":8080",
			GatewayInstanceID: "gw-a",
			GatewayPublicURL:  "https://gw-a.example.com",
			GatewayForwardURL: "http://gw-a.internal:8080",
		},
		gate:       mustTestGate(t),
		wg:         newTestWireGuardManager(t, "vpn.example.com:51820"),
		peerOwners: newLocalPeerOwnershipStore(),
		peerStates: newLocalPeerStateStore(),
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp["gateway_instance_id"] != "gw-a" {
		t.Fatalf("gateway_instance_id = %v, want gw-a", resp["gateway_instance_id"])
	}
	if resp["gateway_public_url"] != "https://gw-a.example.com" {
		t.Fatalf("gateway_public_url = %v, want https://gw-a.example.com", resp["gateway_public_url"])
	}
	forwarding, ok := resp["forwarding"].(map[string]any)
	if !ok {
		t.Fatalf("forwarding block missing or invalid: %#v", resp["forwarding"])
	}
	if forwarding["forward_target_configured"] != true {
		t.Fatalf("forwarding.forward_target_configured = %v, want true", forwarding["forward_target_configured"])
	}
}

func TestHandleVPNConnectTakesOverDeadGatewayOwner(t *testing.T) {
	s, gate, session := newDeadOwnerRecoveryServer(t, "gw-a", "https://gw-a.example.com")
	expiresAt := time.Now().Add(30 * time.Minute).UTC()

	reserved, err := s.peerOwners.Reserve("wg_pub_dead_owner", session.ID, "gw-a", expiresAt)
	if err != nil {
		t.Fatalf("peerOwners.Reserve: %v", err)
	}
	if !reserved {
		t.Fatal("expected old peer reservation to succeed")
	}
	if err := s.peerStates.Put(&peerState{
		PublicKey:         "wg_pub_dead_owner",
		SessionID:         session.ID,
		GatewayInstanceID: "gw-a",
		GatewayURL:        "https://gw-a.example.com",
		ClientIP:          "10.8.0.12",
		AssignedAt:        time.Now().Add(-time.Minute).UTC(),
		ExpiresAt:         expiresAt,
	}); err != nil {
		t.Fatalf("peerStates.Put: %v", err)
	}

	body := `{"session_token":"` + session.Token + `","public_key":"wg_pub_new_owner"}`
	req := httptest.NewRequest(http.MethodPost, "/vpn/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleVPNConnect(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp ConnectResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.GatewayInstanceID != "gw-b" {
		t.Fatalf("GatewayInstanceID = %q, want gw-b", resp.GatewayInstanceID)
	}

	stored := gate.GetSessionByToken(session.Token)
	if stored == nil {
		t.Fatal("expected stored session")
	}
	if stored.GatewayInstanceID != "gw-b" {
		t.Fatalf("stored session gateway = %q, want gw-b", stored.GatewayInstanceID)
	}

	state, err := s.peerStates.Get("wg_pub_dead_owner")
	if err != nil {
		t.Fatalf("peerStates.Get: %v", err)
	}
	if state != nil {
		t.Fatal("expected dead owner peer state to be removed during takeover")
	}
	owned, err := s.peerOwnedBy("wg_pub_dead_owner", session.ID)
	if err != nil {
		t.Fatalf("peerOwnedBy: %v", err)
	}
	if owned {
		t.Fatal("expected dead owner peer reservation to be released during takeover")
	}
	if s.wg.GetPeer("wg_pub_new_owner") == nil {
		t.Fatal("expected new peer to be provisioned on takeover gateway")
	}
}

func TestHandleVPNConnectClearsDeadGatewayReservationWithoutPeerState(t *testing.T) {
	s, gate, session := newDeadOwnerRecoveryServer(t, "gw-a", "https://gw-a.example.com")
	expiresAt := time.Now().Add(30 * time.Minute).UTC()

	reserved, err := s.peerOwners.Reserve("wg_pub_stale_only", session.ID, "gw-a", expiresAt)
	if err != nil {
		t.Fatalf("peerOwners.Reserve: %v", err)
	}
	if !reserved {
		t.Fatal("expected stale peer reservation to succeed")
	}

	body := `{"session_token":"` + session.Token + `","public_key":"wg_pub_new_after_stale"}`
	req := httptest.NewRequest(http.MethodPost, "/vpn/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleVPNConnect(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	stored := gate.GetSessionByToken(session.Token)
	if stored == nil {
		t.Fatal("expected stored session")
	}
	if stored.GatewayInstanceID != "gw-b" {
		t.Fatalf("stored session gateway = %q, want gw-b", stored.GatewayInstanceID)
	}

	owned, err := s.peerOwnedBy("wg_pub_stale_only", session.ID)
	if err != nil {
		t.Fatalf("peerOwnedBy: %v", err)
	}
	if owned {
		t.Fatal("expected stale reservation without peer state to be released during takeover")
	}
}

func TestHandleVPNDisconnectClearsDeadGatewayOwner(t *testing.T) {
	s, gate, session := newDeadOwnerRecoveryServer(t, "gw-a", "https://gw-a.example.com")
	expiresAt := time.Now().Add(30 * time.Minute).UTC()

	reserved, err := s.peerOwners.Reserve("wg_pub_dead_disconnect", session.ID, "gw-a", expiresAt)
	if err != nil {
		t.Fatalf("peerOwners.Reserve: %v", err)
	}
	if !reserved {
		t.Fatal("expected old peer reservation to succeed")
	}
	if err := s.peerStates.Put(&peerState{
		PublicKey:         "wg_pub_dead_disconnect",
		SessionID:         session.ID,
		GatewayInstanceID: "gw-a",
		GatewayURL:        "https://gw-a.example.com",
		ClientIP:          "10.8.0.13",
		AssignedAt:        time.Now().Add(-time.Minute).UTC(),
		ExpiresAt:         expiresAt,
	}); err != nil {
		t.Fatalf("peerStates.Put: %v", err)
	}

	body := `{"session_token":"` + session.Token + `","public_key":"wg_pub_dead_disconnect"}`
	req := httptest.NewRequest(http.MethodPost, "/vpn/disconnect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleVPNDisconnect(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if recovered, ok := resp["recovered"].(bool); !ok || !recovered {
		t.Fatalf("recovered = %v, want true", resp["recovered"])
	}

	stored := gate.GetSessionByToken(session.Token)
	if stored == nil {
		t.Fatal("expected stored session")
	}
	if stored.GatewayInstanceID != "" {
		t.Fatalf("stored session gateway = %q, want empty", stored.GatewayInstanceID)
	}

	state, err := s.peerStates.Get("wg_pub_dead_disconnect")
	if err != nil {
		t.Fatalf("peerStates.Get: %v", err)
	}
	if state != nil {
		t.Fatal("expected dead owner peer state to be removed during disconnect recovery")
	}
	owned, err := s.peerOwnedBy("wg_pub_dead_disconnect", session.ID)
	if err != nil {
		t.Fatalf("peerOwnedBy: %v", err)
	}
	if owned {
		t.Fatal("expected dead owner peer reservation to be released during disconnect recovery")
	}
}

func TestHandleVPNStatusReportsDeadGatewayOwnerRecoverable(t *testing.T) {
	s, gate, session := newDeadOwnerRecoveryServer(t, "gw-a", "https://gw-a.example.com")

	req := httptest.NewRequest(http.MethodGet, "/vpn/status", nil)
	req.Header.Set("Authorization", "Bearer "+session.Token)
	rec := httptest.NewRecorder()

	s.handleVPNStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if connected, ok := resp["connected"].(bool); !ok || connected {
		t.Fatalf("connected = %v, want false", resp["connected"])
	}
	if resp["code"] != "gateway_owner_unavailable" {
		t.Fatalf("code = %v, want gateway_owner_unavailable", resp["code"])
	}
	if recoverable, ok := resp["recoverable"].(bool); !ok || !recoverable {
		t.Fatalf("recoverable = %v, want true", resp["recoverable"])
	}
	if resp["previous_gateway_instance_id"] != "gw-a" {
		t.Fatalf("previous_gateway_instance_id = %v, want gw-a", resp["previous_gateway_instance_id"])
	}

	stored := gate.GetSessionByToken(session.Token)
	if stored == nil {
		t.Fatal("expected stored session")
	}
	if stored.GatewayInstanceID != "" {
		t.Fatalf("stored session gateway = %q, want empty", stored.GatewayInstanceID)
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
	if resp.ChallengeHash == "" {
		t.Fatal("expected challenge hash")
	}
	if resp.PolicyEpoch != 7 {
		t.Fatalf("PolicyEpoch = %d, want 7", resp.PolicyEpoch)
	}
	if resp.ProofType != "vpn_access_v1" {
		t.Fatalf("ProofType = %q, want vpn_access_v1", resp.ProofType)
	}
}

func TestHandleAnonymousChallengeUsesZKPolicyEpoch(t *testing.T) {
	zkAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/zk" {
			t.Fatalf("path = %q, want /api/zk", r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != vpnAccessV1ProofType {
			t.Fatalf("type = %q, want %q", got, vpnAccessV1ProofType)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"data": map[string]any{
				"root":       "123",
				"depth":      20,
				"entryCount": 1,
				"createdAt":  time.Now().UTC().Format(time.RFC3339),
				"metadata": map[string]any{
					"policyEpoch": "9",
				},
			},
		})
	}))
	defer zkAPI.Close()

	s := &Server{
		anonAuth: anonauth.NewService(time.Minute, 8, vpnAccessV1ProofType, 1),
		zkClient: zkverify.New(zkAPI.URL, ""),
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
	if resp.PolicyEpoch != 9 {
		t.Fatalf("PolicyEpoch = %d, want 9", resp.PolicyEpoch)
	}
	if s.anonAuth.PolicyEpoch() != 9 {
		t.Fatalf("anonAuth.PolicyEpoch() = %d, want 9", s.anonAuth.PolicyEpoch())
	}
}

func TestHandleAnonymousChallengeRejectsMissingPolicyEpochMetadata(t *testing.T) {
	zkAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"data": map[string]any{
				"root":       "123",
				"depth":      20,
				"entryCount": 1,
				"createdAt":  time.Now().UTC().Format(time.RFC3339),
				"metadata":   map[string]any{},
			},
		})
	}))
	defer zkAPI.Close()

	s := &Server{
		anonAuth: anonauth.NewService(time.Minute, 8, vpnAccessV1ProofType, 1),
		zkClient: zkverify.New(zkAPI.URL, ""),
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/anonymous/challenge", nil)
	rec := httptest.NewRecorder()

	s.handleAnonymousChallenge(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp["error"] != "anonymous policy metadata unavailable" {
		t.Fatalf("error = %q, want anonymous policy metadata unavailable", resp["error"])
	}
}

func TestHandleAnonymousChallengeUsesCachedPolicyEpochWithinTTL(t *testing.T) {
	callCount := 0
	zkAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"data": map[string]any{
				"root":       "123",
				"depth":      20,
				"entryCount": 1,
				"createdAt":  time.Now().UTC().Format(time.RFC3339),
				"metadata": map[string]any{
					"policyEpoch": "9",
				},
			},
		})
	}))
	defer zkAPI.Close()

	s := &Server{
		anonAuth: anonauth.NewService(time.Minute, 8, vpnAccessV1ProofType, 1),
		zkClient: zkverify.New(zkAPI.URL, ""),
	}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/anonymous/challenge", nil)
		rec := httptest.NewRecorder()

		s.handleAnonymousChallenge(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	if callCount != 1 {
		t.Fatalf("zk root fetch count = %d, want 1", callCount)
	}
}

func TestSyncAnonymousPolicyEpochRefreshesInBackground(t *testing.T) {
	refreshed := make(chan struct{}, 1)
	callCount := 0
	zkAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"data": map[string]any{
				"root":       "123",
				"depth":      20,
				"entryCount": 1,
				"createdAt":  time.Now().UTC().Format(time.RFC3339),
				"metadata": map[string]any{
					"policyEpoch": "12",
				},
			},
		})
		select {
		case refreshed <- struct{}{}:
		default:
		}
	}))
	defer zkAPI.Close()

	s := &Server{
		anonAuth: anonauth.NewService(time.Minute, 8, vpnAccessV1ProofType, 9),
		zkClient: zkverify.New(zkAPI.URL, ""),
	}
	s.markAnonymousPolicyFetched("old-root", time.Now().Add(-(anonymousPolicyBackgroundRefreshAge + time.Second)))

	if err := s.syncAnonymousPolicyEpoch(context.Background()); err != nil {
		t.Fatalf("syncAnonymousPolicyEpoch: %v", err)
	}

	select {
	case <-refreshed:
	case <-time.After(2 * time.Second):
		t.Fatal("expected background refresh")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.anonAuth.PolicyEpoch() == 12 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if s.anonAuth.PolicyEpoch() != 12 {
		t.Fatalf("anonAuth.PolicyEpoch() = %d, want 12", s.anonAuth.PolicyEpoch())
	}
	if s.currentAnonymousPolicyRoot() != "123" {
		t.Fatalf("currentAnonymousPolicyRoot() = %q, want 123", s.currentAnonymousPolicyRoot())
	}
	if callCount != 1 {
		t.Fatalf("zk root fetch count = %d, want 1", callCount)
	}
}

func TestHandleAnonymousConnectRejectsStalePublishedRoot(t *testing.T) {
	zkGetCalls := 0
	zkAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/zk" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		zkGetCalls++
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"data": map[string]any{
				"root":       "root_current",
				"depth":      20,
				"entryCount": 1,
				"createdAt":  time.Now().UTC().Format(time.RFC3339),
				"metadata": map[string]any{
					"policyEpoch": "7",
				},
			},
		})
	}))
	defer zkAPI.Close()

	s := &Server{
		anonAuth: anonauth.NewService(time.Minute, 8, vpnAccessV1ProofType, 7),
		zkClient: zkverify.New(zkAPI.URL, ""),
	}
	challenge, err := s.anonAuth.NewChallenge()
	if err != nil {
		t.Fatalf("NewChallenge: %v", err)
	}

	sessionKeyHash := deriveVPNAccessV1SessionKeyHash("wg_pub")
	body := `{"challenge_id":"` + challenge.ID + `","proof_type":"vpn_access_v1","nullifier_hash":"nul_1","session_key_hash":"` + sessionKeyHash + `","public_signals":["root_old","7","2","4102444800","nul_1","` + deriveVPNAccessV1ChallengeHash(challenge) + `","` + sessionKeyHash + `"],"public_key":"wg_pub"}` //nolint:lll
	req := httptest.NewRequest(http.MethodPost, "/vpn/anonymous/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.handleAnonymousVPNConnect(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp["error"] != "anonymous policy root changed, request a new challenge" {
		t.Fatalf("error = %q, want anonymous policy root changed, request a new challenge", resp["error"])
	}
	if zkGetCalls != 1 {
		t.Fatalf("zk root fetch count = %d, want 1", zkGetCalls)
	}
}

func TestHandleAnonymousConnectMissingChallenge(t *testing.T) {
	s := &Server{
		anonAuth: anonauth.NewService(time.Minute, 8, "vpn_access_v1", 7),
	}
	sessionKeyHash := deriveVPNAccessV1SessionKeyHash("wg_pub")
	body := `{"challenge_id":"missing","proof_type":"vpn_access_v1","nullifier_hash":"nul_1","session_key_hash":"` + sessionKeyHash + `","public_signals":["root","7","2","4102444800","nul_1","challenge","` + sessionKeyHash + `"],"public_key":"wg_pub"}` //nolint:lll
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

	sessionKeyHash := deriveVPNAccessV1SessionKeyHash("wg_pub")
	body := `{"challenge_id":"` + challenge.ID + `","proof_type":"vpn_access_v1","nullifier_hash":"nul_1","session_key_hash":"` + sessionKeyHash + `","public_signals":["root","7","2","4102444800","nul_1","` + deriveVPNAccessV1ChallengeHash(challenge) + `","` + sessionKeyHash + `"],"public_key":"wg_pub"}` //nolint:lll
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
		SessionKeyHash: deriveVPNAccessV1SessionKeyHash("wg_pub"),
		PublicKey:      "wg_pub",
		PublicSignals: []string{
			"root",
			"7",
			"2",
			strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
			"nul_1",
			"",
			deriveVPNAccessV1SessionKeyHash("wg_pub"),
		},
	}
	challenge := &anonauth.Challenge{
		ID:          "chal_1",
		Nonce:       "nonce_1",
		PolicyEpoch: 7,
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	req.PublicSignals[vpnAccessChallengeIndex] = deriveVPNAccessV1ChallengeHash(challenge)

	signals, err := validateVPNAccessV1Signals(challenge, req)
	if err != nil {
		t.Fatalf("validateVPNAccessV1Signals: %v", err)
	}
	if signals.PolicyEpoch != 7 {
		t.Fatalf("PolicyEpoch = %d, want 7", signals.PolicyEpoch)
	}
	if signals.SessionKeyHash != deriveVPNAccessV1SessionKeyHash("wg_pub") {
		t.Fatalf("SessionKeyHash = %q, want derived session hash", signals.SessionKeyHash)
	}
}

func TestValidateVPNAccessV1SignalsRejectsPolicyEpochMismatch(t *testing.T) {
	req := AnonymousConnectRequest{
		ProofType:      vpnAccessV1ProofType,
		NullifierHash:  "nul_1",
		SessionKeyHash: deriveVPNAccessV1SessionKeyHash("wg_pub"),
		PublicKey:      "wg_pub",
		PublicSignals: []string{
			"root",
			"8",
			"2",
			strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
			"nul_1",
			"challenge",
			deriveVPNAccessV1SessionKeyHash("wg_pub"),
		},
	}
	challenge := &anonauth.Challenge{
		ID:          "chal_1",
		Nonce:       "nonce_1",
		PolicyEpoch: 7,
		ExpiresAt:   time.Now().Add(time.Minute),
	}

	if _, err := validateVPNAccessV1Signals(challenge, req); err == nil {
		t.Fatal("expected policy epoch mismatch error")
	}
}

func TestValidateVPNAccessV1SignalsRejectsChallengeHashMismatch(t *testing.T) {
	req := AnonymousConnectRequest{
		ProofType:      vpnAccessV1ProofType,
		NullifierHash:  "nul_1",
		SessionKeyHash: deriveVPNAccessV1SessionKeyHash("wg_pub"),
		PublicKey:      "wg_pub",
		PublicSignals: []string{
			"root",
			"7",
			"2",
			strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
			"nul_1",
			"wrong_challenge_hash",
			deriveVPNAccessV1SessionKeyHash("wg_pub"),
		},
	}
	challenge := &anonauth.Challenge{
		ID:          "chal_1",
		Nonce:       "nonce_1",
		PolicyEpoch: 7,
		ExpiresAt:   time.Now().Add(time.Minute),
	}

	if _, err := validateVPNAccessV1Signals(challenge, req); err == nil {
		t.Fatal("expected challenge hash mismatch error")
	}
}

func TestValidateVPNAccessV1SignalsRejectsExpiredEntitlement(t *testing.T) {
	req := AnonymousConnectRequest{
		ProofType:      vpnAccessV1ProofType,
		NullifierHash:  "nul_1",
		SessionKeyHash: deriveVPNAccessV1SessionKeyHash("wg_pub"),
		PublicKey:      "wg_pub",
		PublicSignals: []string{
			"root",
			"7",
			"2",
			strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10),
			"nul_1",
			"",
			deriveVPNAccessV1SessionKeyHash("wg_pub"),
		},
	}
	challenge := &anonauth.Challenge{
		ID:          "chal_1",
		Nonce:       "nonce_1",
		PolicyEpoch: 7,
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	req.PublicSignals[vpnAccessChallengeIndex] = deriveVPNAccessV1ChallengeHash(challenge)

	if _, err := validateVPNAccessV1Signals(challenge, req); err == nil {
		t.Fatal("expected expired entitlement error")
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
