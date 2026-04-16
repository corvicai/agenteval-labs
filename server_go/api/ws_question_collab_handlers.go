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

// -----------------------------------------------------------------------
// compat helpers (mirrors ws_question_share_handlers.go pattern)
// -----------------------------------------------------------------------

func (h *Hub) createQuestionSetCollabInviteCompat(invite *models.QuestionSetCollabInvite) error {
	if err := h.db.Create(invite).Error; err != nil {
		if !dbcompat.IsMissingQuestionSetCollaboratorsRelationError(err) {
			return err
		}
		if ensureErr := dbcompat.EnsureQuestionSetCollaboratorSchema(h.db); ensureErr != nil {
			return ensureErr
		}
		return h.db.Create(invite).Error
	}
	return nil
}

func (h *Hub) findCollabInviteByTokenCompat(queryDB *gorm.DB, token string, lock bool, invite *models.QuestionSetCollabInvite) error {
	load := func(db *gorm.DB) error {
		q := db
		if lock {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		return q.Where("token = ?", token).First(invite).Error
	}

	if err := load(queryDB); err != nil {
		if !dbcompat.IsMissingQuestionSetCollaboratorsRelationError(err) {
			return err
		}
		if ensureErr := dbcompat.EnsureQuestionSetCollaboratorSchema(h.db); ensureErr != nil {
			return ensureErr
		}
		return load(queryDB)
	}
	return nil
}

func questionSetCollabInviteStatus(invite models.QuestionSetCollabInvite) string {
	if invite.AcceptedAt != nil {
		return "used"
	}
	if time.Now().UTC().After(invite.ExpiresAt) {
		return "expired"
	}
	return "ready"
}

// -----------------------------------------------------------------------
// handleCreateQuestionSetCollabInvite — REQ_CREATE_QS_COLLAB_INVITE
// -----------------------------------------------------------------------

func (h *Hub) handleCreateQuestionSetCollabInvite(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		QuestionSetID string `json:"question_set_id"`
		InvitedEmail  string `json:"invited_email,omitempty"`
		Role          string `json:"role,omitempty"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	questionSetID, err := uuid.Parse(payload.QuestionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid question_set_id")
		return
	}

	_, _, workspace, err := h.loadQuestionSetWithWorkspace(h.db, questionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}

	// Only the owner may create invites.
	if workspace.UserID != c.UserID {
		c.SendError(env.CorrelationID, "only the question set owner can create collaboration invites")
		return
	}

	role := strings.TrimSpace(payload.Role)
	if role == "" {
		role = "editor"
	}
	if role != "editor" && role != "viewer" {
		c.SendError(env.CorrelationID, "invalid role: must be 'editor' or 'viewer'")
		return
	}

	token, err := models.GenerateQuestionSetShareToken()
	if err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to generate invite token", err.Error())
		return
	}

	invite := models.QuestionSetCollabInvite{
		ID:              uuid.New(),
		Token:           token,
		QuestionSetID:   questionSetID,
		CreatedByUserID: c.UserID,
		InvitedEmail:    strings.TrimSpace(payload.InvitedEmail),
		Role:            role,
		ExpiresAt:       time.Now().UTC().Add(defaultQuestionSetShareTTL),
	}

	if err := h.createQuestionSetCollabInviteCompat(&invite); err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to create collaboration invite", err.Error())
		return
	}

	logger.Info("[COLLAB] Created collab invite for QS %s by user %s", questionSetID, c.UserID)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"token":           invite.Token,
		"expires_at":      invite.ExpiresAt,
		"invited_email":   invite.InvitedEmail,
		"role":            invite.Role,
		"question_set_id": questionSetID.String(),
	})
}

// -----------------------------------------------------------------------
// handleGetQuestionSetCollabInvite — REQ_GET_QS_COLLAB_INVITE
// -----------------------------------------------------------------------

func (h *Hub) handleGetQuestionSetCollabInvite(c *Connection, env models.Envelope) {
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

	var invite models.QuestionSetCollabInvite
	if err := h.findCollabInviteByTokenCompat(h.db, token, false, &invite); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
				"status": "invalid",
			})
			return
		}
		c.SendErrorWithDetails(env.CorrelationID, "failed to load collaboration invite", err.Error())
		return
	}

	status := questionSetCollabInviteStatus(invite)

	var creator models.User
	if err := h.db.First(&creator, "id = ?", invite.CreatedByUserID).Error; err != nil {
		creator = models.User{}
	}

	questionSet, _, _, err := h.loadQuestionSetWithWorkspace(h.db, invite.QuestionSetID)
	if err != nil {
		c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
			"status": "invalid",
		})
		return
	}

	categoryCount, questionCount := questionSetCounts(questionSet.Data)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"status":            status,
		"token":             invite.Token,
		"expires_at":        invite.ExpiresAt,
		"accepted_at":       invite.AcceptedAt,
		"question_set_id":   questionSet.ID.String(),
		"question_set_name": questionSet.Name,
		"category_count":    categoryCount,
		"question_count":    questionCount,
		"shared_by_name":    creator.Name,
		"role":              invite.Role,
	})
}

// -----------------------------------------------------------------------
// handleAcceptQuestionSetCollabInvite — REQ_ACCEPT_QS_COLLAB_INVITE
// -----------------------------------------------------------------------

func (h *Hub) handleAcceptQuestionSetCollabInvite(c *Connection, env models.Envelope) {
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
		ownerWorkspaceID uuid.UUID
		ownerUserID      uuid.UUID
		role             string
		questionSetID    uuid.UUID
	)

	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		var invite models.QuestionSetCollabInvite
		if err := h.findCollabInviteByTokenCompat(tx, token, true, &invite); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("invite not found")
			}
			return err
		}

		switch questionSetCollabInviteStatus(invite) {
		case "used":
			return errors.New("invite already accepted")
		case "expired":
			return errors.New("invite expired")
		}

		_, _, workspace, err := h.loadQuestionSetWithWorkspace(tx, invite.QuestionSetID)
		if err != nil {
			return errors.New("question set no longer exists")
		}

		ownerWorkspaceID = workspace.ID
		ownerUserID = workspace.UserID
		role = invite.Role
		questionSetID = invite.QuestionSetID

		// Upsert collaborator: on conflict re-activate (clear revoked_at, update accepted_at)
		now := time.Now().UTC()
		collab := models.QuestionSetCollaborator{
			ID:              uuid.New(),
			QuestionSetID:   invite.QuestionSetID,
			UserID:          c.UserID,
			Role:            role,
			InvitedByUserID: invite.CreatedByUserID,
			AcceptedAt:      &now,
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "question_set_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"role":              role,
				"invited_by_user_id": invite.CreatedByUserID,
				"accepted_at":       now,
				"revoked_at":        nil,
			}),
		}).Create(&collab).Error; err != nil {
			return err
		}

		// Mark invite as accepted
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

	// Invalidate audience cache so broadcasts pick up the new collaborator.
	h.InvalidateAudienceCache(questionSetID)

	// Broadcast to owner so their UI reflects the new collaborator.
	h.BroadcastEvent(ownerWorkspaceID, "question_set_collaborators", "created", map[string]any{
		"question_set_id":    questionSetID.String(),
		"user_id":            c.UserID.String(),
		"role":               role,
		"owner_workspace_id": ownerWorkspaceID.String(),
	})

	logger.Info("[COLLAB] User %s accepted collab invite for QS %s (owner workspace: %s)", c.UserID, questionSetID, ownerWorkspaceID)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"question_set_id":    questionSetID.String(),
		"role":               role,
		"owner_workspace_id": ownerWorkspaceID.String(),
		"owner_user_id":      ownerUserID.String(),
	})
}

// -----------------------------------------------------------------------
// handleListQuestionSetCollaborators — REQ_LIST_QS_COLLABORATORS
// -----------------------------------------------------------------------

type collaboratorListItem struct {
	UserID    string     `json:"user_id"`
	UserName  string     `json:"user_name"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
}

func (h *Hub) handleListQuestionSetCollaborators(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		QuestionSetID string `json:"question_set_id"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	questionSetID, err := uuid.Parse(payload.QuestionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid question_set_id")
		return
	}

	_, _, workspace, err := h.loadQuestionSetWithWorkspace(h.db, questionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}

	// Only owner may list.
	if workspace.UserID != c.UserID {
		c.SendError(env.CorrelationID, "only the question set owner can list collaborators")
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
		FROM question_set_collaborators c
		JOIN users u ON u.id = c.user_id
		WHERE c.question_set_id = ? AND c.revoked_at IS NULL
		ORDER BY c.created_at ASC
	`, questionSetID).Scan(&rows).Error

	if err != nil {
		if dbcompat.IsMissingQuestionSetCollaboratorsRelationError(err) {
			c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
				"collaborators": []collaboratorListItem{},
			})
			return
		}
		c.SendErrorWithDetails(env.CorrelationID, "failed to list collaborators", err.Error())
		return
	}

	items := make([]collaboratorListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, collaboratorListItem{
			UserID:    r.UserID.String(),
			UserName:  r.UserName,
			Email:     r.Email,
			Role:      r.Role,
			AcceptedAt: r.AcceptedAt,
		})
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"collaborators": items,
	})
}

