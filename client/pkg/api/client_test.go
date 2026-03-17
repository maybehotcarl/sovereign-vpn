package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetChallenge(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/auth/challenge" {
			t.Errorf("expected /auth/challenge, got %s", r.URL.Path)
		}

		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if req["address"] != "0xABC" {
			t.Errorf("expected address 0xABC, got %s", req["address"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChallengeResponse{
			Message: "test-message",
			Nonce:   "test-nonce",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	resp, err := c.GetChallenge("0xABC")
	if err != nil {
		t.Fatalf("GetChallenge: %v", err)
	}
	if resp.Message != "test-message" {
		t.Errorf("expected test-message, got %s", resp.Message)
	}
	if resp.Nonce != "test-nonce" {
		t.Errorf("expected test-nonce, got %s", resp.Nonce)
	}
}

func TestVerify(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)

		if req["message"] == "" || req["signature"] == "" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "missing fields"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VerifyResponse{
			Address:   "0x1234",
			Tier:      "free",
			ExpiresAt: "2026-02-19T00:00:00Z",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	resp, err := c.Verify("msg", "sig")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if resp.Tier != "free" {
		t.Errorf("expected tier free, got %s", resp.Tier)
	}
}

func TestGetAnonymousChallenge(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/auth/anonymous/challenge" {
			t.Errorf("expected /auth/anonymous/challenge, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AnonymousChallengeResponse{
			ChallengeID:   "chal_123",
			Nonce:         "nonce_123",
			ChallengeHash: "123456",
			PolicyEpoch:   9,
			ProofType:     "vpn_access_v1",
			ExpiresAt:     "2026-03-16T00:00:00Z",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	resp, err := c.GetAnonymousChallenge()
	if err != nil {
		t.Fatalf("GetAnonymousChallenge: %v", err)
	}
	if resp.PolicyEpoch != 9 {
		t.Errorf("expected policy epoch 9, got %d", resp.PolicyEpoch)
	}
	if resp.ProofType != "vpn_access_v1" {
		t.Errorf("expected proof type vpn_access_v1, got %s", resp.ProofType)
	}
}

func TestConnect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ConnectResponse{
			ServerPublicKey: "server-pub",
			ServerEndpoint:  "1.2.3.4:51820",
			ClientAddress:   "10.8.0.2/24",
			DNS:             "1.1.1.1",
			AllowedIPs:      "0.0.0.0/0, ::/0",
			ExpiresAt:       "2026-02-19T00:00:00Z",
			Tier:            "paid",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	resp, err := c.Connect("0x1234", "client-pub-key")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if resp.ClientAddress != "10.8.0.2/24" {
		t.Errorf("expected 10.8.0.2/24, got %s", resp.ClientAddress)
	}
	if resp.Tier != "paid" {
		t.Errorf("expected paid, got %s", resp.Tier)
	}
}

func TestAnonymousConnect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/vpn/anonymous/connect" {
			t.Errorf("expected /vpn/anonymous/connect, got %s", r.URL.Path)
		}

		var req AnonymousConnectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode request: %v", err)
		}
		if req.ChallengeID != "chal_123" {
			t.Errorf("expected challenge id chal_123, got %s", req.ChallengeID)
		}
		if req.NullifierHash != "nul_123" {
			t.Errorf("expected nullifier nul_123, got %s", req.NullifierHash)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AnonymousConnectResponse{
			SessionToken:    "tok_anon",
			ServerPublicKey: "server-pub",
			ServerEndpoint:  "1.2.3.4:51820",
			ClientAddress:   "10.8.0.2/24",
			DNS:             "1.1.1.1",
			AllowedIPs:      "0.0.0.0/0, ::/0",
			ExpiresAt:       "2026-02-19T00:00:00Z",
			Tier:            "paid",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	resp, err := c.AnonymousConnect(AnonymousConnectRequest{
		ChallengeID:    "chal_123",
		ProofType:      "vpn_access_v1",
		Proof:          map[string]any{"pi_a": []string{"1", "2"}},
		PublicSignals:  []string{"root", "9", "1", "4102444800", "nul_123", "challenge", "session"},
		NullifierHash:  "nul_123",
		SessionKeyHash: "session",
		PublicKey:      "wg_pub",
	})
	if err != nil {
		t.Fatalf("AnonymousConnect: %v", err)
	}
	if resp.SessionToken != "tok_anon" {
		t.Errorf("expected tok_anon, got %s", resp.SessionToken)
	}
	if resp.Tier != "paid" {
		t.Errorf("expected paid, got %s", resp.Tier)
	}
}

func TestDisconnect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "disconnected"})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	err := c.Disconnect("0x1234", "pub-key")
	if err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
}

func TestStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query string, got %q", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer 0xABC" {
			t.Errorf("expected Authorization header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StatusResponse{
			Connected: true,
			Tier:      "free",
			ExpiresAt: "2026-02-19T00:00:00Z",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	resp, err := c.Status("0xABC")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !resp.Connected {
		t.Error("expected connected=true")
	}
}

func TestHealth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected /health, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":          "ok",
			"active_sessions": 0,
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	resp, err := c.Health()
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

func TestErrorParsing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "access denied"})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	_, err := c.GetChallenge("0xABC")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if err.Error() != "gateway error (403): access denied" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}
