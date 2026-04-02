package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

// ParseEncryptionKey accepts raw AES keys (16/24/32 chars) and hex-encoded keys
// (32/48/64 chars that decode to 16/24/32 bytes).
func ParseEncryptionKey(raw string) ([]byte, string, error) {
	if raw == "" {
		return nil, "", errors.New("ENCRYPTION_KEY environment variable not set")
	}

	if len(raw) == 16 || len(raw) == 24 || len(raw) == 32 {
		return []byte(raw), "raw", nil
	}

	switch len(raw) {
	case 32, 48, 64:
		decoded, err := hex.DecodeString(raw)
		if err == nil {
			switch len(decoded) {
			case 16, 24, 32:
				return decoded, "hex", nil
			}
		}
	}

	return nil, "", fmt.Errorf("invalid ENCRYPTION_KEY length: %d bytes (must be 16, 24, or 32)", len(raw))
}

// Encrypt string using AES-GCM
func Encrypt(plaintext []byte) (string, error) {
	key, _, err := ParseEncryptionKey(os.Getenv("ENCRYPTION_KEY"))
	if err != nil {
		return "", err
	}

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

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt string using AES-GCM
func Decrypt(cryptoText string) ([]byte, error) {
	key, _, err := ParseEncryptionKey(os.Getenv("ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(cryptoText)
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
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
