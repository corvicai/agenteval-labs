package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"
)

type AuthHandler struct {
	db        *gorm.DB
	jwtSecret string
	webauthn  *webauthn.WebAuthn
	sessions  map[string]*webauthn.SessionData
	sessionMu sync.RWMutex
}

func NewAuthHandler(db *gorm.DB, jwtSecret string) *AuthHandler {
	h := &AuthHandler{
		db:        db,
		jwtSecret: jwtSecret,
		sessions:  make(map[string]*webauthn.SessionData),
	}

	rpID := os.Getenv("RP_ID")
	if rpID == "" {
		rpID = "localhost"
	}

	origin := os.Getenv("RP_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3010"
	}

	w, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Benchmarking Platform",
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
	if err == nil {
		h.webauthn = w
	}

	return h
}

type RegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token                string            `json:"token"`
	ExpiresAt            time.Time         `json:"expires_at"`
	User                 UserResponse      `json:"user"`
	Workspace            *models.Workspace `json:"workspace,omitempty"`
	RequiresOrgSelection bool              `json:"requires_org_selection,omitempty"`
	RequiresInviteCode   bool              `json:"requires_invite_code,omitempty"`
	AvailableOrgs        []AuthOrgResponse `json:"available_orgs,omitempty"`
	RequiresTerms        bool              `json:"requires_terms,omitempty"`
}

type AuthOrgResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Role string    `json:"role"`
}

