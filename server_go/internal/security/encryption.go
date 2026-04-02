package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

type EncryptionKeyRuntimeStatus struct {
	Status          string
	Source          string
	Summary         string
	Loaded          bool
	UsedFallback    bool
	Format          string
	CharLength      int
	ParsedBytes     int
	ValidationError string
}

var (
	encryptionKeyRuntimeStatus EncryptionKeyRuntimeStatus
	encryptionKeyRuntimeMu     sync.RWMutex
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

func SetEncryptionKeyRuntimeStatus(status EncryptionKeyRuntimeStatus) {
	encryptionKeyRuntimeMu.Lock()
	defer encryptionKeyRuntimeMu.Unlock()
	encryptionKeyRuntimeStatus = status
}

func GetEncryptionKeyRuntimeStatus() EncryptionKeyRuntimeStatus {
	encryptionKeyRuntimeMu.RLock()
	defer encryptionKeyRuntimeMu.RUnlock()
	return encryptionKeyRuntimeStatus
}

func KeyFingerprint(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])
}

// EncryptWithKey encrypts bytes using AES-GCM and the provided parsed key.
func EncryptWithKey(key []byte, plaintext []byte) (string, error) {
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

// Encrypt string using AES-GCM
func Encrypt(plaintext []byte) (string, error) {
	key, _, err := ParseEncryptionKey(os.Getenv("ENCRYPTION_KEY"))
	if err != nil {
		return "", err
	}

	return EncryptWithKey(key, plaintext)
}

// DecryptWithKey decrypts a base64 AES-GCM ciphertext with the provided parsed key.
func DecryptWithKey(key []byte, cryptoText string) ([]byte, error) {
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

// Decrypt string using AES-GCM
func Decrypt(cryptoText string) ([]byte, error) {
	key, _, err := ParseEncryptionKey(os.Getenv("ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}

	return DecryptWithKey(key, cryptoText)
}
