package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"benchmarking-platform/internal/security"
	"benchmarking-platform/models"

	"gorm.io/gorm"
)

const (
	EncryptionCipherVersion       = "aes-gcm/v1"
	encryptionSentinelPlaintextV1 = "agenteval:encryption-key-sentinel:v1"
)

type CurrentEncryptionKey struct {
	Raw         string
	Key         []byte
	Format      string
	CharLength  int
	ParsedBytes int
	Fingerprint string
}

type EncryptionKeyHealth struct {
	StatePresent              bool
	StateStatus               string
	StateSummary              string
	CipherVersion             string
	ObservedFingerprintPrefix string
	StoredFingerprintPrefix   string
	LastSeenAt                *time.Time
	LastMismatchAt            *time.Time
}

type EncryptionKeyService struct {
	db *gorm.DB
}

func NewEncryptionKeyService(db *gorm.DB) *EncryptionKeyService {
	return &EncryptionKeyService{db: db}
}

func (s *EncryptionKeyService) ObserveCurrentKey() (*CurrentEncryptionKey, error) {
	return observeEncryptionKeyFromRaw(os.Getenv("ENCRYPTION_KEY"))
}

func (s *EncryptionKeyService) ObservePreviousKey() (*CurrentEncryptionKey, error) {
	raw := strings.TrimSpace(os.Getenv("ENCRYPTION_KEY_PREVIOUS"))
	if raw == "" {
		return nil, nil
	}

	return observeEncryptionKeyFromRaw(raw)
}

func observeEncryptionKeyFromRaw(raw string) (*CurrentEncryptionKey, error) {
	key, format, err := security.ParseEncryptionKey(raw)
	if err != nil {
		return nil, err
	}

	return &CurrentEncryptionKey{
		Raw:         raw,
		Key:         key,
		Format:      format,
		CharLength:  len(raw),
		ParsedBytes: len(key),
		Fingerprint: security.KeyFingerprint(key),
	}, nil
}

func (s *EncryptionKeyService) PromoteCurrentKeyState(current *CurrentEncryptionKey) error {
	if current == nil {
		return errors.New("current encryption key unavailable")
	}
	if s.db == nil {
		return errors.New("database unavailable")
	}

	state, err := s.loadState()
	if err != nil {
		return err
	}

	// Capture what the state was BEFORE promoting — used for the audit entry.
	var prevFingerprintPrefix, prevStatus string
	if state != nil {
		prevFingerprintPrefix = shortFingerprint(state.ActiveFingerprint)
		prevStatus = state.LastSeenStatus
	}

	now := time.Now().UTC()
	sentinel, err := security.EncryptWithKey(current.Key, []byte(encryptionSentinelPlaintextV1))
	if err != nil {
		return err
	}

	if state == nil {
		state = &models.EncryptionKeyState{
			ID:        models.EncryptionKeyStatePrimaryID,
			CreatedAt: now,
		}
	}

	state.CipherVersion = EncryptionCipherVersion
	state.ActiveFingerprint = current.Fingerprint
	state.ActiveFormat = current.Format
	state.ActiveCharLength = current.CharLength
	state.ActiveParsedBytes = current.ParsedBytes
	state.SentinelCiphertext = sentinel
	state.LastSeenFingerprint = current.Fingerprint
	state.LastSeenAt = now
	state.LastSeenStatus = "match"
	state.LastError = ""
	state.UpdatedAt = now

	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}

	if err := s.db.Save(state).Error; err != nil {
		return err
	}

	s.AppendKeyStateHistory(
		"promoted",
		prevFingerprintPrefix,
		shortFingerprint(current.Fingerprint),
		prevStatus,
		"match",
		"promote",
		"",
	)
	return nil
}

func (s *EncryptionKeyService) ReconcileCurrentKey() (EncryptionKeyHealth, error) {
	current, err := s.ObserveCurrentKey()
	if err != nil {
		return EncryptionKeyHealth{}, err
	}

	state, err := s.loadState()
	if err != nil {
		return EncryptionKeyHealth{}, err
	}

	now := time.Now().UTC()
	if state == nil {
		sentinel, err := security.EncryptWithKey(current.Key, []byte(encryptionSentinelPlaintextV1))
		if err != nil {
			return EncryptionKeyHealth{}, err
		}

		state = &models.EncryptionKeyState{
			ID:                  models.EncryptionKeyStatePrimaryID,
			CipherVersion:       EncryptionCipherVersion,
			ActiveFingerprint:   current.Fingerprint,
			ActiveFormat:        current.Format,
			ActiveCharLength:    current.CharLength,
			ActiveParsedBytes:   current.ParsedBytes,
			SentinelCiphertext:  sentinel,
			LastSeenFingerprint: current.Fingerprint,
			LastSeenAt:          now,
			LastSeenStatus:      "initialized",
		}
		if err := s.db.Create(state).Error; err != nil {
			return EncryptionKeyHealth{}, err
		}
		s.AppendKeyStateHistory(
			"initialized",
			"",
			shortFingerprint(current.Fingerprint),
			"",
			"initialized",
			"reconcile_init",
			"",
		)
		return buildEncryptionKeyHealth(current, state), nil
	}

	state.LastSeenFingerprint = current.Fingerprint
	state.LastSeenAt = now

	switch {
	case state.ActiveFingerprint == current.Fingerprint:
		if strings.TrimSpace(state.SentinelCiphertext) == "" {
			sentinel, err := security.EncryptWithKey(current.Key, []byte(encryptionSentinelPlaintextV1))
			if err != nil {
				return EncryptionKeyHealth{}, err
			}
			state.SentinelCiphertext = sentinel
		}

		plaintext, err := security.DecryptWithKey(current.Key, state.SentinelCiphertext)
		switch {
		case err != nil:
			state.LastSeenStatus = "sentinel_failed"
			state.LastError = err.Error()
		case string(plaintext) != encryptionSentinelPlaintextV1:
			state.LastSeenStatus = "sentinel_failed"
			state.LastError = "sentinel plaintext mismatch"
		default:
			state.LastSeenStatus = "match"
			state.LastError = ""
		}
	case state.ActiveFingerprint != current.Fingerprint:
		state.LastSeenStatus = "mismatch"
		state.LastMismatchAt = &now
		state.LastMismatchFingerprint = current.Fingerprint
		state.LastError = "current encryption key fingerprint does not match the stored active fingerprint"
	}

	if err := s.db.Save(state).Error; err != nil {
		return EncryptionKeyHealth{}, err
	}

	return buildEncryptionKeyHealth(current, state), nil
}

