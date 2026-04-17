package api

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	dbcompat "benchmarking-platform/internal/db"
	"benchmarking-platform/internal/logger"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Agent sharing invites reuse the question-set TTL so UX is consistent.
const defaultAgentShareTTL = defaultQuestionSetShareTTL

// -----------------------------------------------------------------------
// dbcompat helpers (mirror of the QS collab pattern)
// -----------------------------------------------------------------------

func (h *Hub) createAgentCollabInviteCompat(invite *models.AgentCollabInvite) error {
	if err := h.db.Create(invite).Error; err != nil {
		if !dbcompat.IsMissingAgentCollaboratorsRelationError(err) {
			return err
		}
		if ensureErr := dbcompat.EnsureAgentCollaboratorSchema(h.db); ensureErr != nil {
			return ensureErr
		}
		return h.db.Create(invite).Error
	}
	return nil
}

func (h *Hub) findAgentCollabInviteByTokenCompat(queryDB *gorm.DB, token string, lock bool, invite *models.AgentCollabInvite) error {
	load := func(db *gorm.DB) error {
		q := db
		if lock {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		return q.Where("token = ?", token).First(invite).Error
	}

	if err := load(queryDB); err != nil {
		if !dbcompat.IsMissingAgentCollaboratorsRelationError(err) {
			return err
		}
		if ensureErr := dbcompat.EnsureAgentCollaboratorSchema(h.db); ensureErr != nil {
			return ensureErr
		}
		return load(queryDB)
	}
	return nil
}

func agentCollabInviteStatus(invite models.AgentCollabInvite) string {
	if invite.AcceptedAt != nil {
		return "used"
	}
	if time.Now().UTC().After(invite.ExpiresAt) {
		return "expired"
	}
	return "ready"
}

// loadAgentWithWorkspace fetches an agent and its owning workspace in two
// queries. Wraps the common "find agent by ID" + ownership check we need
// in nearly every agent-collab handler.
func (h *Hub) loadAgentWithWorkspace(db *gorm.DB, agentID uuid.UUID) (models.Agent, models.Workspace, error) {
	var agent models.Agent
	if err := db.First(&agent, "id = ?", agentID).Error; err != nil {
		return agent, models.Workspace{}, err
	}
	var workspace models.Workspace
	if err := db.First(&workspace, "id = ?", agent.WorkspaceID).Error; err != nil {
		return agent, workspace, err
	}
	return agent, workspace, nil
}

// -----------------------------------------------------------------------
// handleCreateAgentCollabInvite — REQ_CREATE_AGENT_COLLAB_INVITE
// -----------------------------------------------------------------------

func (h *Hub) handleCreateAgentCollabInvite(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		AgentID      string `json:"agent_id"`
		InvitedEmail string `json:"invited_email,omitempty"`
		Role         string `json:"role,omitempty"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	agentID, err := uuid.Parse(payload.AgentID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid agent_id")
		return
	}

	_, workspace, err := h.loadAgentWithWorkspace(h.db, agentID)
	if err != nil {
		c.SendError(env.CorrelationID, "agent not found")
		return
	}

	if workspace.UserID != c.UserID {
		c.SendError(env.CorrelationID, "only the agent owner can create collaboration invites")
		return
	}

	role := strings.TrimSpace(payload.Role)
	if role == "" {
		role = "user"
	}
	// Only 'user' (use-only) is supported for the MVP (D2 in Plano 28).
	if role != "user" {
		c.SendError(env.CorrelationID, "invalid role: must be 'user'")
		return
	}

	token, err := models.GenerateAgentShareToken()
	if err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to generate invite token", err.Error())
		return
	}

	invite := models.AgentCollabInvite{
		ID:              uuid.New(),
		Token:           token,
		AgentID:         agentID,
		CreatedByUserID: c.UserID,
		InvitedEmail:    strings.TrimSpace(payload.InvitedEmail),
		Role:            role,
		ExpiresAt:       time.Now().UTC().Add(defaultAgentShareTTL),
	}

	if err := h.createAgentCollabInviteCompat(&invite); err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to create collaboration invite", err.Error())
		return
	}

	logger.Info("[AGENT-COLLAB] Created invite for agent %s by user %s", agentID, c.UserID)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"token":         invite.Token,
		"expires_at":    invite.ExpiresAt,
		"invited_email": invite.InvitedEmail,
		"role":          invite.Role,
		"agent_id":      agentID.String(),
	})
}

// -----------------------------------------------------------------------
// handleGetAgentCollabInvite — REQ_GET_AGENT_COLLAB_INVITE
// -----------------------------------------------------------------------

func (h *Hub) handleGetAgentCollabInvite(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	token := strings.TrimSpace(payload.Token)
	if token == "" {
		c.SendError(env.CorrelationID, "token is required")
		return
	}

	var invite models.AgentCollabInvite
	if err := h.findAgentCollabInviteByTokenCompat(h.db, token, false, &invite); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
				"status": "invalid",
			})
			return
		}
		c.SendErrorWithDetails(env.CorrelationID, "failed to load collaboration invite", err.Error())
		return
	}

	status := agentCollabInviteStatus(invite)

	var creator models.User
	if err := h.db.First(&creator, "id = ?", invite.CreatedByUserID).Error; err != nil {
		creator = models.User{}
	}

	agent, _, err := h.loadAgentWithWorkspace(h.db, invite.AgentID)
	if err != nil {
		c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
			"status": "invalid",
		})
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"status":         status,
		"token":          invite.Token,
		"expires_at":     invite.ExpiresAt,
		"accepted_at":    invite.AcceptedAt,
		"agent_id":       agent.ID.String(),
		"agent_name":     agent.Name,
		"provider_type":  agent.ProviderType,
		"shared_by_name": creator.Name,
		"role":           invite.Role,
	})
}

// -----------------------------------------------------------------------
// handleAcceptAgentCollabInvite — REQ_ACCEPT_AGENT_COLLAB_INVITE
// -----------------------------------------------------------------------

func (h *Hub) handleAcceptAgentCollabInvite(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	token := strings.TrimSpace(payload.Token)
	if token == "" {
		c.SendError(env.CorrelationID, "token is required")
		return
	}

	var (
		agentID     uuid.UUID
		role        string
		ownerUserID uuid.UUID
	)

	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		var invite models.AgentCollabInvite
		if err := h.findAgentCollabInviteByTokenCompat(tx, token, true, &invite); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("invite not found")
			}
			return err
		}

		switch agentCollabInviteStatus(invite) {
		case "used":
			return errors.New("invite already accepted")
		case "expired":
			return errors.New("invite expired")
		}

		_, workspace, err := h.loadAgentWithWorkspace(tx, invite.AgentID)
		if err != nil {
			return errors.New("agent no longer exists")
		}

		// Prevent owners from accepting their own invite — would create a
		// self-collaborator row and confuse audience resolution.
		if workspace.UserID == c.UserID {
			return errors.New("you already own this agent")
		}

		agentID = invite.AgentID
		role = invite.Role
		ownerUserID = workspace.UserID

		now := time.Now().UTC()

		var existing models.AgentCollaborator
		findErr := tx.Where("agent_id = ? AND user_id = ?", invite.AgentID, c.UserID).
			First(&existing).Error

		if findErr == nil {
			if err := tx.Model(&existing).Updates(map[string]any{
				"role":               role,
				"invited_by_user_id": invite.CreatedByUserID,
				"accepted_at":        now,
				"revoked_at":         nil,
			}).Error; err != nil {
				return err
			}
		} else {
			collab := models.AgentCollaborator{
				ID:              uuid.New(),
				AgentID:         invite.AgentID,
				UserID:          c.UserID,
				Role:            role,
				InvitedByUserID: invite.CreatedByUserID,
				AcceptedAt:      &now,
			}
			if err := tx.Create(&collab).Error; err != nil {
				return err
			}
		}

		invite.AcceptedAt = &now
		if err := tx.Save(&invite).Error; err != nil {
			return err
		}
		return nil
	})

	if txErr != nil {
		c.SendError(env.CorrelationID, txErr.Error())
		return
	}

	// Drop any stale audience so the next broadcast picks up the new user.
	h.InvalidateAgentAudienceCache(agentID)

	// Notify the owner so their UI can refresh the collaborators list.
	_ = h.SendEventToUser(ownerUserID, EvtDataChanged, "", models.DataChangedPayload{
		Resource: "agent_collaborators",
		Action:   "created",
		Data: map[string]any{
			"agent_id": agentID.String(),
			"user_id":  c.UserID.String(),
			"role":     role,
		},
	})

	logger.Info("[AGENT-COLLAB] User %s accepted invite for agent %s (owner=%s)", c.UserID, agentID, ownerUserID)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"agent_id":      agentID.String(),
		"role":          role,
		"owner_user_id": ownerUserID.String(),
	})
}

// -----------------------------------------------------------------------
// handleListAgentCollaborators — REQ_LIST_AGENT_COLLABORATORS
// -----------------------------------------------------------------------

type agentCollaboratorListItem struct {
	UserID     string     `json:"user_id"`
	UserName   string     `json:"user_name"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
}

func (h *Hub) handleListAgentCollaborators(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	agentID, err := uuid.Parse(payload.AgentID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid agent_id")
		return
	}

	_, workspace, err := h.loadAgentWithWorkspace(h.db, agentID)
	if err != nil {
		c.SendError(env.CorrelationID, "agent not found")
		return
	}

	if workspace.UserID != c.UserID {
		c.SendError(env.CorrelationID, "only the agent owner can list collaborators")
		return
	}

	type collabRow struct {
		UserID     uuid.UUID  `gorm:"column:user_id"`
		UserName   string     `gorm:"column:user_name"`
		Email      string     `gorm:"column:email"`
		Role       string     `gorm:"column:role"`
		AcceptedAt *time.Time `gorm:"column:accepted_at"`
	}

	var rows []collabRow
	err = h.db.Raw(`
		SELECT c.user_id, u.name AS user_name, u.email, c.role, c.accepted_at
		FROM agent_collaborators c
		JOIN users u ON u.id = c.user_id
		WHERE c.agent_id = ? AND c.revoked_at IS NULL
		ORDER BY c.created_at ASC
	`, agentID).Scan(&rows).Error

	if err != nil {
		if dbcompat.IsMissingAgentCollaboratorsRelationError(err) {
			c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
				"collaborators": []agentCollaboratorListItem{},
			})
			return
		}
		c.SendErrorWithDetails(env.CorrelationID, "failed to list collaborators", err.Error())
		return
	}

	items := make([]agentCollaboratorListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, agentCollaboratorListItem{
			UserID:     r.UserID.String(),
			UserName:   r.UserName,
			Email:      r.Email,
			Role:       r.Role,
			AcceptedAt: r.AcceptedAt,
		})
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"collaborators": items,
	})
}

