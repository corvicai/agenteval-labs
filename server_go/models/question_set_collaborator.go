package models

import (
	"time"

	"github.com/google/uuid"
)

// QuestionSetCollaborator represents a user that has been granted access to a question set
// owned by another user. The owner is implicit via the question set's workspace.
type QuestionSetCollaborator struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	QuestionSetID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"question_set_id"`
	UserID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Role            string     `gorm:"not null;default:'editor'" json:"role"`
	InvitedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"invited_by_user_id"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (QuestionSetCollaborator) TableName() string {
	return "question_set_collaborators"
}

// QuestionSetCollabInvite represents a pending invitation to collaborate on a question set.
type QuestionSetCollabInvite struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Token           string     `gorm:"uniqueIndex;not null" json:"token"`
	QuestionSetID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"question_set_id"`
	CreatedByUserID uuid.UUID  `gorm:"type:uuid;not null" json:"created_by_user_id"`
	InvitedEmail    string     `json:"invited_email,omitempty"`
	InvitedUserID   *uuid.UUID `gorm:"type:uuid" json:"invited_user_id,omitempty"`
	Role            string     `gorm:"not null;default:'editor'" json:"role"`
	ExpiresAt       time.Time  `gorm:"not null;index" json:"expires_at"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (QuestionSetCollabInvite) TableName() string {
	return "question_set_collab_invites"
}
