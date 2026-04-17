package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/internal/validation"
	"benchmarking-platform/models"
)

// AcceptTerms persists the user's agreement to the Terms of Service
func (h *AuthHandler) AcceptTerms(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	now := time.Now().UTC()
	if err := h.db.Model(&models.User{}).Where("id = ?", userID).Update("terms_accepted_at", &now).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update terms acceptance"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// ListWorkspaces returns all workspaces for the current user
func (h *AuthHandler) ListWorkspaces(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	var workspaces []models.Workspace
	h.db.Where("user_id = ?", userID).Find(&workspaces)

	// Fix zeroing for ListWorkspaces
	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}
	safeUser := user
	safeUser.Workspaces = nil
	safeUser.Organizations = nil
	safeUser.UserOrgs = nil

	for i := range workspaces {
		workspaces[i].User = safeUser
	}

	// Add agent count to each workspace
	type WorkspaceWithCount struct {
		models.Workspace
		AgentCount int64 `json:"agent_count"`
	}

	result := make([]WorkspaceWithCount, len(workspaces))
	for i, ws := range workspaces {
		var count int64
		h.db.Model(&models.Agent{}).Where("workspace_id = ?", ws.ID).Count(&count)
		result[i] = WorkspaceWithCount{
			Workspace:  ws,
			AgentCount: count,
		}
	}

	return c.JSON(http.StatusOK, result)
}

// CreateWorkspace creates a new workspace for the current user
func (h *AuthHandler) CreateWorkspace(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if err := validation.ValidateWorkspaceName(req.Name); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: userID,
		Name:   req.Name,
	}

	if err := h.db.Create(&workspace).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create workspace"})
	}

	// Create default client
	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        "Default Client",
	}
	h.db.Create(&client)

	return c.JSON(http.StatusCreated, workspace)
}

// SwitchWorkspace generates a new token for a different workspace
func (h *AuthHandler) SwitchWorkspace(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	workspaceID := c.Param("workspace_id")
	wsUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid workspace ID"})
	}

	// Verify workspace belongs to user
	var workspace models.Workspace
	if err := h.db.Preload("User").Where("id = ? AND user_id = ?", wsUUID, userID).First(&workspace).Error; err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Workspace not found or access denied"})
	}

	// Get user email
	var user models.User
	h.db.First(&user, "id = ?", userID)

	// Generate new token with new workspace (no organization)
	token, err := middleware.GenerateToken(userID.String(), workspaceID, "", user.Email, h.jwtSecret, "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
	}

	h.setTokenCookie(c, token)

	return c.JSON(http.StatusOK, AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		User: UserResponse{
			ID:      user.ID.String(),
			Name:    user.Name,
			Email:   user.Email,
			IsAdmin: user.IsAdmin,
		},
		Workspace: &workspace,
	})
}