// -----------------------------------------------------------------------
// handleRevokeAgentCollaborator — REQ_REVOKE_AGENT_COLLABORATOR
// -----------------------------------------------------------------------

func (h *Hub) handleRevokeAgentCollaborator(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		AgentID string `json:"agent_id"`
		UserID  string `json:"user_id"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	agentID, err := uuid.Parse(payload.AgentID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid agent_id")
		return
	}

	revokedUserID, err := uuid.Parse(payload.UserID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid user_id")
		return
	}

	_, workspace, err := h.loadAgentWithWorkspace(h.db, agentID)
	if err != nil {
		c.SendError(env.CorrelationID, "agent not found")
		return
	}

	if workspace.UserID != c.UserID {
		c.SendError(env.CorrelationID, "only the agent owner can revoke collaborators")
		return
	}

	now := time.Now().UTC()
	result := h.db.Model(&models.AgentCollaborator{}).
		Where("agent_id = ? AND user_id = ? AND revoked_at IS NULL", agentID, revokedUserID).
		Update("revoked_at", now)

	if result.Error != nil {
		if dbcompat.IsMissingAgentCollaboratorsRelationError(result.Error) {
			c.SendError(env.CorrelationID, "collaborator not found")
			return
		}
		c.SendErrorWithDetails(env.CorrelationID, "failed to revoke collaborator", result.Error.Error())
		return
	}

	if result.RowsAffected == 0 {
		c.SendError(env.CorrelationID, "collaborator not found or already revoked")
		return
	}

	h.InvalidateAgentAudienceCache(agentID)

	// Tell the revoked user to drop this agent from their "shared with me"
	// list. Routed through SendEventToUser so the replay buffer captures it.
	_ = h.SendEventToUser(revokedUserID, EvtCollaboratorRevoked, "", map[string]any{
		"resource": "agents",
		"agent_id": agentID.String(),
		"user_id":  revokedUserID.String(),
	})

	logger.Info("[AGENT-COLLAB] Owner %s revoked collaborator %s from agent %s", c.UserID, revokedUserID, agentID)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"status":   "revoked",
		"agent_id": agentID.String(),
		"user_id":  revokedUserID.String(),
	})
}
