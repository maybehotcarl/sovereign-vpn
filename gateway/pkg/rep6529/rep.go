// Package rep6529 queries the 6529 community reputation system (api.6529.io)
// to check if a wallet has sufficient "VPN Operator" rep to run a node.
//
// The 6529 rep system is TDH-weighted and peer-to-peer: any Memes holder can
// give rep to any other user in any free-form category. We use the category
// "VPN Operator" — node operators must accumulate enough community-given rep
// in this category before they can register as a VPN node.
package rep6529

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the 6529 API base URL.
	DefaultBaseURL = "https://api.6529.io/api"

	// DefaultCategory is the rep category for VPN node operators.
	DefaultCategory = "VPN Operator"

	// DefaultMinRep is the minimum rep required to operate a node.
	DefaultMinRep = 50000
)

// Config configures the 6529 rep checker.
type Config struct {
	BaseURL     string        // API base URL (default: https://api.6529.io/api)
	Category    string        // Rep category to check (default: "VPN Operator")
	MinRep      int64         // Minimum rep required (default: 50000)
	CacheTTL    time.Duration // How long to cache rep lookups (default: 5m)
	HTTPTimeout time.Duration // HTTP request timeout (default: 10s)
}

// RepResult holds the result of a rep check.
type RepResult struct {
	Rating    int64     // Total rep in the category
	Eligible  bool      // Whether rating >= MinRep
	CheckedAt time.Time // When this was checked
}

// Identity holds profile info from the 6529 API.
type Identity struct {
	Handle  string `json:"handle"`
	Rep     int64  `json:"rep"`
	TDH     int64  `json:"tdh"`
	Level   int    `json:"level"`
	Display string `json:"display"`
}

type cacheEntry struct {
	result    RepResult
	expiresAt time.Time
}

// Checker queries the 6529 API for VPN Operator rep.
type Checker struct {
	baseURL  string
	category string
	minRep   int64
	cacheTTL time.Duration
	client   *http.Client

	mu    sync.RWMutex
	cache map[string]cacheEntry // wallet address → cached result
}

// NewChecker creates a new 6529 rep checker.
func NewChecker(cfg Config) *Checker {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Category == "" {
		cfg.Category = DefaultCategory
	}
	if cfg.MinRep == 0 {
		cfg.MinRep = DefaultMinRep
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 10 * time.Second
	}

	return &Checker{
		baseURL:  cfg.BaseURL,
		category: cfg.Category,
		minRep:   cfg.MinRep,
		cacheTTL: cfg.CacheTTL,
		client:   &http.Client{Timeout: cfg.HTTPTimeout},
		cache:    make(map[string]cacheEntry),
	}
}

// CheckRep queries the 6529 API for the wallet's rep in the VPN Operator category.
// Returns whether the wallet has sufficient rep to operate a node.
func (c *Checker) CheckRep(ctx context.Context, walletOrHandle string) (RepResult, error) {
	// Check cache
	c.mu.RLock()
	if entry, ok := c.cache[walletOrHandle]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.result, nil
	}
	c.mu.RUnlock()

	// Query 6529 API
	// GET /profiles/{identity}/rep/rating?category=VPN+Operator
	u := fmt.Sprintf("%s/profiles/%s/rep/rating?category=%s",
		c.baseURL,
		url.PathEscape(walletOrHandle),
		url.QueryEscape(c.category),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return RepResult{}, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return RepResult{}, fmt.Errorf("querying 6529 rep API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return RepResult{}, fmt.Errorf("6529 API returned status %d", resp.StatusCode)
	}

	var ratingResp struct {
		Rating int64 `json:"rating"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ratingResp); err != nil {
		return RepResult{}, fmt.Errorf("decoding response: %w", err)
	}

	result := RepResult{
		Rating:    ratingResp.Rating,
		Eligible:  ratingResp.Rating >= c.minRep,
		CheckedAt: time.Now(),
	}

	// Cache result
	c.mu.Lock()
	c.cache[walletOrHandle] = cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.cacheTTL),
	}
	c.mu.Unlock()

	return result, nil
}

// GetIdentity fetches the full 6529 identity for a wallet or handle.
func (c *Checker) GetIdentity(ctx context.Context, walletOrHandle string) (*Identity, error) {
	u := fmt.Sprintf("%s/identities/%s",
		c.baseURL,
		url.PathEscape(walletOrHandle),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying 6529 identity API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // wallet not known to 6529
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("6529 API returned status %d", resp.StatusCode)
	}

	var id Identity
	if err := json.NewDecoder(resp.Body).Decode(&id); err != nil {
		return nil, fmt.Errorf("decoding identity: %w", err)
	}

	return &id, nil
}

// GetRepBreakdown fetches who gave rep to this wallet in the VPN Operator category.
func (c *Checker) GetRepBreakdown(ctx context.Context, walletOrHandle string) ([]RepContribution, error) {
	u := fmt.Sprintf("%s/profiles/%s/rep/ratings/by-rater?category=%s&page_size=50&order=DESC&order_by=rating",
		c.baseURL,
		url.PathEscape(walletOrHandle),
		url.QueryEscape(c.category),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying 6529 rep breakdown: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("6529 API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []RepContribution `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding breakdown: %w", err)
	}

	return result.Data, nil
}

// RepContribution represents a single rep rating from one community member.
type RepContribution struct {
	Handle  string `json:"handle"`
	TDH     int64  `json:"tdh"`
	Rating  int64  `json:"rating"`
	Level   int    `json:"level"`
	Wallets []string `json:"wallets"`
}

// MinRepRequired returns the configured minimum rep threshold.
func (c *Checker) MinRepRequired() int64 {
	return c.minRep
}

// Category returns the configured rep category name.
func (c *Checker) Category() string {
	return c.category
}

// InvalidateCache removes a cached entry for a wallet.
func (c *Checker) InvalidateCache(walletOrHandle string) {
	c.mu.Lock()
	delete(c.cache, walletOrHandle)
	c.mu.Unlock()
}
