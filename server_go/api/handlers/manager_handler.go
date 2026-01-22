package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
)

type ManagerHandler struct {
	db        *gorm.DB
	jwtSecret string
}

func NewManagerHandler(db *gorm.DB, jwtSecret string) *ManagerHandler {
	return &ManagerHandler{db: db, jwtSecret: jwtSecret}
}

// Helper to verify manager status and get current session org ID
func (h *ManagerHandler) getManagerOrgID(c echo.Context) (*uuid.UUID, error) {
	userID := middleware.GetUserID(c)
	orgID := middleware.GetOrgID(c)

	if userID == uuid.Nil || orgID == uuid.Nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Missing user or organization context")
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "User not found")
	}

	// Site Admins are managers of any org session they have
	if user.IsAdmin {
		return &orgID, nil
	}

	// Check many-to-many role
	var userOrg models.UserOrganization
	if err := h.db.First(&userOrg, "user_id = ? AND organization_id = ?", userID, orgID).Error; err != nil {
		return nil, echo.NewHTTPError(http.StatusForbidden, "User is not a member of this organization")
	}

	if userOrg.Role != "manager" {
		return nil, echo.NewHTTPError(http.StatusForbidden, "Not a manager of this organization")
	}

	return &orgID, nil
}

// GetOrgUsers returns all users in the manager's organization
func (h *ManagerHandler) GetOrgUsers(c echo.Context) error {
	orgID, err := h.getManagerOrgID(c)
	if err != nil {
		return err
	}

	type UserResponse struct {
		ID             uuid.UUID `json:"id"`
		Name           string    `json:"name"`
		Email          string    `json:"email"`
		IsAdmin        bool      `json:"is_admin"`
		IsSuspended    bool      `json:"is_suspended"`
		WorkspaceCount int64     `json:"workspace_count"`
		CreatedAt      time.Time `json:"created_at"`
	}

	var users []UserResponse
	h.db.Raw(`
		SELECT u.id, u.name, u.email, u.is_admin, u.is_suspended, u.created_at,
		       COUNT(w.id) as workspace_count
		FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		LEFT JOIN workspaces w ON w.user_id = u.id
		WHERE uo.organization_id = ?
		GROUP BY u.id
		ORDER BY u.name
	`, orgID).Scan(&users)

	return c.JSON(http.StatusOK, users)
}

// CreateOrgUser creates a new user in the manager's organization
func (h *ManagerHandler) CreateOrgUser(c echo.Context) error {
	orgID, err := h.getManagerOrgID(c)
	if err != nil {
		return err
	}

	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if req.Name == "" || req.Email == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Name, email and password are required"})
	}

	// Check if email exists
	var existing models.User
	if err := h.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Email already registered"})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to hash password"})
	}

	user := models.User{
		ID:           uuid.New(),
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		IsAdmin:      false,
		IsSuspended:  false,
	}

	if err := h.db.Create(&user).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
	}

	// Add to many-to-many junction
	userOrg := models.UserOrganization{
		UserID:         user.ID,
		OrganizationID: *orgID,
		Role:           "member",
		JoinedAt:       time.Now(),
	}
	h.db.Create(&userOrg)

	logAuditAction(h.db, c, *orgID, "CREATE_USER", "USER", user.ID.String(), map[string]string{"email": user.Email, "name": user.Name})

	// Create default workspace
	workspace := models.Workspace{
		ID:             uuid.New(),
		UserID:         user.ID,
		OrganizationID: *orgID,
		Name:           "Default Workspace",
	}
	h.db.Create(&workspace)

	return c.JSON(http.StatusCreated, user)
}

// UpdateOrgUser updates a user in the manager's organization
func (h *ManagerHandler) UpdateOrgUser(c echo.Context) error {
	orgID, err := h.getManagerOrgID(c)
	if err != nil {
		return err
	}

	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
	}

	var user models.User
	if err := h.db.Raw(`
		SELECT u.* FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		WHERE u.id = ? AND uo.organization_id = ?
	`, userID, orgID).Scan(&user).Error; err != nil || user.ID == uuid.Nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found in your organization"})
	}

	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Email != "" && req.Email != user.Email {
		// Check if email is taken
		var existing models.User
		if err := h.db.Where("email = ? AND id != ?", req.Email, userID).First(&existing).Error; err == nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": "Email already in use"})
		}
		user.Email = req.Email
	}

	if err := h.db.Save(&user).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update user"})
	}

	logAuditAction(h.db, c, *orgID, "UPDATE_USER", "USER", user.ID.String(), map[string]string{"email": user.Email, "name": user.Name})

	return c.JSON(http.StatusOK, user)
}

