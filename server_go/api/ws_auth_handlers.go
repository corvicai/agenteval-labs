package api

import (
	"encoding/json"
	"fmt"
	"time"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func (h *Hub) handleCheckManagerStatus(c *Connection, env models.Envelope) {
	if c.UserID == uuid.Nil {
		c.SendResponse(DataManagerStatus, env.CorrelationID, map[string]any{"is_manager": false})
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil {
		c.SendResponse(DataManagerStatus, env.CorrelationID, map[string]any{"is_manager": false})
		return
	}

	// Manager concept removed - no organizations
	// Admins can still be considered managers if needed, but no org context
	c.SendResponse(DataManagerStatus, env.CorrelationID, map[string]any{
		"is_manager": user.IsAdmin,
	})
}

func (h *Hub) handleGetMe(c *Connection, env models.Envelope) {
	if c.UserID == uuid.Nil {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}

	// Get user's workspaces (no organization filter)
	var workspaces []models.Workspace
	h.db.Where("user_id = ?", c.UserID).Find(&workspaces)

	// Fix GORM recursion zeroing
	safeUser := user
	safeUser.Workspaces = nil
	safeUser.Organizations = nil
	safeUser.UserOrgs = nil

	for i := range workspaces {
		workspaces[i].User = safeUser
	}

	result := map[string]any{
		"user": map[string]any{
			"id":         user.ID.String(),
			"name":       user.Name,
			"email":      user.Email,
			"is_admin":   user.HasAdminAccess(),
			"created_at": user.CreatedAt,
			"workspaces": workspaces,
		},
	}

	c.SendResponse(DataMe, env.CorrelationID, result)
}

func (h *Hub) handleCheckAdminExists(c *Connection, env models.Envelope) {
	var count int64
	if err := models.AdminScope(h.db.Model(&models.User{})).Count(&count).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to check admin status")
		return
	}
	c.SendResponse(DataCheckAdminExists, env.CorrelationID, map[string]bool{"exists": count > 0})
}

func (h *Hub) handleWsLogin(c *Connection, env models.Envelope) {
	var req models.WsLoginPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	if req.Email == "" || req.Password == "" {
		c.SendError(env.CorrelationID, "email and password are required")
		return
	}

	// Helper to record login log via WebSocket
	recordLog := func(userID *uuid.UUID, status, reason string, orgID *uuid.UUID) {
		logEntry := models.LoginLog{
			ID:             uuid.New(),
			UserID:         userID,
			UserEmail:      req.Email,
			IPAddress:      c.RemoteIP,
			UserAgent:      c.Conn.RemoteAddr().String(), // Best we can do for WS UserAgent if not in req
			Status:         status,
			FailureReason:  reason,
			OrganizationID: orgID,
			CreatedAt:      time.Now().UTC(),
		}
		if err := h.db.Create(&logEntry).Error; err != nil {
			logger.Error("[LOGIN_LOG] Failed to create log entry: %v", err)
		}
	}

	// Find user by email
	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		recordLog(nil, "failed", "invalid_credentials", nil)
		c.SendError(env.CorrelationID, "invalid credentials")
		return
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		recordLog(&user.ID, "failed", "invalid_credentials", nil)
		c.SendError(env.CorrelationID, "invalid credentials")
		return
	}

	// Update last login
	now := time.Now().UTC()
	h.db.Model(&user).Update("last_login_at", &now)

	// Check if user is suspended
	if user.IsSuspended {
		recordLog(&user.ID, "failed", "user_suspended", nil)
		c.SendError(env.CorrelationID, "account is suspended")
		return
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
			c.SendError(env.CorrelationID, "failed to create workspace")
			return
		}

		// Create default client
		client := models.Client{
			ID:          uuid.New(),
			WorkspaceID: workspace.ID,
			Name:        "Default Client",
		}
		h.db.Create(&client) // Non-fatal
	}

	workspace.User = user

	// Update connection with authenticated info
	c.UserID = user.ID
	c.WorkspaceID = workspace.ID
	c.IsAuthenticated = true

	// Generate token for persistence (no organization)
	token, err := middleware.GenerateToken(
		user.ID.String(),
		workspace.ID.String(),
		"", // No organization
		user.Email,
		h.jwtSecret,
		"",
	)
	if err != nil {
		logger.Error("[AUTH] Failed to generate token for user %s: %v", user.ID, err)
		recordLog(&user.ID, "failed", "token_generation_error", nil)
		c.SendError(env.CorrelationID, "failed to generate authentication token")
		return
	}

	recordLog(&user.ID, "success", "", nil)

	c.SendResponse(DataWsLoginResult, env.CorrelationID, map[string]any{
		"success": true,
		"token":   token,
		"user": map[string]any{
			"id":       user.ID.String(),
			"name":     user.Name,
			"email":    user.Email,
			"is_admin": user.HasAdminAccess(),
		},
		"workspace": workspace,
	})
}

