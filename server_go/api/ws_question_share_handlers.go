package api

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const defaultQuestionSetShareTTL = 7 * 24 * time.Hour

func (h *Hub) loadQuestionSetWithWorkspace(db *gorm.DB, questionSetID uuid.UUID) (models.QuestionSet, models.Client, models.Workspace, error) {
	var qs models.QuestionSet
	if err := db.Preload("Client").Preload("Agents").First(&qs, "id = ?", questionSetID).Error; err != nil {
		return models.QuestionSet{}, models.Client{}, models.Workspace{}, err
	}

	var client models.Client
	if err := db.First(&client, "id = ?", qs.ClientID).Error; err != nil {
		return models.QuestionSet{}, models.Client{}, models.Workspace{}, err
	}

	var workspace models.Workspace
	if err := db.First(&workspace, "id = ?", client.WorkspaceID).Error; err != nil {
		return models.QuestionSet{}, models.Client{}, models.Workspace{}, err
	}

	return qs, client, workspace, nil
}

func (h *Hub) loadOwnedWorkspace(db *gorm.DB, userID, workspaceID uuid.UUID) (models.Workspace, error) {
	var workspace models.Workspace
	if err := db.First(&workspace, "id = ? AND user_id = ?", workspaceID, userID).Error; err != nil {
		return models.Workspace{}, err
	}
	return workspace, nil
}

func (h *Hub) ensureWorkspaceClient(tx *gorm.DB, workspaceID uuid.UUID) (models.Client, error) {
	var client models.Client
	err := tx.Where("workspace_id = ?", workspaceID).Order("created_at ASC").First(&client).Error
	if err == nil {
		return client, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Client{}, err
	}

	client = models.Client{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Name:        "Default Client",
	}
	if err := tx.Create(&client).Error; err != nil {
		return models.Client{}, err
	}
	return client, nil
}

func (h *Hub) cloneQuestionSetOnly(tx *gorm.DB, source models.QuestionSet, targetWorkspaceID uuid.UUID, name string) (models.QuestionSet, error) {
	targetClient, err := h.ensureWorkspaceClient(tx, targetWorkspaceID)
	if err != nil {
		return models.QuestionSet{}, err
	}

	finalName := strings.TrimSpace(name)
	if finalName == "" {
		finalName = source.Name
	}

	cloned := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: targetClient.ID,
		Name:     finalName,
		Version:  source.Version,
		Data:     source.Data,
	}
	if err := tx.Create(&cloned).Error; err != nil {
		return models.QuestionSet{}, err
	}
	if err := tx.Preload("Client").Preload("Agents").First(&cloned, "id = ?", cloned.ID).Error; err != nil {
		return models.QuestionSet{}, err
	}
	return cloned, nil
}

