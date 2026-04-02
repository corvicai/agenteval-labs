package models

import "time"

const EncryptionKeyStatePrimaryID = "primary"

type EncryptionKeyState struct {
	ID                      string     `gorm:"primaryKey;size:64" json:"id"`
	CipherVersion           string     `gorm:"not null" json:"cipher_version"`
	ActiveFingerprint       string     `gorm:"not null" json:"active_fingerprint"`
	ActiveFormat            string     `json:"active_format"`
	ActiveCharLength        int        `json:"active_char_length"`
	ActiveParsedBytes       int        `json:"active_parsed_bytes"`
	SentinelCiphertext      string     `gorm:"type:text;not null" json:"-"`
	LastSeenFingerprint     string     `gorm:"not null" json:"last_seen_fingerprint"`
	LastSeenAt              time.Time  `gorm:"not null" json:"last_seen_at"`
	LastSeenStatus          string     `gorm:"not null" json:"last_seen_status"`
	LastMismatchAt          *time.Time `json:"last_mismatch_at,omitempty"`
	LastMismatchFingerprint string     `json:"last_mismatch_fingerprint,omitempty"`
	LastError               string     `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}