// -----------------------------------------------------------------------
// handleRevokeQuestionSetCollaborator — REQ_REVOKE_QS_COLLABORATOR
// -----------------------------------------------------------------------

func (h *Hub) handleRevokeQuestionSetCollaborator(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		QuestionSetID string `json:"question_set_id"`
		UserID        string `json:"user_id"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	questionSetID, err := uuid.Parse(payload.QuestionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid question_set_id")
		return
	}

	revokedUserID, err := uuid.Parse(payload.UserID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid user_id")
		return
	}

	_, _, workspace, err := h.loadQuestionSetWithWorkspace(h.db, questionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}

	// Only owner may revoke.
	if workspace.UserID != c.UserID {
		c.SendError(env.CorrelationID, "only the question set owner can revoke collaborators")
		return
	}

	now := time.Now().UTC()
	result := h.db.Model(&models.QuestionSetCollaborator{}).
		Where("question_set_id = ? AND user_id = ? AND revoked_at IS NULL", questionSetID, revokedUserID).
		Update("revoked_at", now)

	if result.Error != nil {
		if dbcompat.IsMissingQuestionSetCollaboratorsRelationError(result.Error) {
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

	// Invalidate audience cache so the next broadcast won't include this user.
	h.InvalidateAudienceCache(questionSetID)

	// Notify the revoked user so their UI can remove the shared QS.
	revokedPayload, _ := json.Marshal(models.Envelope{
		Type: EvtCollaboratorRevoked,
		Payload: func() json.RawMessage {
			b, _ := json.Marshal(map[string]any{
				"question_set_id": questionSetID.String(),
				"user_id":         revokedUserID.String(),
			})
			return b
		}(),
	})
	h.BroadcastToUser(revokedUserID, revokedPayload)

	logger.Info("[COLLAB] Owner %s revoked collaborator %s from QS %s", c.UserID, revokedUserID, questionSetID)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"status":          "revoked",
		"question_set_id": questionSetID.String(),
		"user_id":         revokedUserID.String(),
	})
}