type UserResponse struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Email          string             `json:"email"`
	IsAdmin        bool               `json:"is_admin"`
	ImpersonatorID string             `json:"impersonator_id,omitempty"`
	OrganizationID string             `json:"organization_id,omitempty"`
	Workspaces     []models.Workspace `json:"workspaces,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
}

// Register creates a new user and their default workspace (no organizations)
func (h *AuthHandler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	req.Name = html.EscapeString(req.Name)
	req.Email = html.EscapeString(req.Email)

	if req.Email == "" || req.Password == "" || req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Name, email and password are required"})
	}

	// Check if email already exists
	var existing models.User
	if err := h.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Email already registered"})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to hash password"})
	}

	tx := h.db.Begin()

	userID := uuid.New()
	user := models.User{
		ID:           userID,
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
	}

	// Create default workspace for the user (no organization)
	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   "main",
	}

	if err := tx.Create(&workspace).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create workspace"})
	}

	// Create default client for the workspace
	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        "Default Client",
	}
	if err := tx.Create(&client).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create client"})
	}

	if err := tx.Commit().Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to finalize registration"})
	}

	seedExampleSetupIfFirstWorkspace(h.db, user.ID, workspace, client)

	workspace.User = user

	// Generate JWT (no organization)
	token, err := middleware.GenerateToken(user.ID.String(), workspace.ID.String(), "", user.Email, h.jwtSecret, "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
	}

	h.setTokenCookie(c, token)

	resp := AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		User:      h.mapUserToResponse(user),
		Workspace: &workspace,
	}
	resp.RequiresTerms = user.TermsAcceptedAt == nil

	return c.JSON(http.StatusCreated, resp)
}

func (h *AuthHandler) setTokenCookie(c echo.Context, token string) {
	cookie := new(http.Cookie)
	cookie.Name = "token"
	cookie.Value = token
	cookie.Expires = time.Now().UTC().Add(24 * time.Hour)
	cookie.HttpOnly = true
	cookie.Path = "/"
	cookie.SameSite = http.SameSiteLaxMode

	// Secure only when the request is actually HTTPS (direct or via proxy).
	if c.Request().TLS != nil || c.Scheme() == "https" || c.Request().Header.Get("X-Forwarded-Proto") == "https" {
		cookie.Secure = true
	}

	c.SetCookie(cookie)
}

// Login authenticates a user and returns a JWT
func (h *AuthHandler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	ip := c.RealIP()
	userAgent := c.Request().UserAgent()

	// Helper to record login log
	recordLog := func(userID *uuid.UUID, status, reason string, orgID *uuid.UUID) {
		logEntry := models.LoginLog{
			ID:             uuid.New(),
			UserID:         userID,
			UserEmail:      req.Email,
			IPAddress:      ip,
			UserAgent:      userAgent,
			Status:         status,
			FailureReason:  reason,
			OrganizationID: orgID,
			CreatedAt:      time.Now().UTC(),
		}
		// Log error but don't block auth flow
		if err := h.db.Create(&logEntry).Error; err != nil {
			fmt.Printf("[Auth] Failed to write login log: %v\n", err)
		}
	}

	if req.Email == "" || req.Password == "" {
		recordLog(nil, "failed", "missing_credentials", nil)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Email and password are required"})
	}

	// Find user by email
	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		recordLog(nil, "failed", "invalid_credentials", nil) // User not found, but we say invalid credentials
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid credentials"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		recordLog(&user.ID, "failed", "invalid_credentials", nil)
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid credentials"})
	}

	// Update last login
	now := time.Now().UTC()
	h.db.Model(&user).Update("last_login_at", &now)

	// Check if user is suspended globally
	if user.IsSuspended {
		recordLog(&user.ID, "failed", "user_suspended", nil)
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Please contact the administrator"})
	}

	// Get user's first workspace (no organization required)
	var workspace models.Workspace
	h.db.Preload("User").Where("user_id = ?", user.ID).First(&workspace)

	// If no workspace exists, create one
	if workspace.ID == uuid.Nil {
		workspace = models.Workspace{
			ID:     uuid.New(),
			UserID: user.ID,
			Name:   "main",
		}
		if err := h.db.Create(&workspace).Error; err != nil {
			recordLog(&user.ID, "failed", "workspace_creation_error", nil)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create workspace"})
		}

		// Create default client
		client := models.Client{
			ID:          uuid.New(),
			WorkspaceID: workspace.ID,
			Name:        "Default Client",
		}
		if err := h.db.Create(&client).Error; err != nil {
			// Non-fatal, continue — workspace is already created
			logger.Warn("[AUTH] Failed to create default client for workspace %s: %v", workspace.ID, err)
		}
	}

	workspace.User = user

	workspaceID := ""
	if workspace.ID != uuid.Nil {
		workspaceID = workspace.ID.String()
	}

	// Generate JWT (no organization)
	token, err := middleware.GenerateToken(user.ID.String(), workspaceID, "", user.Email, h.jwtSecret, "")
	if err != nil {
		recordLog(&user.ID, "failed", "token_generation_error", nil)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
	}

	h.setTokenCookie(c, token)

	recordLog(&user.ID, "success", "", nil)

	return c.JSON(http.StatusOK, AuthResponse{
		Token:         token,
		ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		User:          h.mapUserToResponse(user),
		Workspace:     &workspace,
		RequiresTerms: user.TermsAcceptedAt == nil,
	})
}

// RefreshToken renews the JWT cookie if the current one is valid
func (h *AuthHandler) RefreshToken(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	// Fetch user to get fresh email
	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "User not found"})
	}

	// Check if suspended
	if user.IsSuspended {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Account suspended"})
	}

	workspaceID := middleware.GetWorkspaceID(c)
	orgID := middleware.GetOrgID(c)
	impersonatorID := middleware.GetImpersonatorID(c)

	wsIDStr := ""
	if workspaceID != uuid.Nil {
		wsIDStr = workspaceID.String()
	}

	orgIDStr := ""
	if orgID != uuid.Nil {
		orgIDStr = orgID.String()
	}

	impIDStr := ""
	if impersonatorID != uuid.Nil {
		impIDStr = impersonatorID.String()
	}

	token, err := middleware.GenerateToken(
		user.ID.String(),
		wsIDStr,
		orgIDStr,
		user.Email,
		h.jwtSecret,
		impIDStr,
	)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
	}

	h.setTokenCookie(c, token)

	return c.JSON(http.StatusOK, map[string]any{
		"message":    "Token refreshed",
		"expires_at": time.Now().UTC().Add(24 * time.Hour),
	})
}

// Logout clears the JWT cookie
func (h *AuthHandler) Logout(c echo.Context) error {
	cookie := new(http.Cookie)
	cookie.Name = "token"
	cookie.Value = ""
	cookie.Expires = time.Now().UTC().Add(-1 * time.Hour)
	cookie.HttpOnly = true
	cookie.Path = "/"
	c.SetCookie(cookie)
	return c.JSON(http.StatusOK, map[string]string{"message": "Logged out"})
}

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

// ============ ADMIN ENDPOINTS ============

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
		user.Name = *req.Name
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.IsAdmin != nil {
		user.IsAdmin = *req.IsAdmin
	}
	if req.Password != nil && *req.Password != "" {
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

// BootstrapAdmin creates the first admin user if no admin exists
func (h *AuthHandler) BootstrapAdmin(c echo.Context) error {
	// Check if any admin exists
	var count int64
	models.AdminScope(h.db.Model(&models.User{})).Count(&count)
	if count > 0 {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin already exists"})
	}

	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if req.Email == "" || req.Name == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Name, email and password are required"})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to hash password"})
	}

	// Start transaction
	tx := h.db.Begin()
	if tx.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
	}

	userID := uuid.New()

	user := models.User{
		ID:           userID,
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		IsAdmin:      true,
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		fmt.Printf("Bootstrap Error (User): %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create admin"})
	}

	// Create default workspace (no organization)
	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   "main",
	}
	if err := tx.Create(&workspace).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create workspace"})
	}

	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        "Default Client",
	}
	if err := tx.Create(&client).Error; err != nil {
		tx.Rollback()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create client"})
	}

	if err := tx.Commit().Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
	}

	token, err := middleware.GenerateToken(user.ID.String(), workspace.ID.String(), "", user.Email, h.jwtSecret, "")
	if err != nil {
		logger.Error("[AUTH] Failed to generate token after registration for user %s: %v", user.ID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate authentication token"})
	}

	return c.JSON(http.StatusCreated, AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		User:      h.mapUserToResponse(user),
		Workspace: &workspace,
	})
}

// CheckAdminExists checks if any admin user exists
func (h *AuthHandler) CheckAdminExists(c echo.Context) error {
	var count int64
	if err := models.AdminScope(h.db.Model(&models.User{})).Count(&count).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to check admin status"})
	}
	return c.JSON(http.StatusOK, map[string]bool{"exists": count > 0})
}

func (h *AuthHandler) mapUserToResponse(u models.User) UserResponse {
	// Note: We don't populate OrganizationID here anymore as user can have multiple.
	// Callers should populate it from context if needed.

	var workspaces []models.Workspace
	h.db.Model(&u).Association("Workspaces").Find(&workspaces)

	return UserResponse{
		ID:         u.ID.String(),
		Name:       u.Name,
		Email:      u.Email,
		IsAdmin:    u.HasAdminAccess(),
		Workspaces: workspaces,
		CreatedAt:  u.CreatedAt,
	}
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

// WebAuthn Login Begin
func (h *AuthHandler) WebAuthnLoginBegin(c echo.Context) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "email = ?", req.Email).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	options, sessionData, err := h.webauthn.BeginLogin(user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to begin login: " + err.Error()})
	}

	sessionID := uuid.New().String()
	h.sessionMu.Lock()
	h.sessions[sessionID] = sessionData
	h.sessionMu.Unlock()

	// Store sessionID in a short-lived cookie
	cookie := new(http.Cookie)
	cookie.Name = "webauthn_session"
	cookie.Value = sessionID
	cookie.Expires = time.Now().Add(5 * time.Minute)
	cookie.HttpOnly = true
	cookie.Path = "/"
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, options)
}

// WebAuthn Login Finish
func (h *AuthHandler) WebAuthnLoginFinish(c echo.Context) error {
	var req struct {
		Email    string          `json:"email"`
		Response json.RawMessage `json:"response"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	cookie, err := c.Cookie("webauthn_session")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session cookie missing"})
	}

	h.sessionMu.RLock()
	sessionData, ok := h.sessions[cookie.Value]
	h.sessionMu.RUnlock()

	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session not found or expired"})
	}

	h.sessionMu.Lock()
	delete(h.sessions, cookie.Value)
	h.sessionMu.Unlock()

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "email = ?", req.Email).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	fakeReq, err := http.NewRequest("POST", "/", bytes.NewReader(req.Response))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to build WebAuthn request"})
	}
	fakeReq.Header.Set("Content-Type", "application/json")

	credential, err := h.webauthn.FinishLogin(user, *sessionData, fakeReq)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Failed to finish login: " + err.Error()})
	}

	// Update sign count
	for i, pk := range user.Passkeys {
		if bytes.Equal(pk.CredentialID, credential.ID) {
			h.db.Model(&user.Passkeys[i]).Updates(map[string]any{
				"sign_count":      credential.Authenticator.SignCount,
				"backup_eligible": credential.Flags.BackupEligible,
				"backup_state":    credential.Flags.BackupState,
			})
			break
		}
	}

	now := time.Now().UTC()
	h.db.Model(&user).Update("last_login_at", &now)

	var workspace models.Workspace
	h.db.Preload("User").Where("user_id = ?", user.ID).First(&workspace)

	token, err := middleware.GenerateToken(
		user.ID.String(),
		workspace.ID.String(),
		"", // No organization
		user.Email,
		h.jwtSecret,
		"",
	)
	if err != nil {
		logger.Error("[AUTH] Failed to generate token after passkey login for user %s: %v", user.ID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate authentication token"})
	}

	h.setTokenCookie(c, token)

	return c.JSON(http.StatusOK, AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		User:      h.mapUserToResponse(user),
		Workspace: &workspace,
	})
}

