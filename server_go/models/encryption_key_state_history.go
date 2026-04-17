package models

import (
	"time"

	"github.com/google/uuid"
)

// EncryptionKeyStateHistory is an append-only audit trail of every change to
// the active encryption key state. Each row represents a discrete event (key
// rotation, auto-promote, first initialization, etc.) so operators can answer
// "when did the key change and why?" without relying on the single-row
// encryption_key_states table which is always overwritten in place.
type EncryptionKeyStateHistory struct {
	ID                        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	EventType                 string    `gorm:"not null" json:"event_type"` // 'auto_promoted' | 'rotation_completed' | 'initialized'
	PreviousFingerprintPrefix string    `json:"previous_fingerprint_prefix,omitempty"`
	NewFingerprintPrefix      string    `gorm:"not null" json:"new_fingerprint_prefix"`
	PreviousStatus            string    `json:"previous_status,omitempty"`
	NewStatus                 string    `gorm:"not null" json:"new_status"`
	Source                    string    `gorm:"not null;default:'unknown'" json:"source"` // 'startup_auto_promote' | 'startup_rotation' | 'reconcile_init'
	LastError                 string    `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt                 time.Time `json:"created_at"`
}
