package api

import (
	"context"
	"encoding/json"
	"time"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func (h *Hub) handleAdminGetUsers(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	// Check if current user is admin
	var currentUser models.User
	if err := h.db.First(&currentUser, "id = ?", c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}
	if !currentUser.IsAdmin {
		c.SendError(env.CorrelationID, "admin access required")
		return
	}

	// Parse filter if present
	var filter models.AdminFilterPayload
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &filter); err != nil {
			logger.Warn("[ADMIN] Failed to parse users filter payload: %v", err)
		}
	}

	query := h.db.Model(&models.User{})
	if filter.TimeRange != "" {
		threshold := time.Now().UTC()
		switch filter.TimeRange {
		case "24h":
			threshold = threshold.Add(-24 * time.Hour)
		case "3d":
			threshold = threshold.Add(-3 * 24 * time.Hour)
		case "1w":
			threshold = threshold.Add(-7 * 24 * time.Hour)
		}
		query = query.Where("created_at >= ?", threshold)
	}

	var users []models.User
	query.Preload("Workspaces.User").
		Preload("InvitedBy").
		Order("created_at DESC").
		Find(&users)

	// Map to response format
	result := make([]map[string]any, len(users))
	for i, u := range users {
		// Fix for zeroed User structs in Workspaces due to GORM recursion prevention
		// Prepare safe User (no cycles)
		safeUser := u
		safeUser.Workspaces = nil
		safeUser.Organizations = nil
		safeUser.UserOrgs = nil

		for j := range u.Workspaces {
			// Fix User
			u.Workspaces[j].User = safeUser
		}

		inviterName := ""
		if u.InvitedBy != nil {
			inviterName = u.InvitedBy.Name
		}

		result[i] = map[string]any{
			"id":              u.ID.String(),
			"name":            u.Name,
			"email":           u.Email,
			"is_admin":        u.HasAdminAccess(),
			"is_suspended":    u.IsSuspended,
			"created_at":      u.CreatedAt,
			"last_login_at":   u.LastLoginAt,
			"workspaces":      u.Workspaces,
			"invited_by_name": inviterName, // Safe access
		}
	}

	c.SendResponse(DataAdminUsers, env.CorrelationID, result)
}

func (h *Hub) handleAdminGetUserProfile(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	// Allow if current user is admin OR if they are requesting their own profile
	var currentUser models.User
	if err := h.db.First(&currentUser, "id = ?", c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}

	var req models.AdminProfilePayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	userID, err := uuid.Parse(req.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid user ID")
		return
	}

	if !currentUser.IsAdmin && currentUser.ID != userID {
		c.SendError(env.CorrelationID, "access denied: you can only view your own profile")
		return
	}

	var user models.User
	if err := h.db.Preload("Passkeys").First(&user, "id = ?", userID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}

	// Load all organizations
	var userOrgs []models.UserOrganization
	h.db.Where("user_id = ?", userID).Find(&userOrgs)

	var organizations []map[string]any
	isManagerGlobal := false

	for _, uo := range userOrgs {
		var o models.Organization
		if err := h.db.First(&o, "id = ?", uo.OrganizationID).Error; err == nil {
			isManager := (o.ManagerID != nil && *o.ManagerID == userID) || uo.Role == "manager"
			if isManager {
				isManagerGlobal = true
			}

			organizations = append(organizations, map[string]any{
				"id":           o.ID,
				"name":         o.Name,
				"created_at":   o.CreatedAt,
				"is_suspended": o.IsSuspended,
				"role":         uo.Role,
				"is_manager":   isManager,
			})
		}
	}

	// Get workspaces with agent count
	var wsList []models.Workspace
	h.db.Preload("User").Where("user_id = ?", userID).Find(&wsList)

	workspaces := make([]map[string]any, len(wsList))
	for i, w := range wsList {
		var cnt int64
		h.db.Model(&models.Agent{}).Where("workspace_id = ?", w.ID).Count(&cnt)

		workspaces[i] = map[string]any{
			"id":          w.ID,
			"name":        w.Name,
			"created_at":  w.CreatedAt,
			"user":        w.User,
			"agent_count": cnt,
		}
	}

	result := map[string]any{
		"id":            user.ID.String(),
		"name":          user.Name,
		"email":         user.Email,
		"is_admin":      user.HasAdminAccess(),
		"is_manager":    isManagerGlobal,
		"organizations": organizations,
		"workspaces":    workspaces,
		"created_at":    user.CreatedAt,
		"last_login_at": user.LastLoginAt,
		"passkeys":      user.Passkeys,
	}

	c.SendResponse(DataAdminUserProfile, env.CorrelationID, result)
}