// ToggleUserSuspension suspends or activates a user
func (h *ManagerHandler) ToggleUserSuspension(c echo.Context) error {
	orgID, err := h.getManagerOrgID(c)
	if err != nil {
		return err
	}

	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
	}

	// Can't suspend yourself
	callerID := middleware.GetUserID(c)
	if userID == callerID {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Cannot suspend yourself"})
	}

	var user models.User
	if err := h.db.Raw(`
		SELECT u.* FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		WHERE u.id = ? AND uo.organization_id = ?
	`, userID, orgID).Scan(&user).Error; err != nil || user.ID == uuid.Nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found in your organization"})
	}

	user.IsSuspended = !user.IsSuspended

	if err := h.db.Save(&user).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update user"})
	}

	action := "ACTIVATE_USER"
	if user.IsSuspended {
		action = "SUSPEND_USER"
	}
	logAuditAction(h.db, c, *orgID, action, "USER", user.ID.String(), nil)

	return c.JSON(http.StatusOK, map[string]any{
		"is_suspended": user.IsSuspended,
		"message":      map[bool]string{true: "User suspended", false: "User activated"}[user.IsSuspended],
	})
}

// GetOrgWorkspaces returns all workspaces in the manager's organization
func (h *ManagerHandler) GetOrgWorkspaces(c echo.Context) error {
	orgID, err := h.getManagerOrgID(c)
	if err != nil {
		return err
	}

	type WorkspaceResponse struct {
		ID         uuid.UUID `json:"id"`
		Name       string    `json:"name"`
		UserID     uuid.UUID `json:"user_id"`
		UserName   string    `json:"user_name"`
		AgentCount int64     `json:"agent_count"`
		RunCount   int64     `json:"run_count"`
		CreatedAt  time.Time `json:"created_at"`
	}

	var workspaces []WorkspaceResponse
	h.db.Raw(`
		SELECT w.id, w.name, w.user_id, u.name as user_name, w.created_at,
		       (SELECT COUNT(*) FROM agents WHERE workspace_id = w.id) as agent_count,
		       (SELECT COUNT(*) FROM runs WHERE workspace_id = w.id) as run_count
		FROM workspaces w
		JOIN users u ON w.user_id = u.id
		WHERE w.organization_id = ?
		ORDER BY w.name
	`, orgID).Scan(&workspaces)

	return c.JSON(http.StatusOK, workspaces)
}

// GetOrgAgents returns all agents across the manager's organization
func (h *ManagerHandler) GetOrgAgents(c echo.Context) error {
	orgID, err := h.getManagerOrgID(c)
	if err != nil {
		return err
	}

	type AgentResponse struct {
		ID            uuid.UUID `json:"id"`
		Name          string    `json:"name"`
		ProviderType  string    `json:"provider_type"`
		Enabled       bool      `json:"enabled"`
		WorkspaceID   uuid.UUID `json:"workspace_id"`
		WorkspaceName string    `json:"workspace_name"`
		UserName      string    `json:"user_name"`
		CreatedAt     time.Time `json:"created_at"`
	}

	var agents []AgentResponse
	h.db.Raw(`
		SELECT a.id, a.name, a.provider_type, a.enabled, a.workspace_id, a.created_at,
		       w.name as workspace_name, u.name as user_name
		FROM agents a
		JOIN workspaces w ON a.workspace_id = w.id
		JOIN users u ON w.user_id = u.id
		WHERE w.organization_id = ?
		ORDER BY a.name
	`, orgID).Scan(&agents)

	return c.JSON(http.StatusOK, agents)
}

