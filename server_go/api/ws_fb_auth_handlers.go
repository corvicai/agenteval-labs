package api

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"

	"github.com/google/uuid"
)

func (h *Hub) handleAuth(c *Connection, env models.Envelope) {
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
		log.Printf("[FIREBASE] Token verification failed: %v", err)
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
			CreatedAt:    time.Now(),
		}

		if err := h.db.Create(&user).Error; err != nil {
			log.Printf("[FIREBASE] Error creating user: %v", err)
			c.SendError(env.CorrelationID, "failed to create user record")
			return
		}
	} else if user.FirebaseUID == "" {
		// Update FirebaseUID if user was found by email but didn't have UID linked
		h.db.Model(&user).Update("firebase_uid", uid)
	}

	// Get user's active organization and workspace
	var workspace models.Workspace
	err = h.db.Preload("User").Preload("Organization").
		Joins("JOIN user_organizations ON user_organizations.organization_id = workspaces.organization_id").
		Where("workspaces.user_id = ?", user.ID).
		Order("workspaces.created_at DESC").
		First(&workspace).Error

	requiresOnboarding := false
	if err != nil {
		// User has no workspace -> Needs Onboarding
		requiresOnboarding = true
		log.Printf("[FIREBASE] User %s has no workspace, requiring onboarding.", user.Email)
	} else {
		// Update connection with authenticated info
		c.UserID = user.ID
		c.OrgID = workspace.OrganizationID
		c.WorkspaceID = workspace.ID
		c.IsAuthenticated = true
	}

	// Generate internal JWT (Workspace/Org might be empty if onboarding needed)
	token, _ := middleware.GenerateToken(
		user.ID.String(),
		workspace.ID.String(),              // Empty if onboarding
		workspace.Organization.ID.String(), // Empty if onboarding (via workspace relation)
		user.Email,
		h.jwtSecret,
		"",
	)

	// Update last login
	now := time.Now()
	h.db.Model(&user).Update("last_login_at", &now)

	log.Printf("[FIREBASE] User authenticated: %s (%s) - Onboarding: %v", user.Email, user.ID, requiresOnboarding)

	response := map[string]any{
		"success":             true,
		"token":               token,
		"requires_onboarding": requiresOnboarding,
		"user": map[string]any{
			"id":       user.ID.String(),
			"name":     user.Name,
			"email":    user.Email,
			"is_admin": user.IsAdmin,
		},
	}

	if !requiresOnboarding {
		response["organization"] = map[string]any{
			"id":   workspace.Organization.ID.String(),
			"name": workspace.Organization.Name,
		}
		response["workspace"] = workspace
	}

	c.SendResponse(DataWsLoginResult, env.CorrelationID, response)
}
