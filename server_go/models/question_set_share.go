package models

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
)

type QuestionSetShareLink struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Token                 string     `gorm:"uniqueIndex;not null" json:"token"`
	QuestionSetID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"question_set_id"`
	CreatedByUserID       uuid.UUID  `gorm:"type:uuid;not null" json:"created_by_user_id"`
	UsedByUserID          *uuid.UUID `gorm:"type:uuid" json:"used_by_user_id,omitempty"`
	AcceptedQuestionSetID *uuid.UUID `gorm:"type:uuid" json:"accepted_question_set_id,omitempty"`
	ExpiresAt             time.Time  `gorm:"not null;index" json:"expires_at"`
	UsedAt                *time.Time `json:"used_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}

func GenerateQuestionSetShareToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
