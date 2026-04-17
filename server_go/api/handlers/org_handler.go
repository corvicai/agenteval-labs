package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
)

// JoinOrganizationRequest payload
type JoinOrganizationRequest struct {
	InviteCode string `json:"invite_code"`
}

// JoinOrganization handles adding a user to an organization via invite code post-login
func (h *AuthHandler) JoinOrganization(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	var req JoinOrganizationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if req.InviteCode == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invite code is required"})
	}

	tx := h.db.Begin()

	var invite models.InviteCode
	if err := tx.Where("code = ?", req.InviteCode).First(&invite).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid invite code"})
	}

	if invite.UseCount >= invite.MaxUses {
		tx.Rollback()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invite code usage limit reached"})
	}

	if invite.ExpiresAt.Before(time.Now().UTC()) {
		tx.Rollback()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invite code has expired"})
	}

	if invite.OrganizationID == nil {
		tx.Rollback()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid invite code (no organization associated)"})
	}

	// Check if user is already a member
	var existingUO models.UserOrganization
	if err := tx.Where("user_id = ? AND organization_id = ?", userID, *invite.OrganizationID).First(&existingUO).Error; err == nil {
		tx.Rollback()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "You are already a member of this organization"})
	}

	// Add user to organization
	userOrg := models.UserOrganization{
		UserID:          userID,
		OrganizationID:  *invite.OrganizationID,
		Role:            invite.Role,
		InvitedByUserID: &invite.CreatedBy,
		JoinedAt:        time.Now().UTC(),
	}
	if err := tx.Create(&userOrg).Error; err != nil {
		fmt.Printf("[DB ERROR] Failed to join user %s to org %s: %v\n", userID, *invite.OrganizationID, err)
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to join organization database record"})
	}

	// Create default workspace for user
	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: userID,
		Name:   "main",
	}
	if err := tx.Create(&workspace).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create workspace"})
	}

	// Create default client
	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        "Default Client",
	}
	tx.Create(&client)

	// Update invite code
	invite.UseCount++
	if err := tx.Save(&invite).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update invite code"})
	}

	// Record usage
	usage := models.InviteCodeUsage{
		ID:     uuid.New(),
		Code:   invite.Code,
		UserID: userID,
		UsedAt: time.Now().UTC(),
	}
	if err := tx.Create(&usage).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to record invite usage"})
	}

	tx.Commit()

	if workspace.ID != uuid.Nil && client.ID != uuid.Nil {
		seedExampleSetupIfFirstWorkspace(h.db, userID, workspace, client)
	}

	// Get user for token
	var user models.User
	h.db.First(&user, userID)

	// Generate full token
	token, err := middleware.GenerateToken(user.ID.String(), workspace.ID.String(), invite.OrganizationID.String(), user.Email, h.jwtSecret, "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
	}

	h.setTokenCookie(c, token)

	return c.JSON(http.StatusOK, AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		User:      h.mapUserToResponse(user),
		Workspace: &workspace,
	})
}

// SelectOrganizationRequest payload
type SelectOrganizationRequest struct {
	OrgID string `json:"org_id"`
}

// SelectOrganization handles switching the active organization for the current session
func (h *AuthHandler) SelectOrganization(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	var req SelectOrganizationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	orgID, err := uuid.Parse(req.OrgID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid organization ID"})
	}

	// Check if user belongs to this org
	var uo models.UserOrganization
	if err := h.db.Where("user_id = ? AND organization_id = ?", userID, orgID).First(&uo).Error; err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "You do not belong to this organization"})
	}

	// Get first workspace for this user
	var workspace models.Workspace
	if err := h.db.Where("user_id = ?", userID).First(&workspace).Error; err != nil {
		// If no workspace exists, create one (shouldn't happen with normal flow, but good for safety)
		workspace = models.Workspace{
			ID:     uuid.New(),
			UserID: userID,
			Name:   "main",
		}
		h.db.Create(&workspace)
	}

	// Get user
	var user models.User
	h.db.First(&user, userID)

	// Generate new token
	token, err := middleware.GenerateToken(user.ID.String(), workspace.ID.String(), orgID.String(), user.Email, h.jwtSecret, "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
	}

	h.setTokenCookie(c, token)

	return c.JSON(http.StatusOK, AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		User:      h.mapUserToResponse(user),
		Workspace: &workspace,
	})
}

func (h *AuthHandler) ListOrganizations(c echo.Context) error {
	var orgs []models.Organization
	h.db.Find(&orgs)
	return c.JSON(http.StatusOK, orgs)
}

// ToggleAuditLogs enables or disables audit logging for an organization (admin only)
func (h *AuthHandler) ToggleAuditLogs(c echo.Context) error {
	// Verify general admin
	userID := middleware.GetUserID(c)
	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	if !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Only general admins can perform this action"})
	}

	orgID := c.Param("org_id")
	var org models.Organization
	if err := h.db.First(&org, "id = ?", orgID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Organization not found"})
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	org.AuditLogsEnabled = req.Enabled
	if err := h.db.Save(&org).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update organization"})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"audit_logs_enabled": org.AuditLogsEnabled,
		"message":            "Organization audit log settings updated",
	})
}

// GetManagers returns all users who are managers of organizations (for Dev Quick Login)
func (h *AuthHandler) GetManagers(c echo.Context) error {
	type ManagerInfo struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		OrgName string `json:"org_name"`
	}

	var managers []ManagerInfo
	h.db.Raw(`
		SELECT u.id, u.name, u.email, o.name as org_name
		FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		JOIN organizations o ON uo.organization_id = o.id
		WHERE uo.role = 'manager'
		ORDER BY o.name
	`).Scan(&managers)

	return c.JSON(http.StatusOK, managers)
}

// CheckManagerStatus checks if the current user is a manager of their current organization context
func (h *AuthHandler) CheckManagerStatus(c echo.Context) error {
	userID := middleware.GetUserID(c)
	orgID := middleware.GetOrgID(c)

	if userID == uuid.Nil || orgID == uuid.Nil {
		return c.JSON(http.StatusOK, map[string]any{"is_manager": false})
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusOK, map[string]any{"is_manager": false})
	}

	// Admins are automatically managers of any org they are in
	if user.IsAdmin {
		var org models.Organization
		h.db.First(&org, "id = ?", orgID)
		return c.JSON(http.StatusOK, map[string]any{
			"is_manager": true,
			"org_name":   org.Name,
		})
	}

	var userOrg models.UserOrganization
	err := h.db.First(&userOrg, "user_id = ? AND organization_id = ?", userID, orgID).Error
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{"is_manager": false})
	}

	isManager := userOrg.Role == "manager"

	var org models.Organization
	h.db.First(&org, "id = ?", orgID)

	return c.JSON(http.StatusOK, map[string]any{
		"is_manager": isManager,
		"org_name":   org.Name,
	})
}