func questionSetCounts(raw []byte) (int, int) {
	var payload struct {
		Categories []struct {
			Questions []json.RawMessage `json:"questions"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, 0
	}

	questionCount := 0
	for _, category := range payload.Categories {
		questionCount += len(category.Questions)
	}
	return len(payload.Categories), questionCount
}

func questionSetShareStatus(link models.QuestionSetShareLink) string {
	if link.UsedAt != nil {
		return "used"
	}
	if time.Now().UTC().After(link.ExpiresAt) {
		return "expired"
	}
	return "ready"
}

func (h *Hub) handleCreateQuestionSetShareLink(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		QuestionSetID  string `json:"question_set_id"`
		ExpiresInHours int    `json:"expires_in_hours,omitempty"`
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

	questionSet, _, workspace, err := h.loadQuestionSetWithWorkspace(h.db, questionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}
	if workspace.ID != c.WorkspaceID {
		c.SendError(env.CorrelationID, "workspace mismatch")
		return
	}

	token, err := models.GenerateQuestionSetShareToken()
	if err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to create share link", err.Error())
		return
	}

	ttl := defaultQuestionSetShareTTL
	if payload.ExpiresInHours > 0 {
		ttl = time.Duration(payload.ExpiresInHours) * time.Hour
		if ttl > 30*24*time.Hour {
			ttl = 30 * 24 * time.Hour
		}
	}

	link := models.QuestionSetShareLink{
		ID:              uuid.New(),
		Token:           token,
		QuestionSetID:   questionSet.ID,
		CreatedByUserID: c.UserID,
		ExpiresAt:       time.Now().UTC().Add(ttl),
	}
	if err := h.db.Create(&link).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to create share link", err.Error())
		return
	}

	categoryCount, questionCount := questionSetCounts(questionSet.Data)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"token":             link.Token,
		"expires_at":        link.ExpiresAt,
		"question_set_id":   questionSet.ID.String(),
		"question_set_name": questionSet.Name,
		"category_count":    categoryCount,
		"question_count":    questionCount,
	})
}

func (h *Hub) handleGetQuestionSetShareLink(c *Connection, env models.Envelope) {
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

	var link models.QuestionSetShareLink
	if err := h.db.Where("token = ?", token).First(&link).Error; err != nil {
		c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
			"status": "invalid",
		})
		return
	}

	status := questionSetShareStatus(link)

	var creator models.User
	if err := h.db.First(&creator, "id = ?", link.CreatedByUserID).Error; err != nil {
		creator = models.User{}
	}

	questionSet, _, workspace, err := h.loadQuestionSetWithWorkspace(h.db, link.QuestionSetID)
	if err != nil {
		c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
			"status": "invalid",
		})
		return
	}

	categoryCount, questionCount := questionSetCounts(questionSet.Data)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"status":                status,
		"token":                 link.Token,
		"expires_at":            link.ExpiresAt,
		"used_at":               link.UsedAt,
		"question_set_id":       questionSet.ID.String(),
		"question_set_name":     questionSet.Name,
		"category_count":        categoryCount,
		"question_count":        questionCount,
		"shared_by_name":        creator.Name,
		"source_workspace_id":   workspace.ID.String(),
		"source_workspace_name": workspace.Name,
	})
}

func (h *Hub) handleAcceptQuestionSetShareLink(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		Token             string `json:"token"`
		TargetWorkspaceID string `json:"target_workspace_id"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	targetWorkspaceID, err := uuid.Parse(payload.TargetWorkspaceID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid target_workspace_id")
		return
	}

	targetWorkspace, err := h.loadOwnedWorkspace(h.db, c.UserID, targetWorkspaceID)
	if err != nil {
		c.SendError(env.CorrelationID, "target workspace not found or access denied")
		return
	}

	tx := h.db.Begin()

	var link models.QuestionSetShareLink
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("token = ?", strings.TrimSpace(payload.Token)).
		First(&link).Error; err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "share link not found")
		return
	}

	switch questionSetShareStatus(link) {
	case "used":
		tx.Rollback()
		c.SendError(env.CorrelationID, "share link already used")
		return
	case "expired":
		tx.Rollback()
		c.SendError(env.CorrelationID, "share link expired")
		return
	}

	sourceQuestionSet, _, _, err := h.loadQuestionSetWithWorkspace(tx, link.QuestionSetID)
	if err != nil {
		tx.Rollback()
		c.SendError(env.CorrelationID, "shared question set no longer exists")
		return
	}

	cloned, err := h.cloneQuestionSetOnly(tx, sourceQuestionSet, targetWorkspaceID, sourceQuestionSet.Name)
	if err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to accept share link", err.Error())
		return
	}

	now := time.Now().UTC()
	link.UsedAt = &now
	link.UsedByUserID = &c.UserID
	link.AcceptedQuestionSetID = &cloned.ID
	if err := tx.Save(&link).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to accept share link", err.Error())
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to accept share link", err.Error())
		return
	}

	h.BroadcastEvent(targetWorkspaceID, "question_sets", "created", cloned)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"question_set":      cloned,
		"workspace_id":      targetWorkspace.ID.String(),
		"workspace_name":    targetWorkspace.Name,
		"accepted_via_link": true,
	})
}

