package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAllow(t *testing.T) {
	l := New(3, time.Minute)
	defer l.Stop()

	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("4th request should be denied")
	}
	// Different IP should still be allowed
	if !l.Allow("5.6.7.8") {
		t.Fatal("different IP should be allowed")
	}
}

func TestWindowReset(t *testing.T) {
	l := New(1, 50*time.Millisecond)
	defer l.Stop()

	if !l.Allow("1.2.3.4") {
		t.Fatal("first request should be allowed")
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("second request should be denied")
	}

	time.Sleep(60 * time.Millisecond)

	if !l.Allow("1.2.3.4") {
		t.Fatal("request after window reset should be allowed")
	}
}

func TestWrapMiddleware(t *testing.T) {
	l := New(2, time.Minute)
	defer l.Stop()

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := l.Wrap(ok)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd request: got %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("missing Retry-After header")
	}
}

func TestExtractIP_XFF(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 70.41.3.18")

	ip := extractIP(req)
	if ip != "203.0.113.5" {
		t.Fatalf("got %q, want 203.0.113.5", ip)
	}
}

func TestExtractIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:9999"

	ip := extractIP(req)
	if ip != "192.168.1.1" {
		t.Fatalf("got %q, want 192.168.1.1", ip)
	}
}
