package service

import (
	"testing"

	"benchmarking-platform/internal/security"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newEncryptionKeyServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.EncryptionKeyState{},
		&models.Agent{},
		&models.QuestionSetAgent{},
	))
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

func TestEncryptionKeyRotationServiceReencryptsConfigsWithCurrentKey(t *testing.T) {
	previousRaw := "12345678901234567890123456789012"
	currentRaw := "abcdefghijklmnopqrstuvwxyz123456"
	t.Setenv("ENCRYPTION_KEY", currentRaw)
	t.Setenv("ENCRYPTION_KEY_PREVIOUS", previousRaw)
	t.Setenv("ENCRYPTION_KEY_ROTATE_ON_START", "true")

	db := newEncryptionKeyServiceTestDB(t)
	keyService := NewEncryptionKeyService(db)

	t.Setenv("ENCRYPTION_KEY", previousRaw)
	_, err := keyService.ReconcileCurrentKey()
	require.NoError(t, err)
	t.Setenv("ENCRYPTION_KEY", currentRaw)

	previousKey, _, err := security.ParseEncryptionKey(previousRaw)
	require.NoError(t, err)
	currentKey, _, err := security.ParseEncryptionKey(currentRaw)
	require.NoError(t, err)

	agentID := uuid.New().String()
	questionSetID := uuid.New().String()
	agentConfig, err := security.EncryptWithKey(previousKey, []byte(`{"api_key":"legacy-agent"}`))
	require.NoError(t, err)
	overrideConfig, err := security.EncryptWithKey(previousKey, []byte(`{"api_key":"legacy-override"}`))
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		INSERT INTO agents (id, workspace_id, name, provider_type, config, enabled, position, max_concurrency, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, agentID, uuid.New().String(), "Legacy Agent", "openai", agentConfig, true, 0, 5).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO question_set_agents (question_set_id, agent_id, config, enabled, position, created_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, questionSetID, agentID, overrideConfig, true, 0).Error)

	result, err := NewEncryptionKeyRotationService(db).RotateOnStartIfConfigured()
	require.NoError(t, err)
	require.True(t, result.Requested)
	require.True(t, result.Executed)
	require.Equal(t, "completed", result.Status)
	require.EqualValues(t, 1, result.AgentsRotated)
	require.EqualValues(t, 1, result.QuestionSetAgentsRotated)

	var rotatedAgentConfig string
	require.NoError(t, db.Raw(`SELECT config FROM agents WHERE id = ?`, agentID).Scan(&rotatedAgentConfig).Error)
	agentPlaintext, err := security.DecryptWithKey(currentKey, rotatedAgentConfig)
	require.NoError(t, err)
	require.JSONEq(t, `{"api_key":"legacy-agent"}`, string(agentPlaintext))

	var rotatedOverrideConfig string
	require.NoError(t, db.Raw(`
		SELECT config
		FROM question_set_agents
		WHERE question_set_id = ? AND agent_id = ?
	`, questionSetID, agentID).Scan(&rotatedOverrideConfig).Error)
	overridePlaintext, err := security.DecryptWithKey(currentKey, rotatedOverrideConfig)
	require.NoError(t, err)
	require.JSONEq(t, `{"api_key":"legacy-override"}`, string(overridePlaintext))

	var state models.EncryptionKeyState
	require.NoError(t, db.First(&state, "id = ?", models.EncryptionKeyStatePrimaryID).Error)
	require.Equal(t, security.KeyFingerprint(currentKey), state.ActiveFingerprint)
	require.Equal(t, "match", state.LastSeenStatus)
}

func TestEncryptionKeyRotationServiceRequiresPreviousKeyWhenRequested(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("ENCRYPTION_KEY_ROTATE_ON_START", "true")

	db := newEncryptionKeyServiceTestDB(t)

	_, err := NewEncryptionKeyRotationService(db).RotateOnStartIfConfigured()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ENCRYPTION_KEY_PREVIOUS must be set")
}
