package api

import (
	"encoding/json"
	"log"

	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func (h *Hub) handleImportQuestionSet(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		ClientID string `json:"client_id"`
		Name     string `json:"name"`
		Data     any    `json:"data"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	clientID, err := uuid.Parse(payload.ClientID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid client_id")
		return
	}

	var client models.Client
	if err := h.db.First(&client, "id = ?", clientID).Error; err != nil {
		c.SendError(env.CorrelationID, "client not found")
		return
	}

	dataJSON, _ := json.Marshal(payload.Data)

	qs := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: clientID,
		Name:     payload.Name,
		Data:     datatypes.JSON(dataJSON),
	}

	if err := h.db.Create(&qs).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to create question set", err.Error())
		return
	}

	h.BroadcastEvent(client.WorkspaceID, "question_sets", "created", qs)
	c.SendResponse(DataResponse, env.CorrelationID, qs)
}

func (h *Hub) handleExportQuestionSet(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	qsID, err := uuid.Parse(payload.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid id")
		return
	}

	var qs models.QuestionSet
	if err := h.db.First(&qs, "id = ?", qsID).Error; err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}

	var data any
	json.Unmarshal(qs.Data, &data)

	c.SendResponse(DataResponse, env.CorrelationID, data)
}

func (h *Hub) handleUpdateQuestionSet(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Data any    `json:"data"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	qsID, err := uuid.Parse(payload.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid id")
		return
	}

	var qs models.QuestionSet
	if err := h.db.First(&qs, "id = ?", qsID).Error; err != nil {
		c.SendError(env.CorrelationID, "question set not found")
		return
	}

	dataJSON, _ := json.Marshal(payload.Data)
	qs.Name = payload.Name
	qs.Data = datatypes.JSON(dataJSON)

	if err := h.db.Save(&qs).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to update question set", err.Error())
		return
	}

	// Fetch workspace ID from client for broadcasting
	var client models.Client
	h.db.First(&client, "id = ?", qs.ClientID)

	h.db.Preload("Client").Preload("Agents").First(&qs, "id = ?", qsID)
	h.BroadcastEvent(client.WorkspaceID, "question_sets", "updated", qs)
	c.SendResponse(DataResponse, env.CorrelationID, qs)
}

func (h *Hub) handleCreateQuestionSet(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
		Data        any    `json:"data"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	wsID, err := uuid.Parse(payload.WorkspaceID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid workspace_id")
		return
	}

	// Find or create default client for workspace
	var client models.Client
	if err := h.db.Where("workspace_id = ?", wsID).First(&client).Error; err != nil {
		client = models.Client{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			Name:        "Default Client",
		}
		h.db.Create(&client)
	}

	dataJSON, _ := json.Marshal(payload.Data)
	qs := models.QuestionSet{
		ID:       uuid.New(),
		ClientID: client.ID,
		Name:     payload.Name,
		Data:     datatypes.JSON(dataJSON),
	}

	if err := h.db.Create(&qs).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to create question set", err.Error())
		return
	}

	// Preload before broadcast
	h.db.Preload("Client").Preload("Agents").First(&qs, "id = ?", qs.ID)

	h.BroadcastEvent(wsID, "question_sets", "created", qs)
	c.SendResponse(DataResponse, env.CorrelationID, qs)
}

func (h *Hub) handleUpdateQuestionSetAgents(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		QuestionSetID string `json:"question_set_id"`
		Agents        []struct {
			AgentID  string         `json:"agent_id"`
			Config   map[string]any `json:"config"`
			Enabled  bool           `json:"enabled"`
			Position int            `json:"position"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	log.Printf("[QS_AGENTS] Updating agents for QS %s, received %d agents", payload.QuestionSetID, len(payload.Agents))
	for i, a := range payload.Agents {
		log.Printf("[QS_AGENTS]   Agent %d: %s enabled=%v pos=%d", i, a.AgentID, a.Enabled, a.Position)
	}

	qsID, err := uuid.Parse(payload.QuestionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid question_set_id")
		return
	}

	type qsMeta struct {
		WorkspaceID uuid.UUID `json:"workspace_id"`
	}
	var meta qsMeta
	if err := h.db.Table("question_sets").
		Select("clients.workspace_id").
		Joins("JOIN clients ON clients.id = question_sets.client_id").
		Where("question_sets.id = ?", qsID).
		Scan(&meta).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "question set not found", err.Error())
		return
	}
	if meta.WorkspaceID == uuid.Nil {
		c.SendError(env.CorrelationID, "question set workspace not found")
		return
	}

	// Admin Bypass: If user is admin, allow cross-workspace agent linking (needed for seeder)
	var user models.User
	h.db.First(&user, "id = ?", c.UserID)

	if !user.IsAdmin && c.WorkspaceID != uuid.Nil && meta.WorkspaceID != c.WorkspaceID {
		c.SendError(env.CorrelationID, "workspace mismatch")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		// Delete existing mappings
		if err := tx.Where("question_set_id = ?", qsID).Delete(&models.QuestionSetAgent{}).Error; err != nil {
			return err
		}

		// Insert new mappings
		var mappings []models.QuestionSetAgent
		for _, a := range payload.Agents {
			if !a.Enabled {
				continue
			}
			agentID, err := uuid.Parse(a.AgentID)
			if err != nil {
				log.Printf("[QS_AGENTS] Skipping invalid agent_id: %s", a.AgentID)
				continue
			}
			configJSON, _ := json.Marshal(a.Config)

			mappings = append(mappings, models.QuestionSetAgent{
				QuestionSetID: qsID,
				AgentID:       agentID,
				Config:        models.EncryptedJSON(configJSON),
				Enabled:       true,
				Position:      a.Position,
			})
		}

		if len(mappings) > 0 {
			if err := tx.Select("QuestionSetID", "AgentID", "Config", "Enabled", "Position").Create(&mappings).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to update question set agents", err.Error())
		return
	}

	// Fetch updated question set with agents to broadast/respond
	var qs models.QuestionSet
	h.db.Preload("Client").Preload("Agents").First(&qs, "id = ?", qsID)

	h.BroadcastEvent(meta.WorkspaceID, "question_sets", "updated", qs)
	c.SendResponse(DataResponse, env.CorrelationID, qs)
}