// handleWsRegister handles user registration via invite code
func (h *Hub) handleWsRegister(c *Connection, env models.Envelope) {
	var req struct {
		Name             string `json:"name"`
		Email            string `json:"email"`
		Password         string `json:"password"`
		InviteCode       string `json:"invite_code"`
		OrganizationName string `json:"organization_name"`
		Role             string `json:"role"` // 'manager' or 'user'
	}
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	// 1. Validate Invite Code if provided OR if registering as common user
	var invite models.InviteCode
	var role = req.Role
	if role == "" {
		role = "user"
	}

	if role == "user" || req.InviteCode != "" {
		if req.InviteCode == "" {
			c.SendError(env.CorrelationID, "invite code is required to join an organization")
			return
		}

		if err := h.db.First(&invite, "code = ?", req.InviteCode).Error; err != nil {
			c.SendError(env.CorrelationID, "invalid invite code")
			return
		}

		if invite.UseCount >= invite.MaxUses {
			c.SendError(env.CorrelationID, "invite code usage limit reached")
			return
		}

		if time.Now().UTC().After(invite.ExpiresAt) {
			c.SendError(env.CorrelationID, "invite code expired")
			return
		}
	}

	// 2. Validate Email Uniqueness
	var existing models.User
	if err := h.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		c.SendError(env.CorrelationID, "email already registered")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("[AUTH] Failed to hash password during registration for %s: %v", req.Email, err)
		c.SendError(env.CorrelationID, "failed to hash password")
		return
	}
	user := models.User{
		ID:           uuid.New(),
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
	}

	tx := h.db.Begin()

	// 3. Handle Organization Linking/Creation
	var orgID uuid.UUID

	if role == "manager" && req.InviteCode == "" {
		// Public Manager Registration (New Org)
		if req.OrganizationName == "" {
			tx.Rollback()
			c.SendError(env.CorrelationID, "organization name is required for new organization")
			return
		}

		org := models.Organization{
			ID:        uuid.New(),
			Name:      req.OrganizationName,
			ManagerID: &user.ID,
		}
		if err := tx.Create(&org).Error; err != nil {
			tx.Rollback()
			c.SendError(env.CorrelationID, "failed to create organization. It might already exist.")
			return
		}
		orgID = org.ID
	} else if req.InviteCode != "" {
		// Registration via Invite
		if invite.IsNewOrg {
			if req.OrganizationName == "" {
				tx.Rollback()
				c.SendError(env.CorrelationID, "organization name is required for new organization invite")
				return
			}

			// Create New Org
			org := models.Organization{
				ID:        uuid.New(),
				Name:      req.OrganizationName,
				ManagerID: &user.ID,
			}
			if err := tx.Create(&org).Error; err != nil {
				tx.Rollback()
				c.SendError(env.CorrelationID, "failed to create organization")
				return
			}
			orgID = org.ID
			role = "manager" // Force manager role for new org creator
		} else {
			// Link to Existing Org
			if invite.OrganizationID == nil {
				tx.Rollback()
				c.SendError(env.CorrelationID, "invalid invite configuration")
				return
			}
			orgID = *invite.OrganizationID
			role = invite.Role
		}
	} else {
		tx.Rollback()
		c.SendError(env.CorrelationID, "common users must use an invite code")
		return
	}

	// Create User
	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to create user")
		return
	}

	// Link User to Org
	uo := models.UserOrganization{
		UserID:          user.ID,
		OrganizationID:  orgID,
		Role:            role,
		InvitedByUserID: &invite.CreatedBy,
		JoinedAt:        time.Now().UTC(),
	}
	if err := tx.Create(&uo).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to link user to organization")
		return
	}

	// Create Default Workspace
	ws := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   "main",
	}
	if err := tx.Create(&ws).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to create workspace")
		return
	}

	// 4. Mark Invite as Used
	invite.UseCount++
	if err := tx.Save(&invite).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to update invite status")
		return
	}

	// Record usage
	usage := models.InviteCodeUsage{
		ID:     uuid.New(),
		Code:   invite.Code,
		UserID: user.ID,
		UsedAt: time.Now().UTC(),
	}
	if err := tx.Create(&usage).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to record invite usage")
		return
	}

	tx.Commit()

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"success": true,
		"user":    user,
		"org_id":  orgID,
	})
}

