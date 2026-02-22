// Package zkverify provides an HTTP client for the 6529 ZK API service.
// It forwards Groth16 proofs for server-side verification and nullifier tracking.
package zkverify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client calls the standalone 6529 ZK API for proof verification.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a ZK API client. The apiKey is optional (verify endpoint is public).
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// ProofPayload is sent to POST /api/zk for verification.
type ProofPayload struct {
	ProofType     string   `json:"proofType"`
	Proof         any      `json:"proof"`
	PublicSignals []string `json:"publicSignals"`
}

// VerifyResult is the response from the ZK API verify endpoint.
type VerifyResult struct {
	Success bool   `json:"success"`
	Valid   bool   `json:"valid"`
	Reason  string `json:"reason,omitempty"`
	Data    *struct {
		PublicSignals []string `json:"publicSignals"`
	} `json:"data,omitempty"`
}

// VerifyProof forwards a proof to the ZK API for verification.
// The ZK API handles: Groth16 verification, merkle root freshness, nullifier tracking.
func (c *Client) VerifyProof(ctx context.Context, payload ProofPayload) (*VerifyResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling proof payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/zk", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling ZK API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("ZK API error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return nil, fmt.Errorf("ZK API returned status %d", resp.StatusCode)
	}

	var result VerifyResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// HealthResult is the response from GET /api/health.
type HealthResult struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// Health checks if the ZK API is reachable and healthy.
func (c *Client) Health(ctx context.Context) (*HealthResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/health", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling ZK API health: %w", err)
	}
	defer resp.Body.Close()

	var result HealthResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding health response: %w", err)
	}
	return &result, nil
}
