package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	plaintext := []byte(`{"password":"secret123"}`)

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	ciphertext, _ := Encrypt(key1, []byte("secret"))
	_, err := Decrypt(key2, ciphertext)
	if err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}

func TestParseKeyHex(t *testing.T) {
	keyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	key, err := ParseKeyHex(keyHex)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key length %d, want 32", len(key))
	}
}

func TestParseKeyHex_Invalid(t *testing.T) {
	_, err := ParseKeyHex("not-hex")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
	_, err = ParseKeyHex("0123")
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestGenerateKeyHex(t *testing.T) {
	keyHex, err := GenerateKeyHex()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(keyHex) != 64 {
		t.Errorf("hex length %d, want 64", len(keyHex))
	}
	_, err = hex.DecodeString(keyHex)
	if err != nil {
		t.Errorf("invalid hex: %v", err)
	}
}

func TestLoadKey_EnvVar(t *testing.T) {
	keyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	t.Setenv("MANTISOPS_ENCRYPTION_KEY", keyHex)
	key, err := LoadKey("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key length %d", len(key))
	}
}

func TestLoadKey_NoConfig(t *testing.T) {
	t.Setenv("MANTISOPS_ENCRYPTION_KEY", "")
	_, err := LoadKey("")
	if err == nil {
		t.Error("expected error when no key configured")
	}
}

func TestEnsureKey_AutoGenerate(t *testing.T) {
	t.Setenv("MANTISOPS_ENCRYPTION_KEY", "")
	tmpFile := filepath.Join(t.TempDir(), "test-config.yaml")
	os.WriteFile(tmpFile, []byte("# test config\n"), 0644)

	key, err := EnsureKey("", tmpFile)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key length %d", len(key))
	}
	// Verify key was written to file
	content, _ := os.ReadFile(tmpFile)
	if len(content) < 80 {
		t.Error("key not written to config file")
	}
}

func TestEnsureKey_ReadOnlyFile(t *testing.T) {
	t.Setenv("MANTISOPS_ENCRYPTION_KEY", "")
	_, err := EnsureKey("", "/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error when config file is not writable")
	}
}