func (h *Hub) handleCopyQuestionSetToWorkspace(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		QuestionSetID     string `json:"question_set_id"`
		TargetWorkspaceID string `json:"target_workspace_id"`
		Name              string `json:"name,omitempty"`
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
	targetWorkspaceID, err := uuid.Parse(payload.TargetWorkspaceID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid target_workspace_id")
		return
	}
	if targetWorkspaceID == c.WorkspaceID {
		c.SendError(env.CorrelationID, "target workspace must be different from current workspace")
		return
	}

	targetWorkspace, err := h.loadOwnedWorkspace(h.db, c.UserID, targetWorkspaceID)
	if err != nil {
		c.SendError(env.CorrelationID, "target workspace not found or access denied")
		return
	}

	sourceQuestionSet, _, sourceWorkspace, err := h.loadQuestionSetWithWorkspace(h.db, questionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}
	if sourceWorkspace.ID != c.WorkspaceID {
		c.SendError(env.CorrelationID, "workspace mismatch")
		return
	}

	tx := h.db.Begin()
	cloned, err := h.cloneQuestionSetOnly(tx, sourceQuestionSet, targetWorkspaceID, payload.Name)
	if err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to copy question set", err.Error())
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to copy question set", err.Error())
		return
	}

	h.BroadcastEvent(targetWorkspaceID, "question_sets", "created", cloned)
	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"question_set":   cloned,
		"workspace_id":   targetWorkspace.ID.String(),
		"workspace_name": targetWorkspace.Name,
	})
}

func (h *Hub) handleMoveQuestionSetToWorkspace(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		QuestionSetID     string `json:"question_set_id"`
		TargetWorkspaceID string `json:"target_workspace_id"`
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
	targetWorkspaceID, err := uuid.Parse(payload.TargetWorkspaceID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid target_workspace_id")
		return
	}
	if targetWorkspaceID == c.WorkspaceID {
		c.SendError(env.CorrelationID, "target workspace must be different from current workspace")
		return
	}

	targetWorkspace, err := h.loadOwnedWorkspace(h.db, c.UserID, targetWorkspaceID)
	if err != nil {
		c.SendError(env.CorrelationID, "target workspace not found or access denied")
		return
	}

	sourceQuestionSet, _, sourceWorkspace, err := h.loadQuestionSetWithWorkspace(h.db, questionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}
	if sourceWorkspace.ID != c.WorkspaceID {
		c.SendError(env.CorrelationID, "workspace mismatch")
		return
	}

	tx := h.db.Begin()
	targetClient, err := h.ensureWorkspaceClient(tx, targetWorkspaceID)
	if err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to move question set", err.Error())
		return
	}

	if err := tx.Where("question_set_id = ?", questionSetID).Delete(&models.QuestionSetAgent{}).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to clear question set agents", err.Error())
		return
	}
	if err := tx.Model(&models.QuestionSet{}).
		Where("id = ?", questionSetID).
		Update("client_id", targetClient.ID).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to move question set", err.Error())
		return
	}
	if err := tx.Model(&models.Run{}).
		Where("question_set_id = ?", questionSetID).
		Update("workspace_id", targetWorkspaceID).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to move question set history", err.Error())
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to move question set", err.Error())
		return
	}

	movedQuestionSet, _, _, err := h.loadQuestionSetWithWorkspace(h.db, questionSetID)
	if err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to load moved question set", err.Error())
		return
	}

	h.BroadcastEvent(sourceWorkspace.ID, "question_sets", "deleted", map[string]string{"id": sourceQuestionSet.ID.String()})
	h.BroadcastEvent(targetWorkspaceID, "question_sets", "created", movedQuestionSet)

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"question_set":   movedQuestionSet,
		"workspace_id":   targetWorkspace.ID.String(),
		"workspace_name": targetWorkspace.Name,
		"moved":          true,
	})
}
