package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
)

type OrganizationHandler struct {
	db *gorm.DB
}

func NewOrganizationHandler(db *gorm.DB) *OrganizationHandler {
	return &OrganizationHandler{db: db}
}

type OrgResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	IsSuspended bool       `json:"is_suspended"`
	ManagerID   *uuid.UUID `json:"manager_id"`
	ManagerName string     `json:"manager_name"`
	UserCount   int64      `json:"user_count"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (h *OrganizationHandler) ListOrganizationsAdmin(c echo.Context) error {
	userID := middleware.GetUserID(c)
	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	var orgs []models.Organization
	h.db.Preload("Manager").Find(&orgs)

	result := make([]OrgResponse, len(orgs))
	for i, org := range orgs {
		var userCount int64
		h.db.Raw(`SELECT COUNT(*) FROM user_organizations WHERE organization_id = ?`, org.ID).Scan(&userCount)

		managerName := ""
		if org.Manager != nil {
			managerName = org.Manager.Name
		}

		result[i] = OrgResponse{
			ID:          org.ID,
			Name:        org.Name,
			IsSuspended: org.IsSuspended,
			ManagerID:   org.ManagerID,
			ManagerName: managerName,
			UserCount:   userCount,
			CreatedAt:   org.CreatedAt,
		}
	}

	return c.JSON(http.StatusOK, result)
}

func (h *OrganizationHandler) CreateOrganization(c echo.Context) error {
	userID := middleware.GetUserID(c)
	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	var req struct {
		Name      string     `json:"name"`
		ManagerID *uuid.UUID `json:"manager_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	org := models.Organization{
		ID:        uuid.New(),
		Name:      req.Name,
		ManagerID: req.ManagerID,
	}

	if err := h.db.Create(&org).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create organization"})
	}

	return c.JSON(http.StatusCreated, org)
}

func (h *OrganizationHandler) UpdateOrganization(c echo.Context) error {
	userID := middleware.GetUserID(c)
	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	orgIDStr := c.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid organization ID"})
	}

	var req struct {
		Name        *string    `json:"name"`
		IsSuspended *bool      `json:"is_suspended"`
		ManagerID   *uuid.UUID `json:"manager_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	var org models.Organization
	if err := h.db.First(&org, "id = ?", orgID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Organization not found"})
	}

	if req.Name != nil {
		org.Name = *req.Name
	}
	if req.IsSuspended != nil {
		org.IsSuspended = *req.IsSuspended
	}
	if req.ManagerID != nil {
		org.ManagerID = req.ManagerID
	}

	if err := h.db.Save(&org).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update organization"})
	}

	return c.JSON(http.StatusOK, org)
}

func (h *OrganizationHandler) DeleteOrganization(c echo.Context) error {
	userID := middleware.GetUserID(c)
	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	orgIDStr := c.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid organization ID"})
	}

	if err := h.db.Delete(&models.Organization{}, "id = ?", orgID).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete organization"})
	}

	return c.NoContent(http.StatusNoContent)
}

// GetOrgProfile returns detailed profile information for an organization (admin only)
func (h *OrganizationHandler) GetOrgProfile(c echo.Context) error {
	userID := middleware.GetUserID(c)
	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required"})
	}

	orgIDStr := c.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid organization ID"})
	}

	var org models.Organization
	if err := h.db.Preload("Manager").First(&org, "id = ?", orgID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Organization not found"})
	}

	// Get users with workspace count
	var users []struct {
		ID             string    `json:"id"`
		Name           string    `json:"name"`
		Email          string    `json:"email"`
		IsAdmin        bool      `json:"is_admin"`
		CreatedAt      time.Time `json:"created_at"`
		WorkspaceCount int64     `json:"workspace_count"`
	}
	h.db.Raw(`
		SELECT u.id, u.name, u.email, u.is_admin, u.created_at, 
		       COUNT(w.id) as workspace_count
		FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		LEFT JOIN workspaces w ON w.user_id = u.id
		WHERE uo.organization_id = ?
		GROUP BY u.id
	`, orgID).Scan(&users)

	// Get workspaces with counts
	var workspaces []struct {
		ID         string    `json:"id"`
		Name       string    `json:"name"`
		UserID     string    `json:"user_id"`
		CreatedAt  time.Time `json:"created_at"`
		AgentCount int64     `json:"agent_count"`
		RunCount   int64     `json:"run_count"`
	}
	h.db.Raw(`
		SELECT w.id, w.name, w.user_id, w.created_at,
		       (SELECT COUNT(*) FROM agents WHERE workspace_id = w.id) as agent_count,
		       (SELECT COUNT(*) FROM runs WHERE workspace_id = w.id) as run_count
		FROM workspaces w
		WHERE w.organization_id = ?
	`, orgID).Scan(&workspaces)

	// Add user info to workspaces
	userMap := make(map[string]string)
	for _, u := range users {
		userMap[u.ID] = u.Name
	}

	type WsWithUser struct {
		ID         string    `json:"id"`
		Name       string    `json:"name"`
		UserID     string    `json:"user_id"`
		UserName   string    `json:"user_name"`
		CreatedAt  time.Time `json:"created_at"`
		AgentCount int64     `json:"agent_count"`
		RunCount   int64     `json:"run_count"`
	}
	wsResults := make([]WsWithUser, len(workspaces))
	for i, ws := range workspaces {
		wsResults[i] = WsWithUser{
			ID:         ws.ID,
			Name:       ws.Name,
			UserID:     ws.UserID,
			UserName:   userMap[ws.UserID],
			CreatedAt:  ws.CreatedAt,
			AgentCount: ws.AgentCount,
			RunCount:   ws.RunCount,
		}
	}

	result := map[string]any{
		"id":           org.ID,
		"name":         org.Name,
		"is_suspended": org.IsSuspended,
		"manager_id":   org.ManagerID,
		"manager":      org.Manager,
		"users":        users,
		"workspaces":   wsResults,
		"created_at":   org.CreatedAt,
	}

	return c.JSON(http.StatusOK, result)
}
