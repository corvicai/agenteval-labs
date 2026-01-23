package models

import (
	"time"

	"github.com/google/uuid"
)

type InviteCode struct {
	Code           string     `gorm:"primaryKey" json:"code"`
	CreatedBy      uuid.UUID  `gorm:"type:uuid;not null" json:"created_by"`
	OrganizationID *uuid.UUID `gorm:"type:uuid" json:"organization_id"` // Null if for new org
	Role           string     `gorm:"default:'member'" json:"role"`
	IsNewOrg       bool       `gorm:"default:false" json:"is_new_org"`
	ExpiresAt      time.Time  `gorm:"not null" json:"expires_at"`
	MaxUses        int        `gorm:"default:1" json:"max_uses"`
	UseCount       int        `gorm:"default:0" json:"use_count"`
	CreatedAt      time.Time  `json:"created_at"`
}

type InviteCodeUsage struct {
	ID     uuid.UUID `gorm:"primaryKey" json:"id"`
	Code   string    `gorm:"index;not null" json:"code"`
	UserID uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	UsedAt time.Time `json:"used_at"`
}
