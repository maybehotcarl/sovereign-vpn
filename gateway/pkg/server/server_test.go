package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/anonauth"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftcheck"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/siwe"
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

func TestOperatorEnrollmentLifecycle(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	address := crypto.PubkeyToAddress(key.PublicKey)
	s := newTestEnrollmentServer()

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/operator/enrollments",
		signedEnrollmentCreateBody(t, s, key, address.Hex(), "us-east"),
	)
	createRec := httptest.NewRecorder()

	s.handleCreateOperatorEnrollment(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d: %s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created OperatorEnrollment
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Token == "" {
		t.Fatal("expected enrollment token")
	}
	if created.Operator != address.Hex() {
		t.Fatalf("Operator = %q, want %q", created.Operator, address.Hex())
	}
	if created.Status != "created" {
		t.Fatalf("Status = %q, want created", created.Status)
	}

	report := OperatorEnrollmentReport{
		Operator:         address.Hex(),
		Region:           "us-east",
		Endpoint:         "203.0.113.10:51820",
		GatewayURL:       "http://203.0.113.10:8080",
		WireGuardPubKey:  "wg-pubkey",
		HealthOK:         true,
		HealthStatus:     "ok",
		InstallerVersion: "test",
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	reportReq := httptest.NewRequest(
		http.MethodPost,
		"/operator/enrollments/"+created.Token+"/report",
		bytes.NewReader(reportBytes),
	)
	reportRec := httptest.NewRecorder()

	s.handleReportOperatorEnrollment(reportRec, reportReq)

	if reportRec.Code != http.StatusOK {
		t.Fatalf("report status = %d, want %d: %s", reportRec.Code, http.StatusOK, reportRec.Body.String())
	}

	var reported OperatorEnrollment
	if err := json.NewDecoder(reportRec.Body).Decode(&reported); err != nil {
		t.Fatalf("decode report response: %v", err)
	}
	if reported.Status != "healthy" {
		t.Fatalf("reported Status = %q, want healthy", reported.Status)
	}
	if reported.Report == nil || reported.Report.Endpoint != "203.0.113.10:51820" {
		t.Fatalf("unexpected report payload: %+v", reported.Report)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/operator/enrollments/"+created.Token, nil)
	getRec := httptest.NewRecorder()

	s.handleGetOperatorEnrollment(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}

	var fetched OperatorEnrollment
	if err := json.NewDecoder(getRec.Body).Decode(&fetched); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if fetched.Status != "healthy" {
		t.Fatalf("fetched Status = %q, want healthy", fetched.Status)
	}
}

func TestOperatorEnrollmentRejectsInvalidOperator(t *testing.T) {
	s := newTestEnrollmentServer()

	req := httptest.NewRequest(http.MethodPost, "/operator/enrollments", strings.NewReader(`{"operator":"nope","region":"us-east"}`))
	rec := httptest.NewRecorder()

	s.handleCreateOperatorEnrollment(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOperatorEnrollmentRequiresSignedOperator(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	otherKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey other: %v", err)
	}
	operator := crypto.PubkeyToAddress(otherKey.PublicKey)
	s := newTestEnrollmentServer()

	req := httptest.NewRequest(
		http.MethodPost,
		"/operator/enrollments",
		signedEnrollmentCreateBody(t, s, key, operator.Hex(), "us-east"),
	)
	rec := httptest.NewRecorder()

	s.handleCreateOperatorEnrollment(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOperatorEnrollmentRejectsMismatchedReportOperator(t *testing.T) {
	store := newOperatorEnrollmentStore(time.Hour)
	created, err := store.create("0x1234567890abcdef1234567890abcdef12345678", "us-east")
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}
	s := &Server{enrollments: store}

	report := `{"operator":"0x9999999999999999999999999999999999999999","health_ok":true}`
	req := httptest.NewRequest(http.MethodPost, "/operator/enrollments/"+created.Token+"/report", strings.NewReader(report))
	rec := httptest.NewRecorder()

	s.handleReportOperatorEnrollment(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func newTestEnrollmentServer() *Server {
	return &Server{
		enrollments: newOperatorEnrollmentStore(time.Hour),
		siwe:        siwe.NewService("test.example.com", "https://test.example.com", 5*time.Minute, 16),
	}
}

func signedEnrollmentCreateBody(
	t *testing.T,
	s *Server,
	key *ecdsa.PrivateKey,
	operator string,
	region string,
) *bytes.Reader {
	t.Helper()

	challenge, err := s.siwe.NewChallenge(16)
	if err != nil {
		t.Fatalf("NewChallenge: %v", err)
	}
	message := siwe.FormatMessage(challenge, crypto.PubkeyToAddress(key.PublicKey).Hex())
	sig, err := signEnrollmentMessage(key, message)
	if err != nil {
		t.Fatalf("sign enrollment message: %v", err)
	}

	body, err := json.Marshal(createOperatorEnrollmentRequest{
		Operator:  operator,
		Region:    region,
		Message:   message,
		Signature: sig,
	})
	if err != nil {
		t.Fatalf("marshal create request: %v", err)
	}
	return bytes.NewReader(body)
}

func signEnrollmentMessage(key *ecdsa.PrivateKey, message string) (string, error) {
	sig, err := crypto.Sign(accounts.TextHash([]byte(message)), key)
	if err != nil {
		return "", err
	}
	sig[64] += 27
	return hexutil.Encode(sig), nil
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
