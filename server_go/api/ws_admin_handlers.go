package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	goruntime "runtime"
	"strings"
	"time"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/security"
	"benchmarking-platform/internal/service"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const adminDebugFailureSampleLimit = 8
const adminDebugRecentRecordLimit = 12

type adminDebugAgentRow struct {
	ID          uuid.UUID `gorm:"column:id"`
	WorkspaceID uuid.UUID `gorm:"column:workspace_id"`
	Name        string    `gorm:"column:name"`
	Config      string    `gorm:"column:config"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

type adminDebugQuestionSetAgentRow struct {
	QuestionSetID uuid.UUID `gorm:"column:question_set_id"`
	AgentID       uuid.UUID `gorm:"column:agent_id"`
	Config        string    `gorm:"column:config"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

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
		json.Unmarshal(env.Payload, &filter)
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

func (h *Hub) handleAdminGetOrganizations(c *Connection, env models.Envelope) {
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
		json.Unmarshal(env.Payload, &filter)
	}

	query := h.db.Model(&models.Organization{})
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

	var orgs []models.Organization
	query.Preload("Manager").Order("created_at DESC").Find(&orgs)

	result := make([]map[string]any, len(orgs))
	for i, org := range orgs {
		var userCount int64
		h.db.Raw(`SELECT COUNT(*) FROM user_organizations WHERE organization_id = ?`, org.ID).Scan(&userCount)

		managerName := ""
		managerID := org.ManagerID
		if org.Manager != nil {
			managerName = org.Manager.Name
		} else {
			// Fallback: look for a user with 'manager' role in user_organizations
			var managerUser models.User
			err := h.db.Joins("JOIN user_organizations on user_organizations.user_id = users.id").
				Where("user_organizations.organization_id = ? AND user_organizations.role = ?", org.ID, "manager").
				First(&managerUser).Error
			if err == nil {
				managerName = managerUser.Name
				managerID = &managerUser.ID
			}
		}

		result[i] = map[string]any{
			"id":           org.ID.String(),
			"name":         org.Name,
			"is_suspended": org.IsSuspended,
			"manager_id":   managerID,
			"manager_name": managerName,
			"user_count":   userCount,
			"created_at":   org.CreatedAt,
		}
	}

	c.SendResponse(DataAdminOrganizations, env.CorrelationID, result)
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

	var workspaces []map[string]any
	workspaces = make([]map[string]any, len(wsList))
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

func (h *Hub) handleAdminGetOrgProfile(c *Connection, env models.Envelope) {
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

	var req models.AdminProfilePayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	orgID, err := uuid.Parse(req.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid organization ID")
		return
	}

	var org models.Organization
	if err := h.db.Preload("Manager").First(&org, "id = ?", orgID).Error; err != nil {
		c.SendError(env.CorrelationID, "organization not found")
		return
	}

	// Get users with workspace count
	var users []map[string]any
	h.db.Raw(`
		SELECT u.id, u.name, u.email,
		       CASE WHEN u.is_admin = true OR LOWER(u.email) = ? THEN true ELSE false END as is_admin,
		       u.created_at, 
		       COUNT(w.id) as workspace_count,
		       uo.role,
		       inviter.name as invited_by_name
		FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		LEFT JOIN workspaces w ON w.user_id = u.id
		LEFT JOIN users inviter ON uo.invited_by_user_id = inviter.id
		WHERE uo.organization_id = ?
		GROUP BY u.id, uo.role, inviter.name
	`, models.HardcodedAdminEmail(), orgID).Scan(&users)

	// Get workspaces with counts
	var workspaces []map[string]any
	h.db.Raw(`
		SELECT w.id, w.name, w.user_id, w.created_at,
		       (SELECT COUNT(*) FROM agents WHERE workspace_id = w.id) as agent_count,
		       (SELECT COUNT(*) FROM runs WHERE workspace_id = w.id) as run_count
		FROM workspaces w
		WHERE w.organization_id = ?
	`, orgID).Scan(&workspaces)

	result := map[string]any{
		"id":           org.ID.String(),
		"name":         org.Name,
		"is_suspended": org.IsSuspended,
		"manager_id":   org.ManagerID,
		"manager":      org.Manager,
		"users":        users,
		"workspaces":   workspaces,
		"created_at":   org.CreatedAt,
	}

	c.SendResponse(DataAdminOrgProfile, env.CorrelationID, result)
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

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)

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

func (h *Hub) handleAdminCreateOrg(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	// Check if current user is admin
	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil || !user.IsAdmin {
		c.SendError(env.CorrelationID, "admin access required")
		return
	}

	var req models.AdminCreateOrgPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload: "+err.Error())
		return
	}

	var managerID *uuid.UUID
	if req.ManagerID != "" {
		if id, err := uuid.Parse(req.ManagerID); err == nil {
			managerID = &id
		}
	}

	org := models.Organization{
		ID:              uuid.New(),
		Name:            req.Name,
		ManagerID:       managerID,
		CreatedByUserID: &c.UserID,
	}

	if err := h.db.Create(&org).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to create organization")
		return
	}

	// Broadcast creation
	h.BroadcastEvent(uuid.Nil, "ORGANIZATION", "CREATE", map[string]any{
		"id":   org.ID.String(),
		"name": org.Name,
	})

	c.SendResponse(DataAdminOrganizations, env.CorrelationID, org)
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
		orgID, _ := uuid.Parse(req.OrganizationID)
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

