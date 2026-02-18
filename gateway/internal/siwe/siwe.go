package siwe

import (
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// Challenge represents a SIWE challenge issued to a client.
type Challenge struct {
	Domain    string    `json:"domain"`
	Address   string    `json:"address,omitempty"` // Empty in challenge, filled by client
	URI       string    `json:"uri"`
	Version   string    `json:"version"`
	ChainID   int       `json:"chain_id"`
	Nonce     string    `json:"nonce"`
	IssuedAt  time.Time `json:"issued_at"`
	Statement string    `json:"statement,omitempty"`
}

// SignedMessage represents a client's signed SIWE response.
type SignedMessage struct {
	Message   string `json:"message"`   // The full EIP-4361 message string that was signed
	Signature string `json:"signature"` // Hex-encoded signature (0x-prefixed, 65 bytes)
}

// VerifiedAuth is the result of a successful SIWE verification.
type VerifiedAuth struct {
	Address common.Address `json:"address"` // The recovered wallet address
}

// Service handles SIWE challenge generation and verification.
type Service struct {
	domain     string
	uri        string
	nonceStore *NonceStore
	chainID    int
}

// NewService creates a SIWE service.
func NewService(domain, uri string, challengeTTL time.Duration, nonceLength int) *Service {
	return &Service{
		domain:     domain,
		uri:        uri,
		nonceStore: NewNonceStore(challengeTTL),
		chainID:    1, // Ethereum mainnet; Sepolia = 11155111
	}
}

// SetChainID sets the expected chain ID (1 = mainnet, 11155111 = Sepolia).
func (s *Service) SetChainID(chainID int) {
	s.chainID = chainID
}

// NewChallenge generates a SIWE challenge for the client to sign.
func (s *Service) NewChallenge(nonceLength int) (*Challenge, error) {
	nonce, err := s.nonceStore.Generate(nonceLength)
	if err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	return &Challenge{
		Domain:    s.domain,
		URI:       s.uri,
		Version:   "1",
		ChainID:   s.chainID,
		Nonce:     nonce,
		IssuedAt:  time.Now().UTC(),
		Statement: "Sign in to Sovereign VPN with your Ethereum account.",
	}, nil
}

// FormatMessage creates the EIP-4361 message string for a challenge + address.
// This is what the client should sign with personal_sign.
func FormatMessage(c *Challenge, address string) string {
	// EIP-4361 format:
	// ${domain} wants you to sign in with your Ethereum account:
	// ${address}
	//
	// ${statement}
	//
	// URI: ${uri}
	// Version: ${version}
	// Chain ID: ${chain-id}
	// Nonce: ${nonce}
	// Issued At: ${issued-at}
	var b strings.Builder
	fmt.Fprintf(&b, "%s wants you to sign in with your Ethereum account:\n", c.Domain)
	fmt.Fprintf(&b, "%s\n", address)
	fmt.Fprintf(&b, "\n")
	if c.Statement != "" {
		fmt.Fprintf(&b, "%s\n", c.Statement)
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "URI: %s\n", c.URI)
	fmt.Fprintf(&b, "Version: %s\n", c.Version)
	fmt.Fprintf(&b, "Chain ID: %d\n", c.ChainID)
	fmt.Fprintf(&b, "Nonce: %s\n", c.Nonce)
	fmt.Fprintf(&b, "Issued At: %s", c.IssuedAt.Format(time.RFC3339))
	return b.String()
}

// Verify checks a signed SIWE message:
// 1. Recovers the signer address from the signature
// 2. Parses the message to extract the nonce
// 3. Validates the nonce (single-use, not expired)
// 4. Validates the domain and URI match
// Returns the verified wallet address.
func (s *Service) Verify(signed *SignedMessage) (*VerifiedAuth, error) {
	// Decode the signature
	sigBytes, err := hexutil.Decode(signed.Signature)
	if err != nil {
		return nil, fmt.Errorf("decoding signature: %w", err)
	}
	if len(sigBytes) != 65 {
		return nil, fmt.Errorf("signature must be 65 bytes, got %d", len(sigBytes))
	}

	// Ethereum personal_sign uses ERC-191:
	// keccak256("\x19Ethereum Signed Message:\n" + len(message) + message)
	msgHash := signHash([]byte(signed.Message))

	// Fix recovery ID: MetaMask uses 27/28, go-ethereum expects 0/1
	if sigBytes[64] >= 27 {
		sigBytes[64] -= 27
	}

	// Recover public key from signature
	pubKey, err := crypto.SigToPub(msgHash, sigBytes)
	if err != nil {
		return nil, fmt.Errorf("recovering public key: %w", err)
	}

	// Derive address from public key
	recoveredAddr := crypto.PubkeyToAddress(*pubKey)

	// Parse the message to extract fields
	parsed, err := parseMessage(signed.Message)
	if err != nil {
		return nil, fmt.Errorf("parsing SIWE message: %w", err)
	}

	// Verify the recovered address matches the address in the message
	if !strings.EqualFold(recoveredAddr.Hex(), parsed.address) {
		return nil, fmt.Errorf("recovered address %s does not match message address %s",
			recoveredAddr.Hex(), parsed.address)
	}

	// Verify domain
	if parsed.domain != s.domain {
		return nil, fmt.Errorf("domain mismatch: got %q, expected %q", parsed.domain, s.domain)
	}

	// Consume nonce (single-use)
	if !s.nonceStore.Consume(parsed.nonce) {
		return nil, fmt.Errorf("invalid or expired nonce")
	}

	return &VerifiedAuth{
		Address: recoveredAddr,
	}, nil
}

// signHash computes the Ethereum signed message hash (ERC-191).
func signHash(data []byte) []byte {
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data)
	return crypto.Keccak256([]byte(msg))
}

// parsedMessage holds fields extracted from a SIWE message string.
type parsedMessage struct {
	domain  string
	address string
	nonce   string
}

// parseMessage extracts key fields from an EIP-4361 message string.
func parseMessage(msg string) (*parsedMessage, error) {
	lines := strings.Split(msg, "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("message too short")
	}

	parsed := &parsedMessage{}

	// Line 0: "{domain} wants you to sign in with your Ethereum account:"
	domainLine := lines[0]
	domainEnd := strings.Index(domainLine, " wants you to sign in")
	if domainEnd < 0 {
		return nil, fmt.Errorf("invalid domain line: %q", domainLine)
	}
	parsed.domain = domainLine[:domainEnd]

	// Line 1: address (0x...)
	parsed.address = strings.TrimSpace(lines[1])
	if !common.IsHexAddress(parsed.address) {
		return nil, fmt.Errorf("invalid address: %q", parsed.address)
	}

	// Find nonce line
	for _, line := range lines {
		if strings.HasPrefix(line, "Nonce: ") {
			parsed.nonce = strings.TrimPrefix(line, "Nonce: ")
			break
		}
	}
	if parsed.nonce == "" {
		return nil, fmt.Errorf("nonce not found in message")
	}

	return parsed, nil
}
