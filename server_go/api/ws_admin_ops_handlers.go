package api

import (
	"encoding/json"

	"benchmarking-platform/models"

	"github.com/google/uuid"
)

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
