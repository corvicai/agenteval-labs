package models

import (
	"time"

	"github.com/google/uuid"
)

// AgentConfigQuarantine archives the raw encrypted ciphertext of an agent config
// before it is overwritten (due to decryption failure) or force-deleted.
// If the correct ENCRYPTION_KEY is recovered later, the original credentials can
// be restored from this table by decrypting original_ciphertext with that key.
type AgentConfigQuarantine struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	AgentID            uuid.UUID  `gorm:"type:uuid;not null;index" json:"agent_id"`
	AgentName          string     `gorm:"not null;default:''" json:"agent_name"`
	WorkspaceID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"workspace_id"`
	OriginalCiphertext string     `gorm:"type:text;not null" json:"-"`
	QuarantineReason   string     `gorm:"not null;default:'decryption_failed'" json:"quarantine_reason"`
	Action             string     `gorm:"not null;default:'overwrite'" json:"action"` // 'overwrite' | 'force_delete'
	ActorUserID        *uuid.UUID `gorm:"type:uuid" json:"actor_user_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}