func (h *Hub) handleGetQuestionSetAgentEnvelope(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload models.GetQuestionSetAgentEnvelopePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	qsID, err := uuid.Parse(payload.QuestionSetID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid question_set_id")
		return
	}

	type qsMeta struct {
		WorkspaceID uuid.UUID `json:"workspace_id"`
	}
	var meta qsMeta
	if err := h.db.Table("question_sets").
		Select("clients.workspace_id").
		Joins("JOIN clients ON clients.id = question_sets.client_id").
		Where("question_sets.id = ?", qsID).
		Scan(&meta).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "question set not found", err.Error())
		return
	}
	if meta.WorkspaceID == uuid.Nil {
		c.SendError(env.CorrelationID, "question set workspace not found")
		return
	}

	var user models.User
	h.db.First(&user, "id = ?", c.UserID)
	if !user.IsAdmin && c.WorkspaceID != uuid.Nil && meta.WorkspaceID != c.WorkspaceID {
		c.SendError(env.CorrelationID, "workspace mismatch")
		return
	}

	var workspaceAgents []models.Agent
	if err := h.db.Where("workspace_id = ?", meta.WorkspaceID).
		Order("position ASC").
		Order("created_at ASC").
		Find(&workspaceAgents).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to load workspace agents", err.Error())
		return
	}

	var qsAgents []models.QuestionSetAgent
	if err := h.db.Preload("Agent").
		Where("question_set_id = ?", qsID).
		Order("position ASC").
		Find(&qsAgents).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to load question set agents", err.Error())
		return
	}

	selectedAgents := make([]models.Agent, 0, len(qsAgents))
	selectedIDs := make(map[uuid.UUID]struct{}, len(qsAgents))

	for _, link := range qsAgents {
		if !link.Enabled {
			continue
		}
		if link.Agent.ID == uuid.Nil {
			continue
		}
		selected := link.Agent
		if len(link.Config) > 0 {
			selected.Config = link.Config
		}
		selected.Enabled = true
		selected.Position = link.Position
		selectedAgents = append(selectedAgents, selected)
		selectedIDs[selected.ID] = struct{}{}
	}

	if len(selectedAgents) == 0 {
		for _, candidate := range workspaceAgents {
			if !candidate.Enabled || candidate.ProviderType == "evaluator" {
				continue
			}
			selectedAgents = append(selectedAgents, candidate)
			selectedIDs[candidate.ID] = struct{}{}
		}
	}

	availableAgents := make([]models.Agent, 0, len(workspaceAgents))
	for _, candidate := range workspaceAgents {
		if _, ok := selectedIDs[candidate.ID]; ok {
			continue
		}
		availableAgents = append(availableAgents, candidate)
	}

	c.SendResponse(DataResponse, env.CorrelationID, models.QuestionSetAgentEnvelopeResponse{
		QuestionSetID:   qsID.String(),
		SelectedAgents:  selectedAgents,
		AvailableAgents: availableAgents,
	})
}
