package service

import (
	"fmt"
	"os"
	"strings"

	"benchmarking-platform/internal/security"

	"gorm.io/gorm"
)

const (
	encryptionRotationLockKey = int64(732451901)
)

type EncryptionKeyRotationResult struct {
	Requested                 bool
	Executed                  bool
	LockBusy                  bool
	Status                    string
	CurrentFingerprintPrefix  string
	PreviousFingerprintPrefix string
	AgentsRotated             int64
	QuestionSetAgentsRotated  int64
}

type EncryptionKeyRotationService struct {
	db *gorm.DB
}

type rotationAgentRow struct {
	ID     string
	Config string
}

type rotationQuestionSetAgentRow struct {
	QuestionSetID string
	AgentID       string
	Config        string
}

func NewEncryptionKeyRotationService(db *gorm.DB) *EncryptionKeyRotationService {
	return &EncryptionKeyRotationService{db: db}
}

func (s *EncryptionKeyRotationService) RotateOnStartIfConfigured() (EncryptionKeyRotationResult, error) {
	result := EncryptionKeyRotationResult{
		Requested: parseEncryptionRotationFlag(os.Getenv("ENCRYPTION_KEY_ROTATE_ON_START")),
		Status:    "not_requested",
	}
	if !result.Requested {
		return result, nil
	}
	if s.db == nil {
		return result, fmt.Errorf("database unavailable")
	}

	keyService := NewEncryptionKeyService(s.db)
	current, err := keyService.ObserveCurrentKey()
	if err != nil {
		return result, err
	}
	result.CurrentFingerprintPrefix = shortFingerprint(current.Fingerprint)

	previous, err := keyService.ObservePreviousKey()
	if err != nil {
		return result, err
	}
	if previous == nil {
		return result, fmt.Errorf("ENCRYPTION_KEY_PREVIOUS must be set when ENCRYPTION_KEY_ROTATE_ON_START is enabled")
	}
	result.PreviousFingerprintPrefix = shortFingerprint(previous.Fingerprint)

	if current.Fingerprint == previous.Fingerprint {
		result.Status = "skipped_same_key"
		return result, nil
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		acquired, lockErr := tryAcquireEncryptionRotationLock(tx)
		if lockErr != nil {
			return lockErr
		}
		if !acquired {
			result.Status = "lock_busy"
			result.LockBusy = true
			return nil
		}

		result.Executed = true
		result.Status = "running"

		agentsRotated, rotateAgentsErr := rotateAgentConfigs(tx, current, previous)
		if rotateAgentsErr != nil {
			return rotateAgentsErr
		}

		questionSetAgentsRotated, rotateQuestionSetAgentsErr := rotateQuestionSetAgentConfigs(tx, current, previous)
		if rotateQuestionSetAgentsErr != nil {
			return rotateQuestionSetAgentsErr
		}

		if promoteErr := NewEncryptionKeyService(tx).PromoteCurrentKeyState(current); promoteErr != nil {
			return promoteErr
		}

		result.AgentsRotated = agentsRotated
		result.QuestionSetAgentsRotated = questionSetAgentsRotated
		result.Status = "completed"
		return nil
	})
	if err != nil {
		result.Status = "failed"
		return result, err
	}
	if result.LockBusy {
		return result, nil
	}

	if _, err := keyService.ReconcileCurrentKey(); err != nil {
		result.Status = "failed"
		return result, err
	}

	return result, nil
}

func tryAcquireEncryptionRotationLock(tx *gorm.DB) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("database transaction unavailable")
	}
	if tx.Dialector.Name() != "postgres" {
		return true, nil
	}

	var acquired bool
	if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", encryptionRotationLockKey).Scan(&acquired).Error; err != nil {
		return false, err
	}
	return acquired, nil
}

func rotateAgentConfigs(tx *gorm.DB, current *CurrentEncryptionKey, previous *CurrentEncryptionKey) (int64, error) {
	var rows []rotationAgentRow
	if err := tx.Raw(`
		SELECT id, COALESCE(config, '') AS config
		FROM agents
		ORDER BY created_at ASC
	`).Scan(&rows).Error; err != nil {
		return 0, err
	}

	var rotated int64
	for _, row := range rows {
		updatedConfig, changed, err := rotateEncryptedConfigValue(row.Config, current, previous)
		if err != nil {
			return rotated, fmt.Errorf("agents.%s: %w", row.ID, err)
		}
		if !changed {
			continue
		}
		if err := tx.Exec(`UPDATE agents SET config = ? WHERE id = ?`, updatedConfig, row.ID).Error; err != nil {
			return rotated, err
		}
		rotated++
	}

	return rotated, nil
}

func rotateQuestionSetAgentConfigs(tx *gorm.DB, current *CurrentEncryptionKey, previous *CurrentEncryptionKey) (int64, error) {
	var rows []rotationQuestionSetAgentRow
	if err := tx.Raw(`
		SELECT question_set_id, agent_id, COALESCE(config, '') AS config
		FROM question_set_agents
		ORDER BY created_at ASC
	`).Scan(&rows).Error; err != nil {
		return 0, err
	}

	var rotated int64
	for _, row := range rows {
		updatedConfig, changed, err := rotateEncryptedConfigValue(row.Config, current, previous)
		if err != nil {
			return rotated, fmt.Errorf("question_set_agents.%s.%s: %w", row.QuestionSetID, row.AgentID, err)
		}
		if !changed {
			continue
		}
		if err := tx.Exec(`
			UPDATE question_set_agents
			SET config = ?
			WHERE question_set_id = ? AND agent_id = ?
		`, updatedConfig, row.QuestionSetID, row.AgentID).Error; err != nil {
			return rotated, err
		}
		rotated++
	}

	return rotated, nil
}

func rotateEncryptedConfigValue(raw string, current *CurrentEncryptionKey, previous *CurrentEncryptionKey) (string, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || looksLikePlaintextJSON(trimmed) {
		return "", false, nil
	}

	_, currentErr := security.DecryptWithKey(current.Key, trimmed)
	if currentErr == nil {
		return "", false, nil
	}

	plaintext, previousErr := security.DecryptWithKey(previous.Key, trimmed)
	if previousErr != nil {
		return "", false, fmt.Errorf("config could not be decrypted with current or previous key: current=%v; previous=%v", currentErr, previousErr)
	}

	updatedConfig, err := security.EncryptWithKey(current.Key, plaintext)
	if err != nil {
		return "", false, err
	}

	return updatedConfig, true, nil
}

func parseEncryptionRotationFlag(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func looksLikePlaintextJSON(raw string) bool {
	return (strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}")) ||
		(strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]"))
}