func (h *Hub) handleAdminCreateUser(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	// Check if current user is admin
	var currentUser models.User
	if err := h.db.First(&currentUser, "id = ?", c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}
	if !currentUser.IsAdmin {
		c.SendError(env.CorrelationID, "admin access required")
		return
	}

	var req models.AdminCreateUserPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	// Check if email exists
	var existing models.User
	if err := h.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		c.SendError(env.CorrelationID, "email already registered")
		return
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

	if targetOrgID == uuid.Nil {
		c.SendError(env.CorrelationID, "organization ID is required")
		return
	}

	// Default password if not provided
	pass := req.Password
	if pass == "" {
		pass = "changeme"
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("[ADMIN] Failed to hash password for new user %s: %v", req.Email, err)
		c.SendError(env.CorrelationID, "failed to hash password")
		return
	}

	user := models.User{
		ID:           uuid.New(),
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		IsAdmin:      req.IsAdmin,
	}

	if err := h.db.Create(&user).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to create user")
		return
	}

	// Link user to organization
	role := req.Role
	if role == "" {
		role = "member"
	}
	userOrg := models.UserOrganization{
		UserID:         user.ID,
		OrganizationID: targetOrgID,
		Role:           role,
		JoinedAt:       time.Now().UTC(),
	}
	h.db.Create(&userOrg)

	// Create default workspace
	wsName := req.WorkspaceName
	if wsName == "" {
		wsName = "main"
	}

	workspace := models.Workspace{
		ID:     uuid.New(),
		UserID: user.ID,
		Name:   wsName,
	}
	h.db.Create(&workspace)

	// Create default client
	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        "Default Client",
	}
	h.db.Create(&client)

	// Broadcast creation
	h.BroadcastEvent(uuid.Nil, "USER", "CREATE", map[string]any{
		"id":   user.ID.String(),
		"name": user.Name,
	})

	c.SendResponse(DataAdminUsers, env.CorrelationID, map[string]any{
		"id":              user.ID.String(),
		"name":            user.Name,
		"email":           user.Email,
		"is_admin":        user.HasAdminAccess(),
		"organization_id": targetOrgID.String(),
		"workspace": map[string]any{
			"id": workspace.ID.String(),
		},
	})
}

func (h *Hub) handleAdminUpdateUser(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	// Allow if current user is admin OR if they are updating their own profile
	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}

	var req models.AdminUpdateUserPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	targetUID, err := uuid.Parse(req.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid user_id")
		return
	}

	if !user.IsAdmin && user.ID != targetUID {
		c.SendError(env.CorrelationID, "access denied: you can only update your own profile")
		return
	}

	var targetUser models.User
	if err := h.db.First(&targetUser, "id = ?", targetUID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}

	// Update fields
	if req.Name != "" {
		targetUser.Name = req.Name
	}
	if req.Email != "" {
		targetUser.Email = req.Email
	}

	// Restriction: Only admins can change status or admin rights
	if user.IsAdmin {
		if req.IsAdmin != nil {
			targetUser.IsAdmin = *req.IsAdmin
		}
		if req.IsSuspended != nil {
			targetUser.IsSuspended = *req.IsSuspended
		}
	} else {
		if req.IsAdmin != nil || req.IsSuspended != nil {
			c.SendError(env.CorrelationID, "access denied: only admins can modify administrative flags")
			return
		}
	}

	if err := h.db.Save(&targetUser).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to update user")
		return
	}

	if targetUser.IsSuspended {
		evtPayload := map[string]string{"reason": "admin_suspended"}
		h.BroadcastToUser(targetUser.ID, func() []byte {
			b, _ := json.Marshal(models.Envelope{
				Type:    EvtForceLogout,
				Payload: createJSONPayload(evtPayload),
			})
			return b
		}())
	}

	// If organization_id is provided, update or create link
	if req.OrganizationID != "" {
		orgID, err := uuid.Parse(req.OrganizationID)
		if err != nil {
			c.SendError(env.CorrelationID, "invalid organization ID")
			return
		}
		if orgID != uuid.Nil {
			var userOrg models.UserOrganization
			if err := h.db.Where("user_id = ?", targetUser.ID).First(&userOrg).Error; err == nil {
				// Protect primary manager from role/org change by non-admin or even admin (must change primary manager first)
				var targetOrg models.Organization
				h.db.First(&targetOrg, "id = ?", userOrg.OrganizationID)
				if targetOrg.ManagerID != nil && targetUser.ID == *targetOrg.ManagerID && (orgID != userOrg.OrganizationID || (req.Role != "" && req.Role != "manager")) {
					c.SendError(env.CorrelationID, "cannot demote or move the primary manager. change primary manager first.")
					return
				}

				userOrg.OrganizationID = orgID
				if req.Role != "" {
					userOrg.Role = req.Role
				}
				h.db.Save(&userOrg)
			} else {
				// New link
				role := req.Role
				if role == "" {
					role = "member"
				}
				h.db.Create(&models.UserOrganization{
					UserID:         targetUser.ID,
					OrganizationID: orgID,
					Role:           role,
					JoinedAt:       time.Now().UTC(),
				})
			}
		}
	}

	// Broadcast update
	h.BroadcastEvent(uuid.Nil, "USER", "UPDATE", map[string]any{
		"id": targetUser.ID.String(),
	})

	c.SendResponse(DataAdminUsers, env.CorrelationID, targetUser)
}

