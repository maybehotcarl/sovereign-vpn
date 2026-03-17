package zkverify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetMerkleRoot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/zk" {
			t.Fatalf("path = %q, want /api/zk", r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != "vpn_access_v1" {
			t.Fatalf("type = %q, want vpn_access_v1", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q, want Bearer secret", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"data": {
				"root": "123",
				"depth": 20,
				"entryCount": 5,
				"createdAt": "2026-03-16T00:00:00Z",
				"metadata": {
					"policyEpoch": "11"
				}
			}
		}`))
	}))
	defer server.Close()

	client := New(server.URL, "secret")
	result, err := client.GetMerkleRoot(context.Background(), "vpn_access_v1")
	if err != nil {
		t.Fatalf("GetMerkleRoot: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if result.Data == nil {
		t.Fatal("expected data")
	}
	if result.Data.Root != "123" {
		t.Fatalf("Root = %q, want 123", result.Data.Root)
	}
	if got := result.Data.Metadata["policyEpoch"]; got != "11" {
		t.Fatalf("policyEpoch = %v, want 11", got)
	}
}