func (h *Hub) handleAdminUpdateOrg(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	// Check admin
	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil || !user.IsAdmin {
		c.SendError(env.CorrelationID, "admin access required")
		return
	}

	var req models.AdminUpdateOrgPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	orgID, err := uuid.Parse(req.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid id")
		return
	}

	var org models.Organization
	if err := h.db.First(&org, "id = ?", orgID).Error; err != nil {
		c.SendError(env.CorrelationID, "organization not found")
		return
	}

	if req.Name != "" {
		org.Name = req.Name
	}

	// Handle Managers
	if len(req.ManagerIDs) > 0 {
		// New multi-manager logic

		// 1. Reset all users in this org to 'member' role initially (transaction safe ideally, but simple update here)
		h.db.Model(&models.UserOrganization{}).Where("organization_id = ?", org.ID).Update("role", "member")

		// 2. Set 'manager' role for selected IDs
		for _, midStr := range req.ManagerIDs {
			mid, err := uuid.Parse(midStr)
			if err == nil && mid != uuid.Nil {
				h.db.Model(&models.UserOrganization{}).
					Where("organization_id = ? AND user_id = ?", org.ID, mid).
					Update("role", "manager")
			}
		}

		// 3. Sync legacy ManagerID to the first one for backward compat
		firstMid, _ := uuid.Parse(req.ManagerIDs[0])
		org.ManagerID = &firstMid

	} else if req.ManagerID != "" {
		// Legacy single manager logic (frontend sending old payload or specific single set)
		mID, _ := uuid.Parse(req.ManagerID)
		if mID != uuid.Nil {
			org.ManagerID = &mID

			// Reset others to member
			h.db.Model(&models.UserOrganization{}).Where("organization_id = ?", org.ID).Update("role", "member")
			// Set new manager
			h.db.Model(&models.UserOrganization{}).
				Where("organization_id = ? AND user_id = ?", org.ID, mID).
				Update("role", "manager")
		} else {
			org.ManagerID = nil
			// No manager, reset all
			h.db.Model(&models.UserOrganization{}).Where("organization_id = ?", org.ID).Update("role", "member")
		}
	}

	if req.IsSuspended != nil {
		org.IsSuspended = *req.IsSuspended
	}

	if err := h.db.Save(&org).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to update organization")
		return
	}

	// If suspending, force logout all users of this org
	if req.IsSuspended != nil && *req.IsSuspended {
		go func() {
			var userIDs []uuid.UUID
			h.db.Model(&models.UserOrganization{}).Where("organization_id = ?", org.ID).Pluck("user_id", &userIDs)

			evtPayload := map[string]string{"reason": "organization_suspended"}
			msg := func() []byte {
				b, _ := json.Marshal(models.Envelope{
					Type:    EvtForceLogout,
					Payload: createJSONPayload(evtPayload),
				})
				return b
			}()

			for _, uid := range userIDs {
				h.BroadcastToUser(uid, msg)
			}
		}()
	}

	// Broadcast update
	h.BroadcastEvent(uuid.Nil, "ORGANIZATION", "UPDATE", map[string]any{
		"id": org.ID.String(),
	})

	c.SendResponse(DataAdminOrganizations, env.CorrelationID, org)
}