// WebAuthn Register Begin (Protected)
func (h *AuthHandler) WebAuthnRegisterBegin(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	options, sessionData, err := h.webauthn.BeginRegistration(user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to begin registration: " + err.Error()})
	}

	sessionID := uuid.New().String()
	h.sessionMu.Lock()
	h.sessions[sessionID] = sessionData
	h.sessionMu.Unlock()

	cookie := new(http.Cookie)
	cookie.Name = "webauthn_reg_session"
	cookie.Value = sessionID
	cookie.Expires = time.Now().Add(5 * time.Minute)
	cookie.HttpOnly = true
	cookie.Path = "/"
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, options)
}

// WebAuthn Register Finish (Protected)
func (h *AuthHandler) WebAuthnRegisterFinish(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}

	var req struct {
		Response json.RawMessage `json:"response"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	cookie, err := c.Cookie("webauthn_reg_session")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session cookie missing"})
	}

	h.sessionMu.RLock()
	sessionData, ok := h.sessions[cookie.Value]
	h.sessionMu.RUnlock()

	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Session not found or expired"})
	}

	h.sessionMu.Lock()
	delete(h.sessions, cookie.Value)
	h.sessionMu.Unlock()

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "id = ?", userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	fakeReq, err := http.NewRequest("POST", "/", bytes.NewReader(req.Response))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to build WebAuthn request"})
	}
	fakeReq.Header.Set("Content-Type", "application/json")

	credential, err := h.webauthn.FinishRegistration(user, *sessionData, fakeReq)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Failed to finish registration: " + err.Error()})
	}

	passkey := models.Passkey{
		ID:             uuid.New(),
		UserID:         user.ID,
		CredentialID:   credential.ID,
		PublicKey:      credential.PublicKey,
		Attestation:    credential.AttestationType,
		SignCount:      credential.Authenticator.SignCount,
		BackupEligible: credential.Flags.BackupEligible,
		BackupState:    credential.Flags.BackupState,
		CreatedAt:      time.Now(),
	}

	if err := h.db.Create(&passkey).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save passkey"})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Passkey registered successfully",
	})
}
