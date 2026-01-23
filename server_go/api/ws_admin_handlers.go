package api

import (
	"encoding/json"
	"strings"
	"time"

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

	var users []models.User
	h.db.Preload("Workspaces.Organization").
		Preload("Workspaces.User").
		Order("created_at DESC").
		Find(&users)

	// Map to response format
	result := make([]map[string]any, len(users))
	for i, u := range users {
		var orgNames []string
		var userOrgs []models.UserOrganization
		h.db.Preload("Organization").Where("user_id = ?", u.ID).Find(&userOrgs)
		for _, uo := range userOrgs {
			orgNames = append(orgNames, uo.Organization.Name)
		}

		// Fix for zeroed User and Organization structs in Workspaces due to GORM recursion prevention

		// 1. Prepare safe User (no cycles)
		safeUser := u
		safeUser.Workspaces = nil
		safeUser.Organizations = nil
		safeUser.UserOrgs = nil

		// 2. Prepare Org map for lookup
		orgMap := make(map[uuid.UUID]models.Organization)
		for _, uo := range userOrgs {
			// Create safe Org (no cycles)
			safeOrg := uo.Organization
			safeOrg.Workspaces = nil
			safeOrg.Users = nil
			safeOrg.UserOrgs = nil
			orgMap[uo.OrganizationID] = safeOrg
		}

		for j := range u.Workspaces {
			// Fix User
			u.Workspaces[j].User = safeUser

			// Fix Organization
			if org, ok := orgMap[u.Workspaces[j].OrganizationID]; ok {
				u.Workspaces[j].Organization = org
			}
		}

		result[i] = map[string]any{
			"id":                u.ID.String(),
			"name":              u.Name,
			"email":             u.Email,
			"is_admin":          u.IsAdmin,
			"is_suspended":      u.IsSuspended,
			"created_at":        u.CreatedAt,
			"workspaces":        u.Workspaces,
			"org_names":         orgNames,
			"organization_name": strings.Join(orgNames, ", "),
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

	var orgs []models.Organization
	h.db.Preload("Manager").Find(&orgs)

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
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
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
	h.db.Preload("User").Preload("Organization").Where("user_id = ?", userID).Find(&wsList)

	var workspaces []map[string]any
	workspaces = make([]map[string]any, len(wsList))
	for i, w := range wsList {
		var cnt int64
		h.db.Model(&models.Agent{}).Where("workspace_id = ?", w.ID).Count(&cnt)

		workspaces[i] = map[string]any{
			"id":           w.ID,
			"name":         w.Name,
			"created_at":   w.CreatedAt,
			"user":         w.User,
			"organization": w.Organization,
			"agent_count":  cnt,
		}
	}

	result := map[string]any{
		"id":            user.ID.String(),
		"name":          user.Name,
		"email":         user.Email,
		"is_admin":      user.IsAdmin,
		"is_manager":    isManagerGlobal,
		"organizations": organizations,
		"workspaces":    workspaces,
		"created_at":    user.CreatedAt,
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
		SELECT u.id, u.name, u.email, u.is_admin, u.created_at, 
		       COUNT(w.id) as workspace_count
		FROM users u
		JOIN user_organizations uo ON uo.user_id = u.id
		LEFT JOIN workspaces w ON w.user_id = u.id
		WHERE uo.organization_id = ?
		GROUP BY u.id
	`, orgID).Scan(&users)

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
		JoinedAt:       time.Now(),
	}
	h.db.Create(&userOrg)

	// Create default workspace
	wsName := req.WorkspaceName
	if wsName == "" {
		wsName = "main"
	}

	workspace := models.Workspace{
		ID:             uuid.New(),
		UserID:         user.ID,
		OrganizationID: targetOrgID,
		Name:           wsName,
	}
	h.db.Create(&workspace)

	// Create default client
	client := models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        "Default Client",
	}
	h.db.Create(&client)

	c.SendResponse(DataAdminUsers, env.CorrelationID, map[string]any{
		"id":              user.ID.String(),
		"name":            user.Name,
		"email":           user.Email,
		"is_admin":        user.IsAdmin,
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
		ID:        uuid.New(),
		Name:      req.Name,
		ManagerID: managerID,
	}

	if err := h.db.Create(&org).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to create organization")
		return
	}

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
				userOrg.OrganizationID = orgID
				h.db.Save(&userOrg)
			} else {
				userOrg = models.UserOrganization{
					UserID:         targetUser.ID,
					OrganizationID: orgID,
					Role:           "member",
					JoinedAt:       time.Now(),
				}
				h.db.Create(&userOrg)
			}
		}
	}

	c.SendResponse(DataAdminUsers, env.CorrelationID, targetUser)
}

func (h *Hub) handleAdminDeleteUser(c *Connection, env models.Envelope) {
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

	var req struct {
		ID string `json:"id"`
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

	// Cleanup user associations and the user itself
	h.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("user_id = ?", targetUID).Delete(&models.UserOrganization{})
		tx.Where("user_id = ?", targetUID).Delete(&models.Workspace{}) // Note: cascading deletes should handle nested resources if configured
		if err := tx.Delete(&models.User{}, "id = ?", targetUID).Error; err != nil {
			return err
		}
		return nil
	})

	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "deleted"})
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
	if req.ManagerID != "" {
		mID, _ := uuid.Parse(req.ManagerID)
		if mID != uuid.Nil {
			org.ManagerID = &mID
		} else {
			org.ManagerID = nil
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

	code := generateRandomCode(8) // Helper to generate alphanumeric code
	invite := models.InviteCode{
		Code:      code,
		CreatedBy: c.UserID,
		IsNewOrg:  payload.IsNewOrg,
		ExpiresAt: time.Now().Add(24 * time.Hour * 7), // 7 days validity
		MaxUses:   payload.MaxUses,
		UseCount:  0,
		CreatedAt: time.Now(),
	}

	if !payload.IsNewOrg && payload.TargetOrgID != "" {
		oid, err := uuid.Parse(payload.TargetOrgID)
		if err != nil {
			c.SendError(env.CorrelationID, "invalid target_org_id")
			return
		}
		invite.OrganizationID = &oid
		invite.Role = "member"
	} else if payload.IsNewOrg {
		invite.Role = "manager" // Creator of new org becomes manager
	} else {
		c.SendError(env.CorrelationID, "must specify target_org_id or is_new_org")
		return
	}

	if err := h.db.Create(&invite).Error; err != nil {
		c.SendError(env.CorrelationID, "failed to create invite: "+err.Error())
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"code":       invite.Code,
		"expires_at": invite.ExpiresAt,
	})
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
				select {
				case conn.Send <- msg:
				default:
				}
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
func createJSONPayload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