func (h *Hub) handleWsBootstrapAdmin(c *Connection, env models.Envelope) {
	var req struct {
		Name             string `json:"name"`
		Email            string `json:"email"`
		Password         string `json:"password"`
		OrganizationName string `json:"organization_name"`
	}
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("[AUTH] Failed to hash password during bootstrap admin: %v", err)
		c.SendError(env.CorrelationID, "failed to hash password")
		return
	}
	user := models.User{
		ID:           uuid.New(),
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		IsAdmin:      true,
	}

	org := models.Organization{
		ID:        uuid.New(),
		Name:      req.OrganizationName,
		ManagerID: &user.ID,
	}

	tx := h.db.Begin()
	tx.Create(&user)
	tx.Create(&org)

	uo := models.UserOrganization{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Role:           "manager",
	}
	tx.Create(&uo)

	ws := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   "main",
	}
	tx.Create(&ws)

	tx.Commit()

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"success": true,
		"user":    user,
	})
}

func (h *Hub) handleJoinOrganization(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var req struct {
		InviteCode string `json:"invite_code"`
	}
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	if req.InviteCode == "" {
		c.SendError(env.CorrelationID, "invite code is required")
		return
	}

	tx := h.db.Begin()

	var invite models.InviteCode
	if err := tx.Where("code = ?", req.InviteCode).First(&invite).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "invalid invite code")
		return
	}

	if invite.UseCount >= invite.MaxUses {
		tx.Rollback()
		c.SendError(env.CorrelationID, "invite code usage limit reached")
		return
	}

	if invite.ExpiresAt.Before(time.Now().UTC()) {
		tx.Rollback()
		c.SendError(env.CorrelationID, "invite code has expired")
		return
	}

	if invite.OrganizationID == nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "invalid invite code (no organization associated)")
		return
	}

	// Check if user is already a member
	var existingUO models.UserOrganization
	if err := tx.Where("user_id = ? AND organization_id = ?", c.UserID, *invite.OrganizationID).First(&existingUO).Error; err == nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "you are already a member of this organization")
		return
	}

	// Add user to organization
	userOrg := models.UserOrganization{
		UserID:          c.UserID,
		OrganizationID:  *invite.OrganizationID,
		Role:            invite.Role,
		InvitedByUserID: &invite.CreatedBy,
		JoinedAt:        time.Now().UTC(),
	}
	if err := tx.Create(&userOrg).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to join organization")
		return
	}

	// Create default workspace for user
	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: c.UserID,
		Name:   "main",
	}
	if err := tx.Create(&workspace).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to create workspace")
		return
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
		c.SendError(env.CorrelationID, "failed to update invite status")
		return
	}

	// Record usage
	usage := models.InviteCodeUsage{
		ID:     uuid.New(),
		Code:   invite.Code,
		UserID: c.UserID,
		UsedAt: time.Now().UTC(),
	}
	if err := tx.Create(&usage).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to record invite usage")
		return
	}

	tx.Commit()

	// Get user for response
	var user models.User
	h.db.Preload("Organizations").First(&user, c.UserID)

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"success":   true,
		"user":      user,
		"workspace": workspace,
	})
}

