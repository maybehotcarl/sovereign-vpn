// Package wallet provides Ethereum wallet operations for SIWE authentication.
package wallet

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Wallet holds an Ethereum private key for signing SIWE messages.
type Wallet struct {
	privateKey *ecdsa.PrivateKey
	address    common.Address
}

// FromKeyFile loads a wallet from a hex-encoded private key file.
func FromKeyFile(path string) (*Wallet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading key file: %w", err)
	}
	hexKey := strings.TrimSpace(string(data))
	return FromHex(hexKey)
}

// FromHex creates a wallet from a hex-encoded private key.
func FromHex(hexKey string) (*Wallet, error) {
	hexKey = strings.TrimPrefix(hexKey, "0x")
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}
	return fromKey(key), nil
}

// Generate creates a new random wallet.
func Generate() (*Wallet, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}
	return fromKey(key), nil
}

func fromKey(key *ecdsa.PrivateKey) *Wallet {
	return &Wallet{
		privateKey: key,
		address:    crypto.PubkeyToAddress(key.PublicKey),
	}
}

// Address returns the wallet's Ethereum address.
func (w *Wallet) Address() common.Address {
	return w.address
}

// AddressHex returns the checksummed address string.
func (w *Wallet) AddressHex() string {
	return w.address.Hex()
}

// PrivateKeyHex returns the private key as a hex string (without 0x prefix).
func (w *Wallet) PrivateKeyHex() string {
	return hex.EncodeToString(crypto.FromECDSA(w.privateKey))
}

// SignMessage signs a message using ERC-191 personal_sign.
// This prepends "\x19Ethereum Signed Message:\n{len}" and hashes with Keccak256.
func (w *Wallet) SignMessage(message string) (string, error) {
	hash := signHash([]byte(message))
	sig, err := crypto.Sign(hash, w.privateKey)
	if err != nil {
		return "", fmt.Errorf("signing message: %w", err)
	}

	// Ethereum personal_sign uses v = 27 or 28
	sig[64] += 27

	return "0x" + hex.EncodeToString(sig), nil
}

// SaveKeyFile writes the private key to a file (hex-encoded).
func (w *Wallet) SaveKeyFile(path string) error {
	return os.WriteFile(path, []byte(w.PrivateKeyHex()+"\n"), 0600)
}

// signHash computes the Ethereum signed message hash (ERC-191).
func signHash(data []byte) []byte {
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data)
	return crypto.Keccak256([]byte(msg))
}
