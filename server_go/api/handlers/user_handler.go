package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/internal/validation"
	"benchmarking-platform/models"
)

// Me returns the current user info
func (h *AuthHandler) Me(c echo.Context) error {
	userID := middleware.GetUserID(c)
	orgID := middleware.GetOrgID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	var org models.Organization
	h.db.First(&org, "id = ?", orgID)

	response := map[string]any{
		"user": h.mapUserToResponse(user),
		"organization": map[string]any{
			"id":   org.ID,
			"name": org.Name,
		},
	}

	return c.JSON(http.StatusOK, response)
}

// ListUsers returns all users (admin only)
func (h *AuthHandler) ListUsers(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	// Check if current user is admin
	var currentUser models.User
	if err := h.db.First(&currentUser, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}
	if !currentUser.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	var users []models.User
	h.db.Preload("Workspaces").Order("created_at DESC").Find(&users)

	result := make([]UserResponse, len(users))
	for i, u := range users {
		result[i] = h.mapUserToResponse(u)
	}

	return c.JSON(http.StatusOK, result)
}

// CreateUserAdmin creates a new user (admin only)
func (h *AuthHandler) CreateUserAdmin(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	// Check if current user is admin
	var currentUser models.User
	if err := h.db.First(&currentUser, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}
	if !currentUser.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	var req struct {
		Name           string `json:"name"`
		Email          string `json:"email"`
		Password       string `json:"password"`
		IsAdmin        bool   `json:"is_admin"`
		OrganizationID string `json:"organization_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if err := validation.ValidateUserName(req.Name); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := validation.ValidateEmail(req.Email); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if req.Password != "" {
		if err := validation.ValidatePassword(req.Password); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	}

	// Check if email exists
	var existing models.User
	if err := h.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Email already registered"})
	}

	// Resolve target organization
	var targetOrgID uuid.UUID

	if req.OrganizationID != "" {
		if id, err := uuid.Parse(req.OrganizationID); err == nil {
			targetOrgID = id
		}
	}

	// If no org specified, try to find the admin's org
	if targetOrgID == uuid.Nil {
		var userOrg models.UserOrganization
		if err := h.db.First(&userOrg, "user_id = ?", currentUser.ID).Error; err == nil {
			targetOrgID = userOrg.OrganizationID
		}
	}

	// If still nil, we can't create the user correctly without an org
	if targetOrgID == uuid.Nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Organization ID is required"})
	}

	// Default password if not provided
	pass := req.Password
	if pass == "" {
		pass = "changeme"
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("[AUTH] Failed to hash password for admin-created user %s: %v", req.Email, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
	}

	user := models.User{
		ID:           uuid.New(),
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		IsAdmin:      req.IsAdmin,
	}

	if err := h.db.Create(&user).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
	}

	// Link user to organization
	userOrg := models.UserOrganization{
		UserID:         user.ID,
		OrganizationID: targetOrgID,
		Role:           "member",
		JoinedAt:       time.Now().UTC(),
	}
	h.db.Create(&userOrg)

	// Create default workspace
	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   "main",
	}
	h.db.Create(&workspace)

	// Create default client
	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        "Default Client",
	}
	h.db.Create(&client)

	return c.JSON(http.StatusCreated, UserResponse{
		ID:             user.ID.String(),
		Name:           user.Name,
		Email:          user.Email,
		IsAdmin:        user.HasAdminAccess(),
		OrganizationID: targetOrgID.String(),
	})
}

// UpdateUser updates a user (admin only)
func (h *AuthHandler) UpdateUser(c echo.Context) error {
	adminID := middleware.GetUserID(c)
	if adminID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	// Check if current user is admin
	var admin models.User
	if err := h.db.First(&admin, "id = ?", adminID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}
	if !admin.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	userIDStr := c.Param("user_id")
	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
	}

	var req struct {
		Name     *string `json:"name"`
		Email    *string `json:"email"`
		Password *string `json:"password"`
		IsAdmin  *bool   `json:"is_admin"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userUUID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	if req.Name != nil {
		if err := validation.ValidateUserName(*req.Name); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		user.Name = *req.Name
	}
	if req.Email != nil {
		if err := validation.ValidateEmail(*req.Email); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		user.Email = *req.Email
	}
	if req.IsAdmin != nil {
		user.IsAdmin = *req.IsAdmin
	}
	if req.Password != nil && *req.Password != "" {
		if err := validation.ValidatePassword(*req.Password); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			logger.Error("[AUTH] Failed to hash password during user update %s: %v", user.ID, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		}
		user.PasswordHash = string(hashed)
	}

	h.db.Save(&user)

	return c.JSON(http.StatusOK, UserResponse{
		ID:      user.ID.String(),
		Name:    user.Name,
		Email:   user.Email,
		IsAdmin: user.HasAdminAccess(),
	})
}

// DeleteUser deletes a user (admin only)
func (h *AuthHandler) DeleteUser(c echo.Context) error {
	adminID := middleware.GetUserID(c)
	if adminID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	// Check if current user is admin
	var admin models.User
	if err := h.db.First(&admin, "id = ?", adminID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}
	if !admin.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	userIDStr := c.Param("user_id")
	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
	}

	// Prevent deleting yourself
	if userUUID == adminID {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Cannot delete yourself"})
	}

	if err := h.db.Delete(&models.User{}, "id = ?", userUUID).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete user"})
	}

	return c.NoContent(http.StatusNoContent)
}

// GetUserProfile returns detailed profile information for a user (admin only)
func (h *AuthHandler) GetUserProfile(c echo.Context) error {
	// Verify admin access
	callerID := middleware.GetUserID(c)
	var caller models.User
	if err := h.db.First(&caller, "id = ?", callerID).Error; err != nil || !caller.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	// Load primary organization (first one found)
	// In multi-org implementation, this profile endpoint might need to return a list,
	// but for backward compatibility we return the first one.
	var org models.Organization
	var orgIDStr string
	var userOrg models.UserOrganization

	if err := h.db.First(&userOrg, "user_id = ?", user.ID).Error; err == nil {
		h.db.First(&org, "id = ?", userOrg.OrganizationID)
		orgIDStr = org.ID.String()
	}

	// Get workspaces with agent count
	var workspaces []struct {
		models.Workspace
		AgentCount int64 `json:"agent_count"`
	}
	h.db.Raw(`
		SELECT w.*, COUNT(a.id) as agent_count
		FROM workspaces w
		LEFT JOIN agents a ON a.workspace_id = w.id
		WHERE w.user_id = ?
		GROUP BY w.id
	`, userID).Scan(&workspaces)

	// Check if user is manager of their org
	isManager := false
	if org.ManagerID != nil && *org.ManagerID == userID {
		isManager = true
	}
	// Also check role in user_organizations
	if userOrg.Role == "manager" {
		isManager = true
	}

	result := map[string]any{
		"id":              user.ID,
		"name":            user.Name,
		"email":           user.Email,
		"is_admin":        user.HasAdminAccess(),
		"is_manager":      isManager,
		"organization_id": orgIDStr,
		"organization":    org,
		"workspaces":      workspaces,
		"created_at":      user.CreatedAt,
	}

	return c.JSON(http.StatusOK, result)
}
