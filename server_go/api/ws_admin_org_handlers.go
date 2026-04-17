package api

import (
	"encoding/json"
	"time"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"

	"github.com/google/uuid"
)

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
		if err := json.Unmarshal(env.Payload, &filter); err != nil {
			logger.Warn("[ADMIN] Failed to parse organizations filter payload: %v", err)
		}
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
		if firstMid, err := uuid.Parse(req.ManagerIDs[0]); err == nil {
			org.ManagerID = &firstMid
		} else {
			logger.Warn("[ADMIN] Invalid first ManagerID %q: %v", req.ManagerIDs[0], err)
		}

	} else if req.ManagerID != "" {
		// Legacy single manager logic (frontend sending old payload or specific single set)
		mID, err := uuid.Parse(req.ManagerID)
		if err != nil {
			c.SendError(env.CorrelationID, "invalid manager ID")
			return
		}
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
