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
		token := r.URL.Query().Get("session_token")
		if token == "" {
			t.Error("expected session_token param")
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
