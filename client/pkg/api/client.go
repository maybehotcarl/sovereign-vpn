// Package api provides an HTTP client for the Sovereign VPN gateway.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client communicates with the Sovereign VPN gateway.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a gateway API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ChallengeResponse is returned by POST /auth/challenge.
type ChallengeResponse struct {
	Message string `json:"message"`
	Nonce   string `json:"nonce"`
}

// VerifyResponse is returned by POST /auth/verify.
type VerifyResponse struct {
	Address   string `json:"address"`
	Tier      string `json:"tier"`
	ExpiresAt string `json:"expires_at"`
}

// ConnectResponse is returned by POST /vpn/connect.
type ConnectResponse struct {
	ServerPublicKey string `json:"server_public_key"`
	ServerEndpoint  string `json:"server_endpoint"`
	ClientAddress   string `json:"client_address"`
	DNS             string `json:"dns"`
	AllowedIPs      string `json:"allowed_ips"`
	ExpiresAt       string `json:"expires_at"`
	Tier            string `json:"tier"`
}

// StatusResponse is returned by GET /vpn/status.
type StatusResponse struct {
	Connected bool   `json:"connected"`
	Tier      string `json:"tier,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// ErrorResponse is the standard error format.
type ErrorResponse struct {
	Error string `json:"error"`
}

// GetChallenge requests a SIWE challenge message for the given address.
func (c *Client) GetChallenge(address string) (*ChallengeResponse, error) {
	body, _ := json.Marshal(map[string]string{"address": address})
	resp, err := c.post("/auth/challenge", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result ChallengeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding challenge response: %w", err)
	}
	return &result, nil
}

// Verify submits a signed SIWE message to create a session.
func (c *Client) Verify(message, signature string) (*VerifyResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"message":   message,
		"signature": signature,
	})
	resp, err := c.post("/auth/verify", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding verify response: %w", err)
	}
	return &result, nil
}

// Connect requests a VPN connection with the given session token and WireGuard public key.
func (c *Client) Connect(sessionToken, publicKey string) (*ConnectResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"session_token": sessionToken,
		"public_key":    publicKey,
	})
	resp, err := c.post("/vpn/connect", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result ConnectResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding connect response: %w", err)
	}
	return &result, nil
}

// Disconnect terminates a VPN connection.
func (c *Client) Disconnect(sessionToken, publicKey string) error {
	body, _ := json.Marshal(map[string]string{
		"session_token": sessionToken,
		"public_key":    publicKey,
	})
	resp, err := c.post("/vpn/disconnect", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}
	return nil
}

// Status checks the VPN connection status.
func (c *Client) Status(sessionToken string) (*StatusResponse, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/vpn/status?session_token=" + sessionToken)
	if err != nil {
		return nil, fmt.Errorf("status request: %w", err)
	}
	defer resp.Body.Close()

	var result StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding status response: %w", err)
	}
	return &result, nil
}

// Health checks gateway health.
func (c *Client) Health() (map[string]any, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return nil, fmt.Errorf("health request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding health response: %w", err)
	}
	return result, nil
}

// NodesResponse is returned by GET /nodes.
type NodesResponse struct {
	Nodes []NodeInfo `json:"nodes"`
	Count int        `json:"count"`
}

// NodeInfo represents a VPN node from the registry.
type NodeInfo struct {
	Operator   string `json:"operator"`
	Endpoint   string `json:"endpoint"`
	WgPubKey   string `json:"wg_pub_key"`
	Region     string `json:"region"`
	Reputation uint64 `json:"reputation"`
	Active     bool   `json:"active"`
}

// ListNodes fetches all active VPN nodes from the gateway.
func (c *Client) ListNodes() (*NodesResponse, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/nodes")
	if err != nil {
		return nil, fmt.Errorf("nodes request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result NodesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding nodes response: %w", err)
	}
	return &result, nil
}

// ListNodesByRegion fetches active VPN nodes in a specific region.
func (c *Client) ListNodesByRegion(region string) (*NodesResponse, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/nodes/region?region=" + region)
	if err != nil {
		return nil, fmt.Errorf("nodes request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result NodesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding nodes response: %w", err)
	}
	return &result, nil
}

func (c *Client) post(path string, body []byte) (*http.Response, error) {
	resp, err := c.httpClient.Post(
		c.baseURL+path,
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	return resp, nil
}

func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return fmt.Errorf("gateway error (%d): %s", resp.StatusCode, errResp.Error)
	}
	return fmt.Errorf("gateway error (%d): %s", resp.StatusCode, string(body))
}
