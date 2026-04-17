package api

import (
	"context"
	"encoding/json"
	"time"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"

	"github.com/google/uuid"
)

func (h *Hub) handleAuth(c *Connection, env models.Envelope) {
	// Reset connection state to prevent session bleeding from previous user on same connection
	c.UserID = uuid.Nil
	c.OrgID = uuid.Nil
	c.WorkspaceID = uuid.Nil
	c.IsAuthenticated = false

	if h.Firebase == nil {
		c.SendError(env.CorrelationID, "Firebase auth is not configured on the server")
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: expected { \"token\": \"...\" }")
		return
	}

	if req.Token == "" {
		c.SendError(env.CorrelationID, "token is required")
		return
	}

	// Verify Firebase Token
	fbToken, err := h.Firebase.VerifyIDToken(context.Background(), req.Token)
	if err != nil {
		logger.Error("[FIREBASE] Token verification failed: %v", err)
		c.SendError(env.CorrelationID, "invalid or expired firebase token")
		return
	}

	uid := fbToken.UID
	email, _ := fbToken.Claims["email"].(string)
	name, _ := fbToken.Claims["name"].(string)
	if name == "" {
		name = email
	}

	// Find or Create User
	var user models.User
	// Try finding by FirebaseUID first, then by Email
	err = h.db.Where("firebase_uid = ?", uid).Or("email = ?", email).First(&user).Error

	if err != nil {
		// Create new user if not found
		user = models.User{
			ID:           uuid.New(),
			FirebaseUID:  uid,
			Email:        email,
			Name:         name,
			PasswordHash: "EXTERNAL_AUTH", // Should not be used for comparison
			CreatedAt:    time.Now().UTC(),
		}

		if err := h.db.Create(&user).Error; err != nil {
			logger.Error("[FIREBASE] Error creating user: %v", err)
			c.SendError(env.CorrelationID, "failed to create user record")
			return
		}
	} else if user.FirebaseUID == "" {
		// Update FirebaseUID if user was found by email but didn't have UID linked
		h.db.Model(&user).Update("firebase_uid", uid)
	}

	requiresTerms := user.TermsAcceptedAt == nil
	// Special Case: If it's a new user just created, we might consider them as needing ToS
	// unless we auto-accept (not requested). User wants to show terms if not signed.

	// Get user's workspace
	var workspace models.Workspace
	err = h.db.Preload("User").
		Where("user_id = ?", user.ID).
		Order("created_at DESC").
		First(&workspace).Error

	requiresOnboarding := false
	if err != nil {
		// User has no workspace -> Needs Onboarding
		requiresOnboarding = true
		logger.Info("[FIREBASE] User %s has no workspace, requiring onboarding.", user.Email)
	}

	// ALWAYS update connection with authenticated info if we have a user
	c.UserID = user.ID
	c.IsAuthenticated = true

	if !requiresOnboarding {
		c.OrgID = uuid.Nil // No organizations
		c.WorkspaceID = workspace.ID
	}

	// Generate internal JWT (Workspace/Org might be empty if onboarding needed)
	token, err := middleware.GenerateToken(
		user.ID.String(),
		workspace.ID.String(), // Empty if onboarding
		"",                    // No organization
		user.Email,
		h.jwtSecret,
		"",
	)
	if err != nil {
		logger.Error("[FB_AUTH] Failed to generate token for user %s: %v", user.ID, err)
		c.SendError(env.CorrelationID, "failed to generate authentication token")
		return
	}

	// Update last login
	now := time.Now().UTC()
	h.db.Model(&user).Update("last_login_at", &now)

	// Record Login Log
	var orgID *uuid.UUID // No organizations

	logEntry := models.LoginLog{
		ID:             uuid.New(),
		UserID:         &user.ID,
		UserEmail:      user.Email,
		IPAddress:      c.RemoteIP,
		UserAgent:      c.Conn.RemoteAddr().String(), // Best effort for WS
		Status:         "success",
		FailureReason:  "firebase_oauth", // Marking as oauth for clarity
		OrganizationID: orgID,
		CreatedAt:      time.Now().UTC(),
	}
	if err := h.db.Create(&logEntry).Error; err != nil {
		logger.Error("[LOGIN_LOG] Failed to create log entry (OAuth): %v", err)
	}

	logger.Info("[FIREBASE] User authenticated: %s (%s) - Onboarding: %v", user.Email, user.ID, requiresOnboarding)

	response := map[string]any{
		"success":             true,
		"token":               token,
		"requires_onboarding": requiresOnboarding,
		"requires_terms":      requiresTerms,
		"user": map[string]any{
			"id":       user.ID.String(),
			"name":     user.Name,
			"email":    user.Email,
			"is_admin": user.HasAdminAccess(),
		},
	}

	if !requiresOnboarding {
		// Organizations removed - no organization data to include
		response["workspace"] = workspace
	}

	c.SendResponse(DataWsLoginResult, env.CorrelationID, response)
}

func (h *Hub) handleAcceptTerms(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "authentication required")
		return
	}

	now := time.Now().UTC()
	if err := h.db.Model(&models.User{}).Where("id = ?", c.UserID).Update("terms_accepted_at", &now).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to update terms acceptance")
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "success"})
}
