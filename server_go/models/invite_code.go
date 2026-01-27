package models

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
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

func GenerateInviteForOrg(db *gorm.DB, createdBy uuid.UUID, orgID uuid.UUID, maxUses int) (string, error) {
	code := generateRandomCode(8)
	invite := InviteCode{
		Code:           code,
		CreatedBy:      createdBy,
		OrganizationID: &orgID,
		Role:           "member",
		IsNewOrg:       false,
		ExpiresAt:      time.Now().Add(7 * 24 * time.Hour),
		MaxUses:        maxUses,
		CreatedAt:      time.Now(),
	}
	if err := db.Create(&invite).Error; err != nil {
		return "", err
	}
	return code, nil
}

func GenerateInviteForPlatform(db *gorm.DB, createdBy uuid.UUID, maxUses int) (string, error) {
	code := generateRandomCode(8)
	invite := InviteCode{
		Code:      code,
		CreatedBy: createdBy,
		IsNewOrg:  true,
		Role:      "manager", // Creator becomes manager
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		MaxUses:   maxUses,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&invite).Error; err != nil {
		return "", err
	}
	return code, nil
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generateRandomCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letterBytes))))
		b[i] = letterBytes[num.Int64()]
	}
	return string(b)
}