func (h *Hub) handleAdminDeleteOrg(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", c.UserID).Error; err != nil || !user.IsAdmin {
		c.SendError(env.CorrelationID, "admin access required")
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	orgID, err := uuid.Parse(req.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid id")
		return
	}

	if err := h.db.Delete(&models.Organization{}, "id = ?", orgID).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to delete organization")
		return
	}

	// Broadcast deletion
	h.BroadcastEvent(uuid.Nil, "ORGANIZATION", "DELETE", map[string]any{
		"id": orgID.String(),
	})

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "deleted"})
}

// handleAdminGenerateInvite allows admins to create invite codes.
// Can specify target_org_id OR is_new_org.
func (h *Hub) handleAdminGenerateInvite(c *Connection, env models.Envelope) {
	// 1. Check Admin Permission
	if err := h.checkAdmin(c, env); err != nil {
		return
	}

	var payload struct {
		TargetOrgID string `json:"target_org_id"`
		IsNewOrg    bool   `json:"is_new_org"`
		MaxUses     int    `json:"max_uses"`
	}
	if err := json.Unmarshal([]byte(env.Payload), &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	if payload.MaxUses <= 0 {
		payload.MaxUses = 1
	}

	// invite variable removed as it is now handled by helper functions

	maxUses := payload.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}

	// 2. Case: Invite to Existing Org
	orgID, err := uuid.Parse(payload.TargetOrgID)
	if err == nil && orgID != uuid.Nil {
		// Verify org exists
		var org models.Organization
		if err := h.db.First(&org, "id = ?", orgID).Error; err != nil {
			c.SendError(env.CorrelationID, "organization not found")
			return
		}

		inviteCodeStr, err := models.GenerateInviteForOrg(h.db, c.UserID, orgID, maxUses)
		if err != nil {
			c.SendError(env.CorrelationID, "failed to generate invite: "+err.Error())
			return
		}

		c.SendResponse(DataResponse, env.CorrelationID, map[string]string{
			"code":            inviteCodeStr,
			"organization_id": orgID.String(),
		})
		return
	}

	// 3. Case: Create NEW Org Invite (implicit)
	// If IsNewOrg is true, we generate a special platform invite that triggers org creation
	if payload.IsNewOrg {
		// For now, we reuse the generic platform invite but perhaps we want to attach metadata
		// simplified: just generate a platform invite
		inviteCodeStr, err := models.GenerateInviteForPlatform(h.db, c.UserID, maxUses)
		if err != nil {
			c.SendError(env.CorrelationID, "failed to generate invite: "+err.Error())
			return
		}
		c.SendResponse(DataResponse, env.CorrelationID, map[string]string{
			"code": inviteCodeStr,
			"type": "new_org",
		})
		return
	}

	c.SendError(env.CorrelationID, "invalid request: specify target_org_id or set is_new_org=true")
}

func (h *Hub) handleAdminRemoveUserFromOrg(c *Connection, env models.Envelope) {
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

	var req models.AdminRemoveUserFromOrgPayload
	if err := json.Unmarshal([]byte(env.Payload), &req); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	targetUserID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid user id")
		return
	}
	targetOrgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid organization id")
		return
	}

	// Validation: Cannot remove the Primary Manager
	var org models.Organization
	if err := h.db.First(&org, "id = ?", targetOrgID).Error; err != nil {
		c.SendError(env.CorrelationID, "organization not found")
		return
	}

	if org.ManagerID != nil && *org.ManagerID == targetUserID {
		c.SendError(env.CorrelationID, "cannot remove the primary manager. change primary manager first.")
		return
	}

	// Perform Deletion
	if err := h.db.Where("user_id = ? AND organization_id = ?", targetUserID, targetOrgID).Delete(&models.UserOrganization{}).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to remove user from organization")
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "removed"})
}

