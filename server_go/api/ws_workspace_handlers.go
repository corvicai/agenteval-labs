package api

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"benchmarking-platform/internal/logger"
	"benchmarking-platform/internal/middleware"
	"benchmarking-platform/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (h *Hub) handleGetWorkspaces(c *Connection, env models.Envelope) {
	if c.UserID == uuid.Nil {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var workspaces []models.Workspace
	// Return all workspaces owned by the user (no organization filter)
	h.db.Where("user_id = ?", c.UserID).Find(&workspaces)

	// Add agent count to each workspace if needed, or just return as is
	type WorkspaceResponse struct {
		models.Workspace
		AgentCount int64 `json:"agent_count"`
	}

	var response []WorkspaceResponse
	for _, ws := range workspaces {
		var count int64
		h.db.Model(&models.Agent{}).Where("workspace_id = ?", ws.ID).Count(&count)
		response = append(response, WorkspaceResponse{
			Workspace:  ws,
			AgentCount: count,
		})
	}

	c.SendResponse(DataWorkspaces, env.CorrelationID, response)
}

func (h *Hub) handleSwitchWorkspace(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		WorkspaceID string `json:"workspace_id"`
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

	// Verify access (workspace must belong to user)
	var ws models.Workspace
	if err := h.db.First(&ws, "id = ? AND user_id = ?", wsID, c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "workspace not found or access denied")
		return
	}

	// Update connection
	c.WorkspaceID = wsID

	// Generate new token
	var user models.User
	h.db.First(&user, "id = ?", c.UserID)

	token, err := middleware.GenerateToken(
		c.UserID.String(),
		wsID.String(),
		"", // No organization
		user.Email,
		os.Getenv("JWT_SECRET"),
		"", // impersonatorID
	)

	if err != nil {
		c.SendError(env.CorrelationID, "failed to generate token")
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, map[string]any{
		"token":     token,
		"workspace": ws,
	})
}

func (h *Hub) handleCreateWorkspace(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	ws := models.Workspace{
		ID:     uuid.New(),
		Name:   payload.Name,
		UserID: c.UserID,
		// OrganizationID is not set (will be nil/zero UUID)
	}

	if err := h.db.Create(&ws).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to create workspace", err.Error())
		return
	}

	c.SendResponse(DataResponse, env.CorrelationID, ws)
}

func (h *Hub) handleGetWorkspaceClients(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var clients []models.Client
	h.db.Where("workspace_id = ?", c.WorkspaceID).Find(&clients)

	c.SendResponse(DataResponse, env.CorrelationID, clients)
}