// handleWsChangePassword allows users to change their own password or admins
// to reset others. Admin actions are recorded via logger.Info for audit.
func (h *Hub) handleWsChangePassword(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var req models.ChangePasswordPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	targetUserID := c.UserID
	isSelfChange := true

	if req.ID != "" && req.ID != c.UserID.String() {
		// Admin changing another user's password
		var currentUser models.User
		h.db.First(&currentUser, "id = ?", c.UserID)
		if !currentUser.IsAdmin {
			logger.Warn("[SECURITY] Non-admin user %s attempted to change password for user %s", c.UserID, req.ID)
			c.SendError(env.CorrelationID, "admin access required to change other users' passwords")
			return
		}
		uid, err := uuid.Parse(req.ID)
		if err != nil {
			c.SendError(env.CorrelationID, "invalid target user ID")
			return
		}
		targetUserID = uid
		isSelfChange = false
	}

	// 1. Validate Password Complexity
	pass := req.NewPassword
	if len(pass) < 8 {
		c.SendError(env.CorrelationID, "password must be at least 8 characters long")
		return
	}

	hasUpper := false
	hasSpecialOrNum := false
	for _, char := range pass {
		if char >= 'A' && char <= 'Z' {
			hasUpper = true
		}
		if (char >= '0' && char <= '9') || (char < 'A' || char > 'z') && char != ' ' {
			hasSpecialOrNum = true
		}
	}

	if !hasUpper {
		c.SendError(env.CorrelationID, "password must contain at least one uppercase letter")
		return
	}
	if !hasSpecialOrNum {
		c.SendError(env.CorrelationID, "password must contain at least one number or special character")
		return
	}

	// 2. Fetch User and Verify Old Password if self-change
	var user models.User
	if err := h.db.First(&user, "id = ?", targetUserID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}

	if isSelfChange {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
			c.SendError(env.CorrelationID, "incorrect current password")
			return
		}
	}

	// 3. Hash and Save
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("[SECURITY] Failed to hash new password for user %s: %v", targetUserID, err)
		c.SendError(env.CorrelationID, "failed to hash password")
		return
	}
	user.PasswordHash = string(hashedPassword)
	if err := h.db.Save(&user).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to update password")
		return
	}

	if !isSelfChange {
		logger.Info("[SECURITY] Admin %s changed password for user %s (%s)", c.UserID, targetUserID, user.Email)
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "password updated successfully"})
}

// Helper to verify manager status and get org ID for WebSocket connection
func (h *Hub) verifyManager(c *Connection) (*uuid.UUID, error) {
	if !c.IsAuthenticated {
		return nil, fmt.Errorf("not authenticated")
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil {
		return nil, fmt.Errorf("user not found")
	}

	if user.IsAdmin {
		return &c.OrgID, nil
	}

	var userOrg models.UserOrganization
	if err := h.db.First(&userOrg, "user_id = ? AND organization_id = ?", c.UserID, c.OrgID).Error; err != nil {
		return nil, fmt.Errorf("user is not a member of this organization")
	}

	if userOrg.Role != "manager" {
		return nil, fmt.Errorf("not a manager")
	}

	return &c.OrgID, nil
}

// handleCreateOrganization allows an authenticated user (even without an org) to create a new one
func (h *Hub) handleCreateOrganization(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	if req.Name == "" {
		c.SendError(env.CorrelationID, "organization name is required")
		return
	}

	tx := h.db.Begin()

	// 1. Create Organization
	org := models.Organization{
		ID:        uuid.New(),
		Name:      req.Name,
		ManagerID: &c.UserID,
		CreatedAt: time.Now().UTC(),
	}
	if err := tx.Create(&org).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to create organization (name might be taken)")
		return
	}

	// 2. Link User to Org
	uo := models.UserOrganization{
		UserID:         c.UserID,
		OrganizationID: org.ID,
		Role:           "manager",
		JoinedAt:       time.Now().UTC(),
	}
	if err := tx.Create(&uo).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to link user to organization")
		return
	}

	// 3. Create Default Workspace
	ws := models.Workspace{
		ID:        uuid.New(),
		UserID:    c.UserID,
		Name:      "main",
		CreatedAt: time.Now().UTC(),
	}
	if err := tx.Create(&ws).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "failed to create default workspace")
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.SendError(env.CorrelationID, "failed to finalize creation")
		return
	}

	// 4. Update Connection State
	c.OrgID = org.ID
	c.WorkspaceID = ws.ID

	// 5. Generate Full Token
	var user models.User
	h.db.First(&user, c.UserID)
	token, err := middleware.GenerateToken(
		user.ID.String(),
		ws.ID.String(),
		org.ID.String(),
		user.Email,
		h.jwtSecret,
		"",
	)
	if err != nil {
		logger.Error("[AUTH] Failed to generate token after org select for user %s: %v", user.ID, err)
		c.SendError(env.CorrelationID, "failed to generate authentication token")
		return
	}

	c.SendResponse(DataWsLoginResult, env.CorrelationID, map[string]any{
		"success": true,
		"token":   token,
		"user": map[string]any{
			"id":       user.ID.String(),
			"name":     user.Name,
			"email":    user.Email,
			"is_admin": user.HasAdminAccess(),
		},
		"organization": map[string]any{
			"id":   org.ID.String(),
			"name": org.Name,
		},
		"workspace": ws,
	})
}