func (h *Hub) handleAdminForceLogout(c *Connection, env models.Envelope) {
	if err := h.checkAdmin(c, env); err != nil {
		return
	}

	var payload models.AdminForceLogoutPayload
	if err := json.Unmarshal([]byte(env.Payload), &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	evtPayload := map[string]string{"reason": "admin_forced"}

	if payload.UserID != "" {
		// Logout specific user
		uid, err := uuid.Parse(payload.UserID)
		if err != nil {
			c.SendError(env.CorrelationID, "invalid user_id")
			return
		}

		// Send event ONLY to that user's connections
		h.BroadcastToUser(uid, func() []byte {
			b, _ := json.Marshal(models.Envelope{
				Type:    EvtForceLogout,
				Payload: createJSONPayload(evtPayload),
			})
			return b
		}())
	} else {
		// Global logout (except current admin)
		// We could iterate connected users or just rely on a global broadcast event that clients listen to
		// Broadcasting to all authenticated connections
		msg := func() []byte {
			b, _ := json.Marshal(models.Envelope{
				Type:    EvtForceLogout,
				Payload: createJSONPayload(evtPayload),
			})
			return b
		}()

		h.mu.RLock()
		defer h.mu.RUnlock()
		for _, conn := range h.connections {
			if conn.IsAuthenticated && conn.UserID != c.UserID {
				_ = conn.safeSend(msg)
			}
		}
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "logout_signal_sent"})
}

func (h *Hub) handleAdminGetLoginLogs(c *Connection, env models.Envelope) {
	if err := h.checkAdmin(c, env); err != nil {
		return
	}

	// Default limit: 100
	limit := 100
	var payload struct {
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal([]byte(env.Payload), &payload); err == nil && payload.Limit > 0 {
		limit = payload.Limit
		if limit > 500 {
			limit = 500 // Cap limit
		}
	}

	var logs []models.LoginLog
	// Fetch logs, order by recent first
	// Preload nothing for now as UserID/OrganizationID might be null or deleted, and we just want raw logs mostly
	if err := h.db.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to fetch logs")
		return
	}

	c.SendResponse(DataAdminLoginLogs, env.CorrelationID, logs)
}

