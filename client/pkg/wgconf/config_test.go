package wgconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Keys should be base64-encoded 32-byte values (44 chars with padding)
	if len(kp.PrivateKey) != 44 {
		t.Errorf("private key length should be 44, got %d", len(kp.PrivateKey))
	}
	if len(kp.PublicKey) != 44 {
		t.Errorf("public key length should be 44, got %d", len(kp.PublicKey))
	}

	// Keys should be different
	if kp.PrivateKey == kp.PublicKey {
		t.Error("private and public keys should differ")
	}
}

func TestGenerateKeyPairUniqueness(t *testing.T) {
	kp1, _ := GenerateKeyPair()
	kp2, _ := GenerateKeyPair()

	if kp1.PrivateKey == kp2.PrivateKey {
		t.Error("two generated key pairs should have different private keys")
	}
	if kp1.PublicKey == kp2.PublicKey {
		t.Error("two generated key pairs should have different public keys")
	}
}

func TestConfigString(t *testing.T) {
	cfg := &Config{
		PrivateKey:      "cGF5bG9hZDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIz",
		ClientAddress:   "10.8.0.2/24",
		DNS:             "1.1.1.1",
		ServerPublicKey: "c2VydmVycHVia2V5MTIzNDU2Nzg5MDEyMzQ1Njc4",
		ServerEndpoint:  "vpn.example.com:51820",
		AllowedIPs:      "0.0.0.0/0, ::/0",
	}

	s := cfg.String()

	if !strings.Contains(s, "[Interface]") {
		t.Error("config should contain [Interface] section")
	}
	if !strings.Contains(s, "[Peer]") {
		t.Error("config should contain [Peer] section")
	}
	if !strings.Contains(s, "PrivateKey = cGF5bG9hZDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIz") {
		t.Error("config should contain PrivateKey")
	}
	if !strings.Contains(s, "Address = 10.8.0.2/24") {
		t.Error("config should contain Address")
	}
	if !strings.Contains(s, "DNS = 1.1.1.1") {
		t.Error("config should contain DNS")
	}
	if !strings.Contains(s, "PublicKey = c2VydmVycHVia2V5MTIzNDU2Nzg5MDEyMzQ1Njc4") {
		t.Error("config should contain server PublicKey")
	}
	if !strings.Contains(s, "Endpoint = vpn.example.com:51820") {
		t.Error("config should contain Endpoint")
	}
	if !strings.Contains(s, "AllowedIPs = 0.0.0.0/0, ::/0") {
		t.Error("config should contain AllowedIPs")
	}
	if !strings.Contains(s, "PersistentKeepalive = 25") {
		t.Error("config should contain PersistentKeepalive")
	}
}

func TestConfigWriteFile(t *testing.T) {
	cfg := &Config{
		PrivateKey:      "testprivkey",
		ClientAddress:   "10.8.0.5/24",
		DNS:             "8.8.8.8",
		ServerPublicKey: "testserverpub",
		ServerEndpoint:  "1.2.3.4:51820",
		AllowedIPs:      "0.0.0.0/0",
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "wg0.conf")

	if err := cfg.WriteFile(path); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Check file exists and has correct permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("config file should be 0600, got %o", info.Mode().Perm())
	}

	// Read it back
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "testprivkey") {
		t.Error("written file should contain the private key")
	}
}