func (h *Hub) handleAdminDeleteUser(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	// Check if current user is admin
	var adminUser models.User
	if err := h.db.First(&adminUser, "id = ?", c.UserID).Error; err != nil || !adminUser.IsAdmin {
		c.SendError(env.CorrelationID, "admin access required")
		return
	}

	var req struct {
		ID   string `json:"id"`
		Mode string `json:"mode"` // "hard" (wipe everything) or "ghost" (keep evaluations, wipe answers)
	}
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	targetUID, err := uuid.Parse(req.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid id")
		return
	}

	if targetUID == c.UserID {
		c.SendError(env.CorrelationID, "cannot delete yourself")
		return
	}

	// Fetch target user first
	var targetUser models.User
	if err := h.db.First(&targetUser, "id = ?", targetUID).Error; err != nil {
		c.SendError(env.CorrelationID, "user not found")
		return
	}

	firebaseUID := targetUser.FirebaseUID

	// Handle deletion based on mode
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if req.Mode == "ghost" {
			logger.Info("[ADMIN] Ghost-deleting user: %s (%s)", targetUser.Email, targetUser.ID)

			// 1. Wipe all answer content across all workspaces of this user
			// Subquery to find all workspace IDs for this user
			wsIDsQuery := tx.Model(&models.Workspace{}).Select("id").Where("user_id = ?", targetUID)

			// Update RunResults to remove content
			if err := tx.Model(&models.RunResult{}).
				Where("run_id IN (SELECT id FROM runs WHERE workspace_id IN (?))", wsIDsQuery).
				Update("answer", "[CONTENT DELETED BY USER]").Error; err != nil {
				return err
			}

			// 2. Anonymize user record
			// We change email and name to avoid collisions and protect privacy
			ghostID := uuid.New().String()[:8]
			if err := tx.Model(&targetUser).Updates(map[string]any{
				"name":         "Ghost User " + ghostID,
				"email":        "deleted-" + ghostID + "@example.ghost",
				"firebase_uid": "",
				"is_suspended": true, // Prevent any accidental login
			}).Error; err != nil {
				return err
			}

			return nil
		}

		// Mode "hard" (Default) - Complete wipe
		logger.Info("[ADMIN] Hard-deleting user and all data: %s (%s)", targetUser.Email, targetUser.ID)

		// 0. Nullify references in other tables to avoid FK constraints
		// Organizations
		if err := tx.Model(&models.Organization{}).Where("manager_id = ?", targetUID).Update("manager_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Organization{}).Where("created_by_user_id = ?", targetUID).Update("created_by_user_id", nil).Error; err != nil {
			return err
		}

		// User Invites
		if err := tx.Model(&models.User{}).Where("invited_by_user_id = ?", targetUID).Update("invited_by_user_id", nil).Error; err != nil {
			return err
		}

		// UserOrganization Invites
		if err := tx.Model(&models.UserOrganization{}).Where("invited_by_user_id = ?", targetUID).Update("invited_by_user_id", nil).Error; err != nil {
			return err
		}

		// Invite Codes created by this user
		if err := tx.Where("created_by = ?", targetUID).Delete(&models.InviteCode{}).Error; err != nil {
			return err
		}

		// 1. Audit Logs
		if err := tx.Where("user_id = ?", targetUID).Delete(&models.AuditLog{}).Error; err != nil {
			return err
		}

		// 2. Login Logs
		if err := tx.Where("user_id = ?", targetUID).Delete(&models.LoginLog{}).Error; err != nil {
			return err
		}

		// 3. Passkeys
		if err := tx.Where("user_id = ?", targetUID).Delete(&models.Passkey{}).Error; err != nil {
			return err
		}

		// 4. Invite Code Usages
		if err := tx.Where("user_id = ?", targetUID).Delete(&models.InviteCodeUsage{}).Error; err != nil {
			return err
		}

		// Get all workspace IDs for cleanup
		var wsIDs []uuid.UUID
		tx.Model(&models.Workspace{}).Where("user_id = ?", targetUID).Pluck("id", &wsIDs)

		if len(wsIDs) > 0 {
			// Wipe Evaluations
			if err := tx.Exec(`DELETE FROM evaluations WHERE run_result_id IN (
				SELECT id FROM run_results WHERE run_id IN (
					SELECT id FROM runs WHERE workspace_id IN (?)
				)
			)`, wsIDs).Error; err != nil {
				return err
			}

			// Wipe Run Results
			if err := tx.Exec(`DELETE FROM run_results WHERE run_id IN (
				SELECT id FROM runs WHERE workspace_id IN (?)
			)`, wsIDs).Error; err != nil {
				return err
			}

			// Wipe Runs
			if err := tx.Where("workspace_id IN (?)", wsIDs).Delete(&models.Run{}).Error; err != nil {
				return err
			}

			// Wipe Agent Configs for Question Sets (Junction)
			if err := tx.Exec(`DELETE FROM question_set_agents WHERE question_set_id IN (
				SELECT id FROM question_sets WHERE client_id IN (
					SELECT id FROM clients WHERE workspace_id IN (?)
				)
			)`, wsIDs).Error; err != nil {
				return err
			}

			// Wipe Question Sets
			if err := tx.Exec(`DELETE FROM question_sets WHERE client_id IN (
				SELECT id FROM clients WHERE workspace_id IN (?)
			)`, wsIDs).Error; err != nil {
				return err
			}

			// Wipe Clients
			if err := tx.Where("workspace_id IN (?)", wsIDs).Delete(&models.Client{}).Error; err != nil {
				return err
			}

			// Wipe Agents
			if err := tx.Where("workspace_id IN (?)", wsIDs).Delete(&models.Agent{}).Error; err != nil {
				return err
			}

			// Wipe StatsCache entries and other workspace scoped items if any
			if err := tx.Where("scope = 'workspace' AND scope_id IN (?)", wsIDs).Delete(&models.StatsCache{}).Error; err != nil {
				return err
			}

			// Wipe Workspaces
			if err := tx.Where("user_id = ?", targetUID).Delete(&models.Workspace{}).Error; err != nil {
				return err
			}
		}

		// 5. Org links
		if err := tx.Where("user_id = ?", targetUID).Delete(&models.UserOrganization{}).Error; err != nil {
			return err
		}

		// 6. Finally wipe the user
		if err := tx.Delete(&models.User{}, "id = ?", targetUID).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		logger.Error("[ADMIN] Transaction failed during user deletion: %v", err)
		c.SendError(env.CorrelationID, "failed to delete user: "+err.Error())
		return
	}

	// Broadcast change to all admins (refresh list)
	h.BroadcastEvent(uuid.Nil, "USER", "DELETE", map[string]string{"id": req.ID, "mode": req.Mode})

	// Remote Firebase delete (Best effort)
	if firebaseUID != "" && h.Firebase != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.Firebase.DeleteUser(ctx, firebaseUID); err != nil {
			logger.Warn("[FIREBASE] Failed to delete user from firebase: %v", err)
			// We don't fail the whole request because the local data IS gone
		} else {
			logger.Info("[FIREBASE] User %s deleted from firebase", firebaseUID)
		}
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{
		"status": "deleted",
		"mode":   req.Mode,
	})
}
