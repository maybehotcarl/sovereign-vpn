package rep6529

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mock6529API returns a test server that mimics the 6529 rep API.
func mock6529API(repByWallet map[string]int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := r.URL.Path

		// GET /api/profiles/{identity}/rep/ratings/by-rater
		if strings.Contains(path, "/ratings/by-rater") {
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"handle": "supporter1", "tdh": 500000, "rating": 30000, "level": 40, "wallets": []string{"0x111"}},
					{"handle": "supporter2", "tdh": 200000, "rating": 20000, "level": 30, "wallets": []string{"0x222"}},
				},
			})
			return
		}

		// GET /api/profiles/{identity}/rep/rating?category=...
		if strings.Contains(path, "/rep/rating") {
			parts := strings.Split(path, "/")
			// path: /api/profiles/{identity}/rep/rating
			var identity string
			for i, p := range parts {
				if p == "profiles" && i+1 < len(parts) {
					identity = parts[i+1]
					break
				}
			}

			rating, ok := repByWallet[identity]
			if !ok {
				rating = 0
			}

			json.NewEncoder(w).Encode(map[string]int64{"rating": rating})
			return
		}

		// GET /api/identities/{identity}
		if strings.Contains(path, "/identities/") {
			parts := strings.Split(path, "/")
			identity := parts[len(parts)-1]

			if identity == "unknown" {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			json.NewEncoder(w).Encode(map[string]any{
				"handle":  identity,
				"rep":     repByWallet[identity],
				"tdh":     1000000,
				"level":   50,
				"display": identity,
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestCheckRepEligible(t *testing.T) {
	api := mock6529API(map[string]int64{
		"0xOperator1": 75000, // above 50k threshold
	})
	defer api.Close()

	c := NewChecker(Config{
		BaseURL:  api.URL + "/api",
		Category: "VPN Operator",
		MinRep:   50000,
		CacheTTL: time.Minute,
	})

	result, err := c.CheckRep(context.Background(), "0xOperator1")
	if err != nil {
		t.Fatalf("CheckRep: %v", err)
	}

	if !result.Eligible {
		t.Errorf("expected eligible (75000 >= 50000)")
	}
	if result.Rating != 75000 {
		t.Errorf("expected rating 75000, got %d", result.Rating)
	}
}

func TestCheckRepNotEligible(t *testing.T) {
	api := mock6529API(map[string]int64{
		"0xNewbie": 10000, // below 50k threshold
	})
	defer api.Close()

	c := NewChecker(Config{
		BaseURL:  api.URL + "/api",
		Category: "VPN Operator",
		MinRep:   50000,
		CacheTTL: time.Minute,
	})

	result, err := c.CheckRep(context.Background(), "0xNewbie")
	if err != nil {
		t.Fatalf("CheckRep: %v", err)
	}

	if result.Eligible {
		t.Errorf("expected not eligible (10000 < 50000)")
	}
	if result.Rating != 10000 {
		t.Errorf("expected rating 10000, got %d", result.Rating)
	}
}

func TestCheckRepUnknownWallet(t *testing.T) {
	api := mock6529API(map[string]int64{})
	defer api.Close()

	c := NewChecker(Config{
		BaseURL:  api.URL + "/api",
		Category: "VPN Operator",
		MinRep:   50000,
		CacheTTL: time.Minute,
	})

	result, err := c.CheckRep(context.Background(), "0xNobody")
	if err != nil {
		t.Fatalf("CheckRep: %v", err)
	}

	if result.Eligible {
		t.Errorf("expected not eligible for unknown wallet")
	}
	if result.Rating != 0 {
		t.Errorf("expected rating 0, got %d", result.Rating)
	}
}

func TestCheckRepCaching(t *testing.T) {
	callCount := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"rating": 60000})
	}))
	defer api.Close()

	c := NewChecker(Config{
		BaseURL:  api.URL + "/api",
		Category: "VPN Operator",
		MinRep:   50000,
		CacheTTL: time.Minute,
	})

	// First call hits API
	_, err := c.CheckRep(context.Background(), "0xCached")
	if err != nil {
		t.Fatalf("first CheckRep: %v", err)
	}

	// Second call should use cache
	_, err = c.CheckRep(context.Background(), "0xCached")
	if err != nil {
		t.Fatalf("second CheckRep: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 API call (cached), got %d", callCount)
	}
}

func TestCheckRepCacheInvalidation(t *testing.T) {
	callCount := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"rating": 60000})
	}))
	defer api.Close()

	c := NewChecker(Config{
		BaseURL:  api.URL + "/api",
		Category: "VPN Operator",
		MinRep:   50000,
		CacheTTL: time.Minute,
	})

	c.CheckRep(context.Background(), "0xCached")
	c.InvalidateCache("0xCached")
	c.CheckRep(context.Background(), "0xCached")

	if callCount != 2 {
		t.Errorf("expected 2 API calls after invalidation, got %d", callCount)
	}
}

func TestGetIdentity(t *testing.T) {
	api := mock6529API(map[string]int64{"testuser": 100000})
	defer api.Close()

	c := NewChecker(Config{BaseURL: api.URL + "/api"})

	id, err := c.GetIdentity(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("GetIdentity: %v", err)
	}
	if id == nil {
		t.Fatal("expected non-nil identity")
	}
	if id.Handle != "testuser" {
		t.Errorf("expected handle 'testuser', got %q", id.Handle)
	}
	if id.Level != 50 {
		t.Errorf("expected level 50, got %d", id.Level)
	}
}

func TestGetIdentityUnknown(t *testing.T) {
	api := mock6529API(map[string]int64{})
	defer api.Close()

	c := NewChecker(Config{BaseURL: api.URL + "/api"})

	id, err := c.GetIdentity(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("GetIdentity: %v", err)
	}
	if id != nil {
		t.Errorf("expected nil identity for unknown wallet, got %+v", id)
	}
}

func TestGetRepBreakdown(t *testing.T) {
	api := mock6529API(map[string]int64{"0xOp": 50000})
	defer api.Close()

	c := NewChecker(Config{
		BaseURL:  api.URL + "/api",
		Category: "VPN Operator",
	})

	breakdown, err := c.GetRepBreakdown(context.Background(), "0xOp")
	if err != nil {
		t.Fatalf("GetRepBreakdown: %v", err)
	}
	if len(breakdown) != 2 {
		t.Fatalf("expected 2 contributions, got %d", len(breakdown))
	}
	if breakdown[0].Handle != "supporter1" {
		t.Errorf("expected supporter1, got %q", breakdown[0].Handle)
	}
	if breakdown[0].Rating != 30000 {
		t.Errorf("expected 30000, got %d", breakdown[0].Rating)
	}
}

func TestDefaultConfig(t *testing.T) {
	c := NewChecker(Config{})

	if c.baseURL != DefaultBaseURL {
		t.Errorf("expected default base URL, got %q", c.baseURL)
	}
	if c.category != DefaultCategory {
		t.Errorf("expected default category, got %q", c.category)
	}
	if c.minRep != DefaultMinRep {
		t.Errorf("expected default min rep %d, got %d", DefaultMinRep, c.minRep)
	}
}

func TestMinRepAndCategory(t *testing.T) {
	c := NewChecker(Config{
		Category: "Custom Category",
		MinRep:   100000,
	})

	if c.MinRepRequired() != 100000 {
		t.Errorf("expected 100000, got %d", c.MinRepRequired())
	}
	if c.Category() != "Custom Category" {
		t.Errorf("expected 'Custom Category', got %q", c.Category())
	}
}