// GetOrgRuns returns all runs across the manager's organization
func (h *ManagerHandler) GetOrgRuns(c echo.Context) error {
	orgID, err := h.getManagerOrgID(c)
	if err != nil {
		return err
	}

	type RunResponse struct {
		ID              uuid.UUID `json:"id"`
		Status          string    `json:"status"`
		WorkspaceID     uuid.UUID `json:"workspace_id"`
		WorkspaceName   string    `json:"workspace_name"`
		UserName        string    `json:"user_name"`
		QuestionSetName string    `json:"question_set_name"`
		ResultCount     int64     `json:"result_count"`
		CreatedAt       time.Time `json:"created_at"`
	}

	var runs []RunResponse
	h.db.Raw(`
		SELECT r.id, r.status, r.workspace_id, r.created_at,
		       w.name as workspace_name, u.name as user_name,
		       qs.name as question_set_name,
		       (SELECT COUNT(*) FROM run_results WHERE run_id = r.id) as result_count
		FROM runs r
		JOIN workspaces w ON r.workspace_id = w.id
		JOIN users u ON w.user_id = u.id
		LEFT JOIN question_sets qs ON r.question_set_id = qs.id
		WHERE w.organization_id = ?
		ORDER BY r.created_at DESC
		LIMIT 100
	`, orgID).Scan(&runs)

	return c.JSON(http.StatusOK, runs)
}

// GetOrgStats returns statistics for the manager's organization
func (h *ManagerHandler) GetOrgStats(c echo.Context) error {
	orgID, err := h.getManagerOrgID(c)
	if err != nil {
		return err
	}

	var stats struct {
		UserCount      int64 `json:"user_count"`
		WorkspaceCount int64 `json:"workspace_count"`
		AgentCount     int64 `json:"agent_count"`
		RunCount       int64 `json:"run_count"`
	}

	h.db.Raw(`SELECT COUNT(*) FROM user_organizations WHERE organization_id = ?`, orgID).Scan(&stats.UserCount)
	h.db.Raw(`SELECT COUNT(*) FROM workspaces WHERE organization_id = ?`, orgID).Scan(&stats.WorkspaceCount)
	h.db.Raw(`SELECT COUNT(*) FROM agents a JOIN workspaces w ON a.workspace_id = w.id WHERE w.organization_id = ?`, orgID).Scan(&stats.AgentCount)
	h.db.Raw(`SELECT COUNT(*) FROM runs r JOIN workspaces w ON r.workspace_id = w.id WHERE w.organization_id = ?`, orgID).Scan(&stats.RunCount)

	return c.JSON(http.StatusOK, stats)
}

// ImpersonateUser allows a manager to login as another user in their organization
func (h *ManagerHandler) ImpersonateUser(c echo.Context) error {
	orgID, err := h.getManagerOrgID(c)
	if err != nil {
		return err
	}

	targetUserIDStr := c.Param("user_id")
	targetUserID, err := uuid.Parse(targetUserIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid target user ID"})
	}

	// Manager ID (impersonator)
	managerID := middleware.GetUserID(c)

	// Can't impersonate yourself (redundant but safe)
	if targetUserID == managerID {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Cannot impersonate yourself"})
	}

	// Verify target user is in the same organization
	var targetUser models.User
	if err := h.db.First(&targetUser, "id = ?", targetUserID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Target user not found"})
	}

	// Check if target user is in the manager's organization
	var userOrg models.UserOrganization
	if err := h.db.First(&userOrg, "user_id = ? AND organization_id = ?", targetUserID, orgID).Error; err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Target user is not in your organization"})
	}

	// Get target user's first workspace in THIS organization
	var workspace models.Workspace
	h.db.Where("user_id = ? AND organization_id = ?", targetUserID, orgID).First(&workspace)

	workspaceID := ""
	if workspace.ID != uuid.Nil {
		workspaceID = workspace.ID.String()
	}

	// Generate JWT for target user, with managerID as ImpersonatorID
	token, err := middleware.GenerateToken(
		targetUserID.String(),
		workspaceID,
		orgID.String(),
		targetUser.Email,
		h.jwtSecret,
		managerID.String(),
	)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate impersonation token"})
	}

	// Set the token cookie
	cookie := new(http.Cookie)
	cookie.Name = "token"
	cookie.Value = token
	cookie.Expires = time.Now().Add(24 * time.Hour)
	cookie.HttpOnly = true
	cookie.Path = "/"
	cookie.SameSite = http.SameSiteLaxMode
	if c.Request().TLS != nil || c.Scheme() == "https" || c.Request().Header.Get("X-Forwarded-Proto") == "https" {
		cookie.Secure = true
	}
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, map[string]any{
		"token":   token,
		"message": "Now impersonating " + targetUser.Name,
		"user":    targetUser,
	})
}
