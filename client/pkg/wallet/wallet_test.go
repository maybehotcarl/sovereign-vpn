package wallet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestGenerate(t *testing.T) {
	w, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Address should be valid
	if w.Address() == (common.Address{}) {
		t.Error("generated address should not be zero")
	}

	// AddressHex should start with 0x
	if !strings.HasPrefix(w.AddressHex(), "0x") {
		t.Errorf("address should start with 0x, got %s", w.AddressHex())
	}

	// Private key should be 64 hex chars
	if len(w.PrivateKeyHex()) != 64 {
		t.Errorf("expected 64 hex chars for private key, got %d", len(w.PrivateKeyHex()))
	}
}

func TestFromHex(t *testing.T) {
	// Generate a known key
	key, _ := crypto.GenerateKey()
	hexKey := strings.TrimPrefix(crypto.PubkeyToAddress(key.PublicKey).Hex(), "0x")
	_ = hexKey

	privHex := crypto.FromECDSA(key)
	w, err := FromHex(common.Bytes2Hex(privHex))
	if err != nil {
		t.Fatalf("FromHex: %v", err)
	}

	expected := crypto.PubkeyToAddress(key.PublicKey)
	if w.Address() != expected {
		t.Errorf("address mismatch: got %s, want %s", w.AddressHex(), expected.Hex())
	}
}

func TestFromHexWith0xPrefix(t *testing.T) {
	key, _ := crypto.GenerateKey()
	hexKey := "0x" + common.Bytes2Hex(crypto.FromECDSA(key))

	w, err := FromHex(hexKey)
	if err != nil {
		t.Fatalf("FromHex with 0x prefix: %v", err)
	}

	if w.Address() == (common.Address{}) {
		t.Error("address should not be zero")
	}
}

func TestFromHexInvalid(t *testing.T) {
	_, err := FromHex("not-a-valid-key")
	if err == nil {
		t.Error("expected error for invalid hex key")
	}
}

func TestSaveAndLoadKeyFile(t *testing.T) {
	w, _ := Generate()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.key")

	if err := w.SaveKeyFile(path); err != nil {
		t.Fatalf("SaveKeyFile: %v", err)
	}

	// Check file permissions
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("key file should be 0600, got %o", info.Mode().Perm())
	}

	// Load it back
	w2, err := FromKeyFile(path)
	if err != nil {
		t.Fatalf("FromKeyFile: %v", err)
	}

	if w.Address() != w2.Address() {
		t.Errorf("addresses should match: %s vs %s", w.AddressHex(), w2.AddressHex())
	}
}

func TestFromKeyFileNotFound(t *testing.T) {
	_, err := FromKeyFile("/nonexistent/path/key.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestSignMessage(t *testing.T) {
	w, _ := Generate()

	sig, err := w.SignMessage("test message")
	if err != nil {
		t.Fatalf("SignMessage: %v", err)
	}

	// Signature should be hex-encoded with 0x prefix
	if !strings.HasPrefix(sig, "0x") {
		t.Errorf("signature should start with 0x, got %s", sig[:4])
	}

	// 65 bytes = 130 hex chars + "0x" prefix = 132 chars
	if len(sig) != 132 {
		t.Errorf("expected 132 chars (0x + 130 hex), got %d", len(sig))
	}
}

func TestSignMessageRecoverable(t *testing.T) {
	w, _ := Generate()
	message := "hello sovereign vpn"

	sig, err := w.SignMessage(message)
	if err != nil {
		t.Fatal(err)
	}

	// Decode signature
	sigBytes := common.FromHex(sig)
	if len(sigBytes) != 65 {
		t.Fatalf("expected 65 bytes, got %d", len(sigBytes))
	}

	// Adjust v back from Ethereum format
	sigBytes[64] -= 27

	// Recover the public key
	hash := signHash([]byte(message))
	pubKey, err := crypto.Ecrecover(hash, sigBytes)
	if err != nil {
		t.Fatalf("Ecrecover: %v", err)
	}

	recovered, err := crypto.UnmarshalPubkey(pubKey)
	if err != nil {
		t.Fatalf("UnmarshalPubkey: %v", err)
	}

	recoveredAddr := crypto.PubkeyToAddress(*recovered)
	if recoveredAddr != w.Address() {
		t.Errorf("recovered address %s != wallet address %s", recoveredAddr.Hex(), w.AddressHex())
	}
}