func (h *Hub) handleAdminGetRuns(c *Connection, env models.Envelope) {
	if err := h.checkAdmin(c, env); err != nil {
		return
	}

	limit := 100
	var payload models.AdminRunsPayload
	if err := json.Unmarshal([]byte(env.Payload), &payload); err == nil && payload.Limit > 0 {
		limit = payload.Limit
		if limit > 500 {
			limit = 500
		}
	}

	hasCreatedByColumn := h.db.Migrator().HasColumn(&models.Run{}, "created_by_user_id")
	starterJoin := ""
	startedByExpr := "COALESCE(owner.name, 'Unknown')"
	activeUsersExpr := "COUNT(DISTINCT w.user_id)"
	if hasCreatedByColumn {
		starterJoin = "LEFT JOIN users starter ON starter.id = r.created_by_user_id"
		startedByExpr = "COALESCE(starter.name, owner.name, 'Unknown')"
		activeUsersExpr = "COUNT(DISTINCT COALESCE(r.created_by_user_id, w.user_id))"
	}

	type runRow struct {
		ID              uuid.UUID `json:"id"`
		Status          string    `json:"status"`
		WorkspaceID     uuid.UUID `json:"workspace_id"`
		WorkspaceName   string    `json:"workspace_name"`
		QuestionSetName string    `json:"question_set_name"`
		StartedByName   string    `json:"started_by_name"`
		TotalTasks      int       `json:"total_tasks"`
		ResultCount     int64     `json:"result_count"`
		SuccessCount    int64     `json:"success_count"`
		ErrorCount      int64     `json:"error_count"`
		CreatedAt       time.Time `json:"created_at"`
		LastActivityAt  string    `json:"last_activity_at"`
	}

	var runRows []runRow
	runsQuery := fmt.Sprintf(`
		WITH recent_runs AS (
			SELECT
				r.id,
				r.status,
				r.workspace_id,
				r.total_tasks,
				r.created_at,
				w.name AS workspace_name,
				COALESCE(qs.name, '(deleted question set)') AS question_set_name,
				%s AS started_by_name
			FROM runs r
			JOIN workspaces w ON w.id = r.workspace_id
			%s
			LEFT JOIN users owner ON owner.id = w.user_id
			LEFT JOIN question_sets qs ON qs.id = r.question_set_id
			ORDER BY CASE WHEN r.status = 'running' THEN 0 ELSE 1 END, r.created_at DESC
			LIMIT ?
		)
		SELECT
			recent_runs.id,
			recent_runs.status,
			recent_runs.workspace_id,
			recent_runs.workspace_name,
			recent_runs.question_set_name,
			recent_runs.started_by_name,
			recent_runs.total_tasks,
			COUNT(rr.id) AS result_count,
			COALESCE(SUM(CASE WHEN rr.status = 'success' THEN 1 ELSE 0 END), 0) AS success_count,
			COALESCE(SUM(CASE WHEN rr.status = 'error' THEN 1 ELSE 0 END), 0) AS error_count,
			recent_runs.created_at,
			MAX(rr.created_at) AS last_activity_at
		FROM recent_runs
		LEFT JOIN run_results rr ON rr.run_id = recent_runs.id
		GROUP BY
			recent_runs.id,
			recent_runs.status,
			recent_runs.workspace_id,
			recent_runs.workspace_name,
			recent_runs.question_set_name,
			recent_runs.started_by_name,
			recent_runs.total_tasks,
			recent_runs.created_at
		ORDER BY CASE WHEN recent_runs.status = 'running' THEN 0 ELSE 1 END, recent_runs.created_at DESC
	`, startedByExpr, starterJoin)
	if err := h.db.Raw(runsQuery, limit).Scan(&runRows).Error; err != nil {
		logger.Error("[ADMIN] failed to fetch admin runs: %v", err)
		c.SendError(env.CorrelationID, "failed to fetch runs: "+err.Error())
		return
	}

	var summary models.AdminRunsSummary
	summaryQuery := fmt.Sprintf(`
		SELECT
			COUNT(*) AS active_runs,
			COUNT(DISTINCT r.workspace_id) AS active_workspaces,
			%s AS active_users
		FROM runs r
		JOIN workspaces w ON w.id = r.workspace_id
		WHERE r.status = 'running'
	`, activeUsersExpr)
	if err := h.db.Raw(summaryQuery).Scan(&summary).Error; err != nil {
		logger.Error("[ADMIN] failed to fetch admin run summary: %v", err)
		c.SendError(env.CorrelationID, "failed to fetch run summary: "+err.Error())
		return
	}

	type pendingRow struct {
		TotalTasks  int   `json:"total_tasks"`
		ResultCount int64 `json:"result_count"`
	}
	var pendingRows []pendingRow
	if err := h.db.Raw(`
		SELECT
			r.total_tasks,
			COUNT(rr.id) AS result_count
		FROM runs r
		LEFT JOIN run_results rr ON rr.run_id = r.id
		WHERE r.status = 'running'
		GROUP BY r.id, r.total_tasks
	`).Scan(&pendingRows).Error; err != nil {
		logger.Error("[ADMIN] failed to calculate admin pending tasks: %v", err)
		c.SendError(env.CorrelationID, "failed to calculate pending tasks: "+err.Error())
		return
	}

	var runs []models.AdminRunRecord
	runs = make([]models.AdminRunRecord, 0, len(runRows))
	var totalPendingTasks int64
	for _, row := range runRows {
		pendingCount := int64(row.TotalTasks) - row.ResultCount
		if pendingCount < 0 {
			pendingCount = 0
		}

		progressPercent := 0.0
		if row.TotalTasks > 0 {
			progressPercent = (float64(row.ResultCount) / float64(row.TotalTasks)) * 100
			if progressPercent > 100 {
				progressPercent = 100
			}
		}

		lastActivityAt := parseAdminRunTimestamp(row.LastActivityAt, row.CreatedAt)

		runs = append(runs, models.AdminRunRecord{
			ID:              row.ID,
			Status:          row.Status,
			WorkspaceID:     row.WorkspaceID,
			WorkspaceName:   row.WorkspaceName,
			QuestionSetName: row.QuestionSetName,
			StartedByName:   row.StartedByName,
			TotalTasks:      row.TotalTasks,
			ResultCount:     row.ResultCount,
			SuccessCount:    row.SuccessCount,
			ErrorCount:      row.ErrorCount,
			PendingCount:    pendingCount,
			ProgressPercent: progressPercent,
			CreatedAt:       row.CreatedAt,
			LastActivityAt:  lastActivityAt,
		})
	}

	for _, row := range pendingRows {
		pendingCount := int64(row.TotalTasks) - row.ResultCount
		if pendingCount > 0 {
			totalPendingTasks += pendingCount
		}
	}

	summary.PendingTasks = totalPendingTasks
	summary.RecentRuns = int64(len(runs))

	c.SendResponse(DataAdminRuns, env.CorrelationID, models.AdminRunsResponse{
		Summary:     summary,
		Runs:        runs,
		GeneratedAt: time.Now().UTC(),
	})
}

