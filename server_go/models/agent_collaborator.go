package models

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
)

// AgentCollaborator represents a user that has been granted use-only access
// to an Agent owned by another user. The owner is implicit via the agent's
// workspace. Collaborators never see the agent's credentials in clear text —
// the backend decrypts the config only at execution time on their behalf.
type AgentCollaborator struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	AgentID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"agent_id"`
	UserID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Role            string     `gorm:"not null;default:'user'" json:"role"`
	InvitedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"invited_by_user_id"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (AgentCollaborator) TableName() string {
	return "agent_collaborators"
}

// AgentCollabInvite represents a pending invitation to use an Agent.
type AgentCollabInvite struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Token           string     `gorm:"uniqueIndex;not null" json:"token"`
	AgentID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"agent_id"`
	CreatedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"created_by_user_id"`
	InvitedEmail    string     `json:"invited_email,omitempty"`
	InvitedUserID   *uuid.UUID `gorm:"type:uuid" json:"invited_user_id,omitempty"`
	Role            string     `gorm:"not null;default:'user'" json:"role"`
	ExpiresAt       time.Time  `gorm:"not null;index" json:"expires_at"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (AgentCollabInvite) TableName() string {
	return "agent_collab_invites"
}

// GenerateAgentShareToken returns a URL-safe random token used for agent
// collaboration invites. 24 random bytes → ~32 char base64url string.
func GenerateAgentShareToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
