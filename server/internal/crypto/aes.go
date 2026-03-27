package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
)

func Encrypt(key, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func Decrypt(key []byte, ciphertext string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}

func ParseKeyHex(keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

func GenerateKeyHex() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

// LoadKey loads encryption key: env var > config file. Returns error if neither configured.
func LoadKey(configKeyHex string) ([]byte, error) {
	if envKey := os.Getenv("OPSBOARD_ENCRYPTION_KEY"); envKey != "" {
		return ParseKeyHex(envKey)
	}
	if configKeyHex != "" {
		return ParseKeyHex(configKeyHex)
	}
	return nil, fmt.Errorf("encryption_key not configured: set OPSBOARD_ENCRYPTION_KEY env var or encryption_key in server.yaml")
}

// EnsureKey loads key via LoadKey; if missing, auto-generates and writes back to configPath.
// If write-back fails, returns error (caller should log.Fatal -- no persistent key = credentials unrecoverable).
func EnsureKey(configKeyHex, configPath string) ([]byte, error) {
	key, err := LoadKey(configKeyHex)
	if err == nil {
		return key, nil
	}
	keyHex, genErr := GenerateKeyHex()
	if genErr != nil {
		return nil, genErr
	}
	f, writeErr := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0644)
	if writeErr != nil {
		return nil, fmt.Errorf("auto-generated key %s but cannot write to %s: %w. Set OPSBOARD_ENCRYPTION_KEY env var manually", keyHex, configPath, writeErr)
	}
	defer f.Close()
	if _, writeErr = fmt.Fprintf(f, "\nencryption_key: \"%s\"\n", keyHex); writeErr != nil {
		return nil, fmt.Errorf("failed to write key to %s: %w", configPath, writeErr)
	}
	log.Printf("[crypto] encryption_key auto-generated and saved to %s", configPath)
	return ParseKeyHex(keyHex)
}