func (h *Hub) handleAdminGetDebugInfo(c *Connection, env models.Envelope) {
	if err := h.checkAdmin(c, env); err != nil {
		return
	}

	var agentRows []adminDebugAgentRow
	if err := h.db.Raw(`
		SELECT id, workspace_id, name, COALESCE(config, '') AS config, created_at
		FROM agents
		ORDER BY created_at DESC
	`).Scan(&agentRows).Error; err != nil {
		logger.Error("[ADMIN] failed to inspect agent configs: %v", err)
		c.SendError(env.CorrelationID, "failed to inspect agent configs: "+err.Error())
		return
	}

	var questionSetAgentRows []adminDebugQuestionSetAgentRow
	if err := h.db.Raw(`
		SELECT question_set_id, agent_id, COALESCE(config, '') AS config, created_at
		FROM question_set_agents
		ORDER BY created_at DESC
	`).Scan(&questionSetAgentRows).Error; err != nil {
		logger.Error("[ADMIN] failed to inspect question set agent configs: %v", err)
		c.SendError(env.CorrelationID, "failed to inspect question set agent configs: "+err.Error())
		return
	}

	encryptionKeyHealth, err := service.NewEncryptionKeyService(h.db).InspectCurrentKeyHealth()
	if err != nil {
		logger.Warn("[ADMIN] failed to inspect persisted encryption key state: %v", err)
	}

	response := models.AdminDebugResponse{
		AppEnv:            strings.TrimSpace(os.Getenv("APP_ENV")),
		GoVersion:         goruntime.Version(),
		ServiceName:       strings.TrimSpace(os.Getenv("K_SERVICE")),
		ServiceRevision:   strings.TrimSpace(os.Getenv("K_REVISION")),
		Revision:          buildAdminDebugRevision(),
		Key:               buildAdminDebugKeyStatus(encryptionKeyHealth),
		Agents:            analyzeAdminDebugAgents(agentRows),
		QuestionSetAgents: analyzeAdminDebugQuestionSetAgents(questionSetAgentRows),
		GeneratedAt:       time.Now().UTC(),
	}

	if response.AppEnv == "" {
		response.AppEnv = "development"
	}

	c.SendResponse(DataAdminDebugInfo, env.CorrelationID, response)
}

func parseAdminDebugConfig(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "empty", nil
	}

	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		return "plaintext_json", nil
	}

	shape := "invalid_other"
	if _, err := base64.StdEncoding.DecodeString(trimmed); err == nil {
		shape = "encrypted_like"
	}

	_, err := security.Decrypt(trimmed)
	return shape, err
}

