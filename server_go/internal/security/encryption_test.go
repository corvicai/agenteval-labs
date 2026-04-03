package security

import (
	"encoding/hex"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEncryptionKeyRaw(t *testing.T) {
	raw := "12345678901234567890123456789012"

	key, format, err := ParseEncryptionKey(raw)
	require.NoError(t, err)
	require.Equal(t, "raw", format)
	require.Equal(t, []byte(raw), key)
}

func TestParseEncryptionKeyHex(t *testing.T) {
	rawKey := []byte("12345678901234567890123456789012")
	hexKey := hex.EncodeToString(rawKey)

	key, format, err := ParseEncryptionKey(hexKey)
	require.NoError(t, err)
	require.Equal(t, "hex", format)
	require.Equal(t, rawKey, key)
}

func TestEncryptDecryptWithHexEncodedKey(t *testing.T) {
	rawKey := []byte("12345678901234567890123456789012")
	hexKey := hex.EncodeToString(rawKey)
	t.Setenv("ENCRYPTION_KEY", hexKey)

	encrypted, err := Encrypt([]byte(`{"ok":true}`))
	require.NoError(t, err)

	decrypted, err := Decrypt(encrypted)
	require.NoError(t, err)
	require.Equal(t, `{"ok":true}`, string(decrypted))
}

func TestDecryptFallsBackToPreviousKey(t *testing.T) {
	previousKey := []byte("12345678901234567890123456789012")
	currentKey := []byte("abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("ENCRYPTION_KEY", string(currentKey))
	t.Setenv("ENCRYPTION_KEY_PREVIOUS", string(previousKey))

	encrypted, err := EncryptWithKey(previousKey, []byte(`{"legacy":true}`))
	require.NoError(t, err)

	decrypted, err := Decrypt(encrypted)
	require.NoError(t, err)
	require.Equal(t, `{"legacy":true}`, string(decrypted))
}

func TestParseEncryptionKeyInvalidLength(t *testing.T) {
	_, _, err := ParseEncryptionKey("1234567890123456789012345678901234567890123456789012345678901234zz")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid ENCRYPTION_KEY length")
}

func TestEncryptWithoutKey(t *testing.T) {
	original, hadOriginal := os.LookupEnv("ENCRYPTION_KEY")
	t.Cleanup(func() {
		if hadOriginal {
			require.NoError(t, os.Setenv("ENCRYPTION_KEY", original))
			return
		}
		require.NoError(t, os.Unsetenv("ENCRYPTION_KEY"))
	})
	require.NoError(t, os.Unsetenv("ENCRYPTION_KEY"))

	_, err := Encrypt([]byte("hello"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "ENCRYPTION_KEY environment variable not set")
}
