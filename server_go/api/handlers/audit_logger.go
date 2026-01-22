package handlers

import (
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func logAuditAction(db *gorm.DB, c echo.Context, orgID uuid.UUID, action, resourceType, resourceID string, details any) {
	// 1. Check if audit logs are enabled for this organization
	var org models.Organization
	if err := db.Select("audit_logs_enabled").First(&org, "id = ?", orgID).Error; err != nil {
		return
	}
	if !org.AuditLogsEnabled {
		return
	}

	// 2. Prepare details string
	detailsStr := ""
	if details != nil {
		if s, ok := details.(string); ok {
			detailsStr = s
		} else {
			bytes, _ := json.Marshal(details)
			detailsStr = string(bytes)
		}
	}

	// 3. Get current user
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return
	}

	// 4. Create log entry
	log := models.AuditLog{
		ID:             uuid.New(),
		OrganizationID: orgID,
		UserID:         userID,
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		Details:        detailsStr,
		RemoteIP:       c.RealIP(),
	}

	db.Create(&log)
}