func analyzeAdminDebugAgents(rows []adminDebugAgentRow) models.AdminDebugConfigStats {
	stats := models.AdminDebugConfigStats{Total: int64(len(rows))}

	for index, row := range rows {
		shape, err := parseAdminDebugConfig(row.Config)
		decryptStatus := adminDebugDecryptStatus(shape, err)
		switch shape {
		case "empty":
			stats.Empty++
		case "plaintext_json":
			stats.PlaintextJSON++
		case "encrypted_like":
			stats.EncryptedLike++
		default:
			stats.InvalidOther++
		}

		if index < adminDebugRecentRecordLimit {
			record := models.AdminDebugConfigRecord{
				ID:            row.ID.String(),
				WorkspaceID:   row.WorkspaceID.String(),
				Name:          row.Name,
				CreatedAt:     row.CreatedAt,
				Shape:         shape,
				DecryptStatus: decryptStatus,
			}
			if err != nil && decryptStatus == "failed" {
				record.Error = err.Error()
			}
			stats.RecentRecords = append(stats.RecentRecords, record)
		}

		if err == nil || shape == "empty" || shape == "plaintext_json" {
			if shape == "encrypted_like" {
				stats.DecryptOK++
			}
			continue
		}

		stats.DecryptFailed++
		if len(stats.SampleFailures) < adminDebugFailureSampleLimit {
			stats.SampleFailures = append(stats.SampleFailures, models.AdminDebugConfigFailure{
				ID:          row.ID.String(),
				WorkspaceID: row.WorkspaceID.String(),
				Name:        row.Name,
				CreatedAt:   row.CreatedAt,
				Shape:       shape,
				Error:       err.Error(),
			})
		}
	}

	return stats
}

func analyzeAdminDebugQuestionSetAgents(rows []adminDebugQuestionSetAgentRow) models.AdminDebugConfigStats {
	stats := models.AdminDebugConfigStats{Total: int64(len(rows))}

	for index, row := range rows {
		shape, err := parseAdminDebugConfig(row.Config)
		decryptStatus := adminDebugDecryptStatus(shape, err)
		switch shape {
		case "empty":
			stats.Empty++
		case "plaintext_json":
			stats.PlaintextJSON++
		case "encrypted_like":
			stats.EncryptedLike++
		default:
			stats.InvalidOther++
		}

		if index < adminDebugRecentRecordLimit {
			record := models.AdminDebugConfigRecord{
				QuestionSetID: row.QuestionSetID.String(),
				AgentID:       row.AgentID.String(),
				CreatedAt:     row.CreatedAt,
				Shape:         shape,
				DecryptStatus: decryptStatus,
			}
			if err != nil && decryptStatus == "failed" {
				record.Error = err.Error()
			}
			stats.RecentRecords = append(stats.RecentRecords, record)
		}

		if err == nil || shape == "empty" || shape == "plaintext_json" {
			if shape == "encrypted_like" {
				stats.DecryptOK++
			}
			continue
		}

		stats.DecryptFailed++
		if len(stats.SampleFailures) < adminDebugFailureSampleLimit {
			stats.SampleFailures = append(stats.SampleFailures, models.AdminDebugConfigFailure{
				AgentID:       row.AgentID.String(),
				QuestionSetID: row.QuestionSetID.String(),
				CreatedAt:     row.CreatedAt,
				Shape:         shape,
				Error:         err.Error(),
			})
		}
	}

	return stats
}

func adminDebugDecryptStatus(shape string, err error) string {
	if err == nil {
		if shape == "encrypted_like" {
			return "ok"
		}
		return "not_applicable"
	}
	return "failed"
}

