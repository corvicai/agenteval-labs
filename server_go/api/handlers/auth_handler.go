package handlers

import (
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
	"benchmarking-platform/internal/validation"
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

	if err := validation.ValidateUserName(req.Name); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := validation.ValidateEmail(req.Email); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := validation.ValidatePassword(req.Password); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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
