package models

import (
	"time"

	"github.com/google/uuid"
)

type LoginLog struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID         *uuid.UUID `gorm:"type:uuid;index" json:"user_id"`   // Nullable, as login might fail for unknown user
	UserEmail      string     `gorm:"not null" json:"user_email"`       // Always capture the attempted email
	OrganizationID *uuid.UUID `gorm:"type:uuid" json:"organization_id"` // Nullable
	IPAddress      string     `gorm:"not null" json:"ip_address"`
	UserAgent      string     `gorm:"type:text" json:"user_agent"`
	Status         string     `gorm:"not null" json:"status"`          // 'success', 'failed'
	FailureReason  string     `gorm:"type:text" json:"failure_reason"` // e.g. 'invalid_password', 'suspended'
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`
}