func (s *EncryptionKeyService) InspectCurrentKeyHealth() (EncryptionKeyHealth, error) {
	current, err := s.ObserveCurrentKey()
	if err != nil {
		return EncryptionKeyHealth{}, err
	}

	state, err := s.loadState()
	if err != nil {
		return EncryptionKeyHealth{}, err
	}

	return buildEncryptionKeyHealth(current, state), nil
}

func (s *EncryptionKeyService) loadState() (*models.EncryptionKeyState, error) {
	if s.db == nil {
		return nil, errors.New("database unavailable")
	}

	var state models.EncryptionKeyState
	if err := s.db.First(&state, "id = ?", models.EncryptionKeyStatePrimaryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

func buildEncryptionKeyHealth(current *CurrentEncryptionKey, state *models.EncryptionKeyState) EncryptionKeyHealth {
	health := EncryptionKeyHealth{
		ObservedFingerprintPrefix: shortFingerprint(current.Fingerprint),
	}

	if state == nil {
		health.StatePresent = false
		health.StateStatus = "state_missing"
		health.StateSummary = "No persisted encryption key state exists yet"
		return health
	}

	health.StatePresent = true
	health.CipherVersion = state.CipherVersion
	health.StoredFingerprintPrefix = shortFingerprint(state.ActiveFingerprint)
	health.LastSeenAt = nullableTime(state.LastSeenAt)
	health.LastMismatchAt = state.LastMismatchAt

	switch state.LastSeenStatus {
	case "initialized":
		health.StateStatus = "initialized"
		health.StateSummary = fmt.Sprintf("Encryption key state was bootstrapped with fingerprint %s", shortFingerprint(state.ActiveFingerprint))
	case "match":
		health.StateStatus = "match"
		health.StateSummary = "Current encryption key fingerprint matches the stored active fingerprint and sentinel decrypt succeeded"
	case "mismatch":
		health.StateStatus = "mismatch"
		health.StateSummary = "Current encryption key fingerprint does not match the stored active fingerprint"
	case "sentinel_failed":
		health.StateStatus = "sentinel_failed"
		health.StateSummary = "Current encryption key fingerprint matches the stored fingerprint, but sentinel verification failed"
	default:
		health.StateStatus = firstNonEmptyEncryptionHealth(state.LastSeenStatus, "unknown")
		health.StateSummary = firstNonEmptyEncryptionHealth(state.LastError, "Encryption key state exists, but no summary is available")
	}

	return health
}

func shortFingerprint(fingerprint string) string {
	trimmed := strings.TrimSpace(fingerprint)
	if len(trimmed) <= 12 {
		return trimmed
	}
	return trimmed[:12]
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	cloned := value
	return &cloned
}

func firstNonEmptyEncryptionHealth(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// AppendKeyStateHistory writes a single row to the append-only
// encryption_key_state_history table. Failures are non-fatal (logged only).
func (s *EncryptionKeyService) AppendKeyStateHistory(
	eventType, prevFingerprintPrefix, newFingerprintPrefix,
	prevStatus, newStatus, source, lastError string,
) {
	if s.db == nil {
		return
	}
	entry := &models.EncryptionKeyStateHistory{
		ID:                        generateHistoryUUID(),
		EventType:                 eventType,
		PreviousFingerprintPrefix: prevFingerprintPrefix,
		NewFingerprintPrefix:      newFingerprintPrefix,
		PreviousStatus:            prevStatus,
		NewStatus:                 newStatus,
		Source:                    source,
		LastError:                 lastError,
	}
	if err := s.db.Create(entry).Error; err != nil {
		// Non-fatal — history is observability-only. Log at warn and continue.
		// Do not use the logger package here to avoid import cycles; fmt suffices.
		_ = fmt.Sprintf("[SECURITY] Failed to append encryption key state history: %v", err)
	}
}

func generateHistoryUUID() (id [16]byte) {
	// crypto/rand-backed UUID v4
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return id
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	copy(id[:], b)
	return id
}