func (h *Hub) handleUpdateAgent(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		ID             string         `json:"id"`
		Name           string         `json:"name"`
		ProviderType   string         `json:"provider_type"`
		Config         map[string]any `json:"config"`
		Enabled        bool           `json:"enabled"`
		Position       int            `json:"position"`
		MaxConcurrency int            `json:"max_concurrency"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	agentID, err := uuid.Parse(payload.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid agent id")
		return
	}

	configJSON, _ := json.Marshal(payload.Config)

	var agent models.Agent
	if err := h.db.First(&agent, "id = ?", agentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.SendError(env.CorrelationID, "agent not found")
			return
		}
		c.SendErrorWithDetails(env.CorrelationID, "failed to load agent (check encryption config)", err.Error())
		return
	}

	// Check for decryption-failed marker - warn but allow update if new config is provided
	var configMap map[string]any
	if err := json.Unmarshal(agent.Config, &configMap); err != nil {
		logger.Warn("[WS][UPDATE_AGENT] Failed to parse agent %s config: %v", agentID, err)
	}
	if _, failed := configMap["_error"]; failed {
		// If no new config is provided, block the update
		if len(payload.Config) == 0 {
			c.SendError(env.CorrelationID, "agent configuration is undecryptable; provide new credentials to fix")
			return
		}
		// Otherwise proceed with update - new config will overwrite the corrupted one
		logger.Info("[WS][UPDATE_AGENT] Overwriting undecryptable config for agent_id=%s", agentID)
	}

	agent.Name = payload.Name
	agent.ProviderType = payload.ProviderType
	agent.Config = models.EncryptedJSON(configJSON)
	agent.Enabled = payload.Enabled
	agent.Position = payload.Position
	if payload.MaxConcurrency > 0 {
		agent.MaxConcurrency = payload.MaxConcurrency
	} else if agent.MaxConcurrency == 0 {
		agent.MaxConcurrency = 5 // Default
	}

	if err := h.db.Save(&agent).Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to update agent", err.Error())
		return
	}

	// Propagate config changes to all QuestionSetAgent links for this agent.
	// Links store a snapshot of the config at assignment time; if stale, runs
	// would use the old credentials instead of the updated ones.
	if err := h.db.Model(&models.QuestionSetAgent{}).
		Where("agent_id = ?", agentID).
		Update("config", models.EncryptedJSON(configJSON)).Error; err != nil {
		logger.Warn("[WS][UPDATE_AGENT] failed to propagate config to question set agents: agent_id=%s err=%v", agentID, err)
	}

	h.BroadcastEvent(agent.WorkspaceID, "agents", "updated", agent)
	c.SendResponse(DataResponse, env.CorrelationID, agent)
}

func (h *Hub) handleCreateAgent(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		WorkspaceID  string         `json:"workspace_id"`
		Name         string         `json:"name"`
		ProviderType string         `json:"provider_type"`
		Config       map[string]any `json:"config"`
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

	configJSON, _ := json.Marshal(payload.Config)
	agent := models.Agent{
		ID:             uuid.New(),
		WorkspaceID:    wsID,
		Name:           payload.Name,
		ProviderType:   payload.ProviderType,
		Config:         models.EncryptedJSON(configJSON),
		Enabled:        true,
		MaxConcurrency: 5, // Default: 5 parallel requests
	}

	if err := h.db.Create(&agent).Error; err != nil {
		logger.Error(
			"[WS][CREATE_AGENT] db create failed user_id=%s workspace_id=%s agent_name=%q provider_type=%q err=%v",
			c.UserID,
			wsID,
			payload.Name,
			payload.ProviderType,
			err,
		)
		errMsg := "failed to create agent"
		if strings.Contains(err.Error(), "ENCRYPTION_KEY") {
			errMsg = "encryption key not configured in production"
		}
		c.SendErrorWithDetails(env.CorrelationID, errMsg, err.Error())
		return
	}

	logger.Info("[WS][CREATE_AGENT] agent created successfully agent_id=%s user_id=%s", agent.ID, c.UserID)

	h.BroadcastEvent(wsID, "agents", "created", agent)
	c.SendResponse(DataResponse, env.CorrelationID, agent)
}

func (h *Hub) handleDeleteAgent(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		ID    string `json:"id"`
		Force bool   `json:"force"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	agentID, err := uuid.Parse(payload.ID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid id")
		return
	}

	// Force mode: delete without loading the agent (bypasses decryption errors)
	if payload.Force {
		tx := h.db.Begin()

		// Get workspace_id before deletion for broadcasting
		var workspaceID uuid.UUID
		h.db.Raw("SELECT workspace_id FROM agents WHERE id = ?", agentID).Scan(&workspaceID)

		if err := tx.Where("agent_id = ?", agentID).Delete(&models.QuestionSetAgent{}).Error; err != nil {
			tx.Rollback()
			c.SendErrorWithDetails(env.CorrelationID, "failed to delete agent mappings", err.Error())
			return
		}

		// Use Exec for direct SQL delete to avoid GORM loading the model
		if err := tx.Exec("DELETE FROM agents WHERE id = ?", agentID).Error; err != nil {
			tx.Rollback()
			c.SendErrorWithDetails(env.CorrelationID, "failed to delete agent", err.Error())
			return
		}

		if err := tx.Commit().Error; err != nil {
			c.SendErrorWithDetails(env.CorrelationID, "failed to delete agent", err.Error())
			return
		}

		if workspaceID != uuid.Nil {
			h.BroadcastEvent(workspaceID, "agents", "deleted", map[string]string{"id": agentID.String()})
		}
		c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "deleted_force"})
		return
	}

	// Normal flow: load agent first (may fail if config is undecryptable)
	var agent models.Agent
	if err := h.db.First(&agent, "id = ?", agentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// If not found, we can't broadcast because we don't know the workspace
			c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "already_deleted"})
			return
		}
		c.SendErrorWithDetails(env.CorrelationID, "failed to load agent for deletion (check encryption config). Use force=true to delete anyway", err.Error())
		return
	}

	tx := h.db.Begin()
	if err := tx.Where("agent_id = ?", agentID).Delete(&models.QuestionSetAgent{}).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to delete agent mappings", err.Error())
		return
	}
	if err := tx.Delete(&agent).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to delete agent", err.Error())
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to delete agent", err.Error())
		return
	}

	h.BroadcastEvent(agent.WorkspaceID, "agents", "deleted", map[string]string{"id": agentID.String()})
	c.SendResponse(DataResponse, env.CorrelationID, map[string]string{"status": "success"})
}

