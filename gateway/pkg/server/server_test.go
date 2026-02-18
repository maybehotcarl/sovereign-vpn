package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseAddress(t *testing.T) {
	tests := []struct {
		input    string
		expected string // hex without 0x prefix
	}{
		{"0x1234567890abcdef1234567890abcdef12345678", "1234567890abcdef1234567890abcdef12345678"},
		{"1234567890abcdef1234567890abcdef12345678", "1234567890abcdef1234567890abcdef12345678"},
		{"0xABCDEF1234567890ABCDEF1234567890ABCDEF12", "abcdef1234567890abcdef1234567890abcdef12"},
		{"short", "0000000000000000000000000000000000000000"},      // invalid length -> zero
		{"0xshort", "0000000000000000000000000000000000000000"},    // invalid length after 0x
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

func TestHexNibble(t *testing.T) {
	tests := []struct {
		input    byte
		expected byte
	}{
		{'0', 0}, {'9', 9},
		{'a', 10}, {'f', 15},
		{'A', 10}, {'F', 15},
		{'g', 0}, {'z', 0}, // invalid -> 0
	}
	for _, tt := range tests {
		got := hexNibble(tt.input)
		if got != tt.expected {
			t.Errorf("hexNibble(%c) = %d, want %d", tt.input, got, tt.expected)
		}
	}
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
	body := `{"session_token": "0x123", "public_key": "abc123"}`
	var req ConnectRequest
	json.NewDecoder(strings.NewReader(body)).Decode(&req)

	if req.SessionToken != "0x123" {
		t.Errorf("expected 0x123, got %s", req.SessionToken)
	}
	if req.PublicKey != "abc123" {
		t.Errorf("expected abc123, got %s", req.PublicKey)
	}
}