func buildAdminDebugRevision() models.AdminDebugRevision {
	return models.AdminDebugRevision{
		Commit:    firstNonEmptyAdminDebug(os.Getenv("APP_REVISION"), os.Getenv("GIT_COMMIT")),
		Branch:    firstNonEmptyAdminDebug(os.Getenv("APP_REVISION_BRANCH")),
		Dirty:     firstNonEmptyAdminDebug(os.Getenv("APP_REVISION_DIRTY")),
		UpdatedAt: firstNonEmptyAdminDebug(os.Getenv("APP_REVISION_UPDATED_AT")),
	}
}

func buildAdminDebugKeyStatus(health service.EncryptionKeyHealth) models.AdminDebugKeyStatus {
	raw := os.Getenv("ENCRYPTION_KEY")
	runtimeStatus := security.GetEncryptionKeyRuntimeStatus()
	status := models.AdminDebugKeyStatus{
		Status:                  runtimeStatus.Status,
		Source:                  runtimeStatus.Source,
		Summary:                 runtimeStatus.Summary,
		Present:                 strings.TrimSpace(raw) != "",
		CharLength:              len(raw),
		Loaded:                  runtimeStatus.Loaded,
		UsedFallback:            runtimeStatus.UsedFallback,
		StatePresent:            health.StatePresent,
		StateStatus:             health.StateStatus,
		StateSummary:            health.StateSummary,
		CipherVersion:           health.CipherVersion,
		FingerprintPrefix:       health.ObservedFingerprintPrefix,
		StoredFingerprintPrefix: health.StoredFingerprintPrefix,
		LastSeenAt:              health.LastSeenAt,
		LastMismatchAt:          health.LastMismatchAt,
	}

	if !status.Present {
		if status.Status == "" {
			status.Status = "missing"
		}
		if status.Summary == "" {
			status.Summary = "ENCRYPTION_KEY is not set"
		}
		status.Error = "ENCRYPTION_KEY environment variable not set"
		return status
	}

	key, format, err := security.ParseEncryptionKey(raw)
	if err != nil {
		if status.Status == "" {
			status.Status = "invalid"
		}
		if status.Source == "" {
			status.Source = "environment"
		}
		if status.Summary == "" {
			status.Summary = "ENCRYPTION_KEY is present but invalid"
		}
		status.Error = err.Error()
		return status
	}

	status.Format = firstNonEmptyAdminDebug(runtimeStatus.Format, format)
	status.ParsedBytes = maxAdminDebugInt(runtimeStatus.ParsedBytes, len(key))
	if status.FingerprintPrefix == "" {
		status.FingerprintPrefix = shortAdminDebugFingerprint(security.KeyFingerprint(key))
	}
	if status.Status == "" {
		status.Status = "loaded"
	}
	if status.Source == "" {
		status.Source = "environment"
	}
	if status.Summary == "" {
		if status.Format == "hex" {
			status.Summary = "ENCRYPTION_KEY loaded successfully from a hex-encoded environment value"
		} else {
			status.Summary = "ENCRYPTION_KEY loaded successfully from environment"
		}
	}
	status.Loaded = true
	return status
}

func firstNonEmptyAdminDebug(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func maxAdminDebugInt(current, fallback int) int {
	if current > 0 {
		return current
	}
	return fallback
}

func shortAdminDebugFingerprint(fingerprint string) string {
	trimmed := strings.TrimSpace(fingerprint)
	if len(trimmed) <= 12 {
		return trimmed
	}
	return trimmed[:12]
}

func parseAdminRunTimestamp(raw string, fallback time.Time) time.Time {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return fallback
	}

	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, candidate); err == nil {
			return parsed
		}
	}

	return fallback
}

// handleAdminStartMaintenance notifies all users that maintenance is starting.
func (h *Hub) handleAdminStartMaintenance(c *Connection, env models.Envelope) {
	if err := h.checkAdmin(c, env); err != nil {
		return
	}

	// Broadcast maintenance event to ALL connected users
	h.BroadcastToAll(func() []byte {
		b, _ := json.Marshal(models.Envelope{
			Type:    EvtMaintenanceStarted,
			Payload: json.RawMessage("{}"),
		})
		return b
	}())

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "maintenance_signal_sent"})
}

// Helper to avoid repetitive JSON marshaling error handling in this block