func (h *Hub) handleCloneWorkspace(c *Connection, env models.Envelope) {
	if !c.IsAuthenticated {
		c.SendError(env.CorrelationID, "not authenticated")
		return
	}

	var payload struct {
		SourceWorkspaceID string `json:"source_workspace_id"`
		NewName           string `json:"new_name"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.SendError(env.CorrelationID, "invalid payload")
		return
	}

	if payload.NewName == "" {
		c.SendError(env.CorrelationID, "new_name is required")
		return
	}

	sourceID, err := uuid.Parse(payload.SourceWorkspaceID)
	if err != nil {
		c.SendError(env.CorrelationID, "invalid source_workspace_id")
		return
	}

	// Verify access to source workspace (must belong to user)
	var sourceWS models.Workspace
	if err := h.db.First(&sourceWS, "id = ? AND user_id = ?", sourceID, c.UserID).Error; err != nil {
		c.SendError(env.CorrelationID, "source workspace not found or access denied")
		return
	}

	// Start transaction
	tx := h.db.Begin()

	// Create new workspace (no organization)
	newWS := models.Workspace{
		ID:     uuid.New(),
		Name:   payload.NewName,
		UserID: c.UserID,
		// OrganizationID is not set (will be nil/zero UUID)
	}

	if err := tx.Create(&newWS).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to create cloned workspace", err.Error())
		return
	}

	// Clone agents
	var sourceAgents []models.Agent
	if err := h.db.Where("workspace_id = ?", sourceID).Find(&sourceAgents).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to fetch source agents", err.Error())
		return
	}

	agentIDMap := make(map[uuid.UUID]uuid.UUID) // old -> new
	for _, agent := range sourceAgents {
		newAgent := models.Agent{
			ID:           uuid.New(),
			WorkspaceID:  newWS.ID,
			Name:         agent.Name,
			ProviderType: agent.ProviderType,
			Config:       agent.Config, // Clone config (credentials included)
			Enabled:      agent.Enabled,
			Position:     agent.Position,
		}
		if err := tx.Create(&newAgent).Error; err != nil {
			tx.Rollback()
			c.SendErrorWithDetails(env.CorrelationID, "failed to clone agent: "+agent.Name, err.Error())
			return
		}
		agentIDMap[agent.ID] = newAgent.ID
	}

	// Clone clients (QuestionSets belong to Clients)
	var sourceClients []models.Client
	if err := h.db.Where("workspace_id = ?", sourceID).Find(&sourceClients).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to fetch source clients", err.Error())
		return
	}

	clientIDMap := make(map[uuid.UUID]uuid.UUID) // old -> new
	for _, client := range sourceClients {
		newClient := models.Client{
			ID:          uuid.New(),
			WorkspaceID: newWS.ID,
			Name:        client.Name,
		}
		if err := tx.Create(&newClient).Error; err != nil {
			tx.Rollback()
			c.SendErrorWithDetails(env.CorrelationID, "failed to clone client: "+client.Name, err.Error())
			return
		}
		clientIDMap[client.ID] = newClient.ID
	}

	// Clone question sets
	var sourceQS []models.QuestionSet
	if err := h.db.Where("client_id IN ?", keys(clientIDMap)).Find(&sourceQS).Error; err != nil {
		tx.Rollback()
		c.SendErrorWithDetails(env.CorrelationID, "failed to fetch source question sets", err.Error())
		return
	}

	for _, qs := range sourceQS {
		newClientID, ok := clientIDMap[qs.ClientID]
		if !ok {
			continue // Skip if client not found in map
		}

		newQS := models.QuestionSet{
			ID:       uuid.New(),
			ClientID: newClientID,
			Name:     qs.Name,
			Version:  qs.Version,
			Data:     qs.Data, // Clone questions JSON data
		}
		if err := tx.Create(&newQS).Error; err != nil {
			tx.Rollback()
			c.SendErrorWithDetails(env.CorrelationID, "failed to clone question set: "+qs.Name, err.Error())
			return
		}

		// Clone question set agent associations
		var qsAgents []models.QuestionSetAgent
		if err := h.db.Where("question_set_id = ?", qs.ID).Find(&qsAgents).Error; err == nil {
			for _, qsa := range qsAgents {
				if newAgentID, ok := agentIDMap[qsa.AgentID]; ok {
					newQSA := models.QuestionSetAgent{
						QuestionSetID: newQS.ID,
						AgentID:       newAgentID,
						Config:        qsa.Config,
						Enabled:       qsa.Enabled,
						Position:      qsa.Position,
					}
					tx.Create(&newQSA) // Ignore errors for associations
				}
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.SendErrorWithDetails(env.CorrelationID, "failed to commit clone transaction", err.Error())
		return
	}

	// Count agents for response
	var agentCount int64
	h.db.Model(&models.Agent{}).Where("workspace_id = ?", newWS.ID).Count(&agentCount)

	type WorkspaceResponse struct {
		models.Workspace
		AgentCount int64 `json:"agent_count"`
	}

	c.SendResponse(DataResponse, env.CorrelationID, WorkspaceResponse{
		Workspace:  newWS,
		AgentCount: agentCount,
	})
}

// Helper function to get keys from a map
func keys(m map[uuid.UUID]uuid.UUID) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
