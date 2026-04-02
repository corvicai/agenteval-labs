package service

import (
	"testing"

	"benchmarking-platform/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newEncryptionKeyServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.EncryptionKeyState{}))
	return db
}

func TestEncryptionKeyServiceBootstrapState(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "1234567890abcdef")

	db := newEncryptionKeyServiceTestDB(t)
	service := NewEncryptionKeyService(db)

	health, err := service.ReconcileCurrentKey()
	require.NoError(t, err)
	require.True(t, health.StatePresent)
	require.Equal(t, "initialized", health.StateStatus)
	require.NotEmpty(t, health.ObservedFingerprintPrefix)
	require.Equal(t, health.ObservedFingerprintPrefix, health.StoredFingerprintPrefix)

	var state models.EncryptionKeyState
	require.NoError(t, db.First(&state, "id = ?", models.EncryptionKeyStatePrimaryID).Error)
	require.Equal(t, EncryptionCipherVersion, state.CipherVersion)
	require.NotEmpty(t, state.SentinelCiphertext)
}

func TestEncryptionKeyServiceRecognizesMatchingKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "1234567890abcdef")

	db := newEncryptionKeyServiceTestDB(t)
	service := NewEncryptionKeyService(db)

	_, err := service.ReconcileCurrentKey()
	require.NoError(t, err)

	health, err := service.ReconcileCurrentKey()
	require.NoError(t, err)
	require.Equal(t, "match", health.StateStatus)
	require.Equal(t, health.ObservedFingerprintPrefix, health.StoredFingerprintPrefix)
}

func TestEncryptionKeyServiceDetectsMismatch(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "1234567890abcdef")

	db := newEncryptionKeyServiceTestDB(t)
	service := NewEncryptionKeyService(db)

	_, err := service.ReconcileCurrentKey()
	require.NoError(t, err)

	t.Setenv("ENCRYPTION_KEY", "abcdef1234567890")

	health, err := service.ReconcileCurrentKey()
	require.NoError(t, err)
	require.Equal(t, "mismatch", health.StateStatus)
	require.NotEqual(t, health.ObservedFingerprintPrefix, health.StoredFingerprintPrefix)

	var state models.EncryptionKeyState
	require.NoError(t, db.First(&state, "id = ?", models.EncryptionKeyStatePrimaryID).Error)
	require.NotEmpty(t, state.LastMismatchFingerprint)
	require.NotNil(t, state.LastMismatchAt)
}
